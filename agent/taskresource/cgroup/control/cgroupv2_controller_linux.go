//go:build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package control

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/cihub/seelog"
	cgroupsv2 "github.com/containerd/cgroups/v2"
)

const (
	defaultCgroupv2Path = "/sys/fs/cgroup"
	parentCgroupSlice   = "/"
	// This PID is only used when creating a slice for an existing systemd process.
	// When creating a "general slice" that will be used as a parent slice for docker
	// containers, then we use a dummy PID of -1.
	// see https://github.com/containerd/cgroups/blob/1df78138f1e1e6ee593db155c6b369466f577651/v2/manager.go#L732-L735
	generalSlicePID int = -1
)

// controlv2 is used to implement the cgroup Control interface
type controlv2 struct{}

// Create creates a new cgroup based off the spec post validation
func (c *controlv2) Create(cgroupSpec *Spec) error {
	// Validate incoming spec
	err := validateCgroupSpec(cgroupSpec)
	if err != nil {
		return fmt.Errorf("cgroupv2 create: failed to validate spec: %w", err)
	}

	cgroupPath := cgroupSpec.Root
	seelog.Infof("Creating cgroup cgroupv2root=%s parentSlice=%s cgroupPath=%s", defaultCgroupv2Path, parentCgroupSlice, cgroupPath)

	m, err := cgroupsv2.NewSystemd(parentCgroupSlice, cgroupPath, generalSlicePID, cgroupsv2.ToResources(cgroupSpec.Specs))
	if err != nil {
		return fmt.Errorf("cgroupv2 create: unable to create v2 manager: %w", err)
	}

	if err := initializeControllers(m); err != nil {
		return fmt.Errorf("cgroupv2 create: unable initialize cgroup controllers: %w", err)
	}

	return nil
}

// Remove is used to delete the cgroup
func (c *controlv2) Remove(cgroupPath string) error {
	seelog.Infof("Removing cgroup cgroupv2root=%s parentSlice=%s cgroupPath=%s", defaultCgroupv2Path, parentCgroupSlice, cgroupPath)

	m, err := cgroupsv2.LoadSystemd(parentCgroupSlice, cgroupPath)
	if err != nil {
		return fmt.Errorf("cgroupv2 remove: error loading systemd cgroup: %w", err)
	}
	err = m.DeleteSystemd()
	if err != nil {
		return fmt.Errorf("cgroupv2 remove: error deleting systemd cgroup: %w", err)
	}
	return nil
}

// Exists is used to verify the existence of a cgroup
func (c *controlv2) Exists(cgroupPath string) bool {
	fullCgroupPath := fullCgroupPath(cgroupPath)
	seelog.Infof("Checking existence of cgroup cgroupv2root=%s parentSlice=%s cgroupPath=%s fullPath=%s", defaultCgroupv2Path, parentCgroupSlice, cgroupPath, fullCgroupPath)

	_, err := os.Stat(fullCgroupPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		seelog.Errorf("error checking if cgroup exists err=%s", err)
		return false
	}
	return true
}

// Init is used to setup the cgroup root for ecs
func (c *controlv2) Init() error {
	// Load the "root" cgroup and verify cpu and memory cgroup controllers are available.
	m, err := cgroupsv2.LoadSystemd("", "")
	if err != nil {
		return fmt.Errorf("cgroupv2 init: unable to load root cgroup: %w", err)
	}

	if err := initializeControllers(m); err != nil {
		return err
	}

	seelog.Infof("ECS task resource limits cgroupv2 functionality initialized")
	return nil
}

func initializeControllers(manager *cgroupsv2.Manager) error {
	// enable cpu and memory cgroup controllers
	err := manager.ToggleControllers([]string{"cpu", "memory"}, cgroupsv2.Enable)
	if err != nil {
		return fmt.Errorf("cgroupv2 init: error enabling cpu and memory controllers: %w", err)
	}

	// verify that cpu and memory controllers are available
	controllers, err := manager.Controllers()
	if err != nil {
		return fmt.Errorf("cgroupv2 init: unable to get cgroup controllers: %w", err)
	}
	if err := validateController("memory", controllers); err != nil {
		return fmt.Errorf("cgroupv2 init: unable to validate cgroup controllers: %w", err)
	}
	if err := validateController("cpu", controllers); err != nil {
		return fmt.Errorf("cgroupv2 init: unable to validate cgroup controllers: %w", err)
	}
	return nil
}

func validateController(controller string, controllers []string) error {
	for _, v := range controllers {
		if controller == v {
			return nil
		}
	}
	return fmt.Errorf("unable to validate cgroup controllers, did not find %s controller in list of controllers=%v", controller, controllers)
}

// fullCgroupPath returns the full path on disk to a task cgroup slice.
// example: /sys/fs/cgroup/ecstasks.slice/ecstasks-529630467358463ab6bbba4e73afe704.slice
func fullCgroupPath(cgroupPath string) string {
	return filepath.Join(defaultCgroupv2Path, parentCgroupSlice, config.DefaultTaskCgroupV2Prefix+".slice", cgroupPath)
}
