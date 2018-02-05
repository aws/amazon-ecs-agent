// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package cgroup

import (
	"fmt"
	"github.com/containerd/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"path/filepath"
	"strings"
)

// Spec captures the abstraction for a creating a new
// cgroup based on root and the runtime specifications
type Spec struct {
	// Root is the cgroup path
	Root string
	// Specs are for all the linux resources including cpu, memory, etc...
	Specs *specs.LinuxResources
}

type Control interface {
	Create(cgroupSpec *Spec) (cgroups.Cgroup, error)
	Remove(cgroupPath string) error
	Exists(cgroupPath string) bool
	Init(cgroupRoot string) error
}

type cgrpdrv int

const (
	CgroupFS cgrpdrv = iota // default
	SystemD
)

const (
	// Default cgroup prefix for ECS tasks when using Cgroup Driver cgroupfs
	defaultTaskCgroupPrefixCgroupFS = "/ecs"
	// Default cgroup prefix for ECS tasks when using Cgroup Driver systemd
	defaultTaskCgroupPrefixSystemd = "ecs"
)

func NewCgroupDriver(driver string) CgroupDriver {
	if driver == "systemd" {
		return SystemD
	}
	return CgroupFS
}

type CgroupDriver interface {
	Root() string
	TaskCgroupRoot(string) string
}

func (d cgrpdrv) Root() string {
	if d == SystemD {
		return fmt.Sprintf("%.slice", defaultTaskCgroupPrefixSystemd)
	}
	return defaultTaskCgroupPrefixCgroupFS
}

func (d cgrpdrv) TaskCgroupRoot(taskID string) string {
	if d == SystemD {
		// See https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/resource_management_guide/sec-default_cgroup_hierarchies
		return fmt.Sprintf("%s-%s.slice", defaultTaskCgroupPrefixSystemd, strings.Replace(taskID, "-", "_", -1))
	}
	return filepath.Join(defaultTaskCgroupPrefixCgroupFS, taskID)
}
