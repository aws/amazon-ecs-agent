// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package volumes

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	// MountBinary is the binary name of EFS Mount
	MountBinary = "mount"
	// UnmountBinary is the binary name for EFS Unmount
	UnmountBinary = "umount"
)

// MountHelper contains fields and methods for mounting and unmounting EFS volumes
type MountHelper struct {
	MountType string
	Device    string
	Target    string
	Options   string
}

// Mount helps mount EFS volumes
func (m *MountHelper) Mount() error {
	args := []string{}
	if m.MountType != "" {
		args = append(args, "-t", m.MountType)
	}
	if m.Options != "" {
		args = append(args, "-o", m.Options)
	}
	args = append(args, m.Device, m.Target)
	if err := m.Validate(); err != nil {
		return err
	}
	return runMount(args)
}

var runMount = runMountCommand

func runMountCommand(args []string) error {
	mountcmd := exec.Command(MountBinary, args...)
	return runCmd(mountcmd)
}

// Validate validates fields as part of the mount command
func (m *MountHelper) Validate() error {
	requiredFields := []string{}
	if m.Device == "" {
		requiredFields = append(requiredFields, "device")
	}
	if m.Target == "" {
		requiredFields = append(requiredFields, "target")
	}
	if len(requiredFields) > 0 {
		return fmt.Errorf("missing required fields: [%s]", strings.Join(requiredFields, ","))
	}
	return nil
}

// Unmount helps unmount EFS volumes
func (m *MountHelper) Unmount() error {
	path, err := lookPath(UnmountBinary)
	if err != nil {
		return err
	}
	return runUnmount(path, m.Target)
}

var lookPath = getPath

func getPath(binary string) (string, error) {
	return exec.LookPath(binary)
}

var runUnmount = runUnmountCommand

func runUnmountCommand(path string, target string) error {
	// In case of awsvpc network mode, when we unmount the volume, task network namespace has been deleted
	// and nfs server is no longer reachable, so umount will hang. Hence doing lazy unmount here.
	umountCmd := exec.Command(path, "-l", target)
	return runCmd(umountCmd)
}

var runCmd = runCommand

func runCommand(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return err
	}
	return errors.New(stderr.String())
}
