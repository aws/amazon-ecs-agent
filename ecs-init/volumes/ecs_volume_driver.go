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
	"fmt"
	"strings"
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-init/volumes/driver"
	"github.com/aws/amazon-ecs-agent/ecs-init/volumes/types"
	"github.com/cihub/seelog"
)

const (
	// TODO: might be a good idea to check the mount point with a tool like mountpoint instead of matching error message.
	notMountedErrMsg = "not mounted"

	// This error is returned when volume is already unmounted
	// Example -
	// $ sudo umount -l abc
	// umount: abc: no mount point specified.
	noMountPointSpecifiedErrMsg = "no mount point specified"
)

// ECSVolumeDriver holds mount helper and methods for different Volume Mounts
type ECSVolumeDriver struct {
	volumeMounts map[string]*MountHelper
	lock         sync.RWMutex
}

// NewECSVolumeDriver initializes fields for volume mounts
func NewECSVolumeDriver() *ECSVolumeDriver {
	return &ECSVolumeDriver{
		volumeMounts: make(map[string]*MountHelper),
	}
}

// Setup creates the mount helper
func (e *ECSVolumeDriver) Setup(name string, v *types.Volume) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if _, ok := e.volumeMounts[name]; ok {
		seelog.Warnf("Volume %s mount already exists", name)
	}
	mnt := setOptions(v.Options)

	mnt.Target = v.Path
	e.volumeMounts[name] = mnt
}

// Create implements ECSVolumeDriver's Create volume method.
// The lock is held only for map access and validation; the actual mount I/O
// proceeds without the driver lock since the caller holds a per-volume lock.
func (e *ECSVolumeDriver) Create(r *driver.CreateRequest) error {
	mnt := setOptions(r.Options)
	mnt.Target = r.Path

	seelog.Infof("Validating create options for volume %s", r.Name)
	if err := mnt.Validate(); err != nil {
		return err
	}

	// Check for duplicates under read lock
	e.lock.RLock()
	if _, exists := e.volumeMounts[r.Name]; exists {
		e.lock.RUnlock()
		return fmt.Errorf("volume %s already mounted", r.Name)
	}
	e.lock.RUnlock()

	// Perform the mount I/O without holding the driver lock
	seelog.Infof("Mounting volume %s of type %s at path %s", r.Name, mnt.MountType, mnt.Target)
	err := mnt.Mount()
	if err != nil {
		return fmt.Errorf("mounting volume failed: %v", err)
	}

	// Register the mount under write lock
	e.lock.Lock()
	e.volumeMounts[r.Name] = mnt
	e.lock.Unlock()

	return nil
}

func setOptions(options map[string]string) *MountHelper {
	mnt := &MountHelper{}
	for k, v := range options {
		switch k {
		case "type":
			mnt.MountType = v
		case "o":
			mnt.Options = v
		case "device":
			mnt.Device = v
		}
	}
	return mnt
}

// Remove implements ECSVolumeDriver's Remove volume method.
// The unmount I/O proceeds without the driver lock since the caller holds a per-volume lock.
func (e *ECSVolumeDriver) Remove(req *driver.RemoveRequest) error {
	e.lock.RLock()
	mnt, ok := e.volumeMounts[req.Name]
	e.lock.RUnlock()

	if !ok {
		return fmt.Errorf("volume not found")
	}

	// Perform the unmount I/O without holding the driver lock
	err := mnt.Unmount()
	if err != nil {
		if strings.Contains(err.Error(), notMountedErrMsg) ||
			strings.Contains(err.Error(), noMountPointSpecifiedErrMsg) {
			seelog.Infof("Unmounting volume %s failed because it's not mounted.", req.Name)
			e.lock.Lock()
			delete(e.volumeMounts, req.Name)
			e.lock.Unlock()
			return nil
		}
		return fmt.Errorf("unmounting volume failed: %v", err)
	}

	e.lock.Lock()
	delete(e.volumeMounts, req.Name)
	e.lock.Unlock()

	seelog.Infof("Unmounted volume %s successfully.", req.Name)
	return nil
}

// Method to check if a volume is currently mounted.
func (e *ECSVolumeDriver) IsMounted(volumeName string) bool {
	e.lock.RLock()
	defer e.lock.RUnlock()
	_, exists := e.volumeMounts[volumeName]
	return exists
}
