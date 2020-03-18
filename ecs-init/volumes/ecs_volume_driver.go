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

	"github.com/cihub/seelog"
)

// TODO: might be a good idea to check the mount point with a tool like mountpoint instead of matching error message.
const notMountedErrMsg = "not mounted"

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
func (e *ECSVolumeDriver) Setup(name string, v *Volume) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if _, ok := e.volumeMounts[name]; ok {
		seelog.Warnf("Volume %s mount already exists", name)
	}
	mnt := setOptions(v.Options)

	mnt.Target = v.Path
	e.volumeMounts[name] = mnt
}

// Create implements ECSVolumeDriver's Create volume method
func (e *ECSVolumeDriver) Create(r *CreateRequest) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	if _, ok := e.volumeMounts[r.Name]; ok {
		return fmt.Errorf("volume already exists")
	}

	mnt := setOptions(r.Options)
	mnt.Target = r.Path

	seelog.Infof("Validating create options for volume %s", r.Name)
	if err := mnt.Validate(); err != nil {
		return err
	}

	seelog.Infof("Mounting volume %s of type %s at path %s", r.Name, mnt.MountType, mnt.Target)
	err := mnt.Mount()
	if err != nil {
		return fmt.Errorf("mounting volume failed: %v", err)
	}
	e.volumeMounts[r.Name] = mnt
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

// Remove implements ECSVolumeDriver's Remove volume method
func (e *ECSVolumeDriver) Remove(req *RemoveRequest) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	mnt, ok := e.volumeMounts[req.Name]
	if !ok {
		return fmt.Errorf("volume not found")
	}
	err := mnt.Unmount()
	if err != nil {
		if strings.Contains(err.Error(), notMountedErrMsg) {
			seelog.Infof("Unmounting volume %s failed because it's not mounted.", req.Name)
			delete(e.volumeMounts, req.Name)
			return nil
		}
		return fmt.Errorf("unmounting volume failed: %v", err)
	}
	delete(e.volumeMounts, req.Name)
	seelog.Infof("Unmounted volume %s successfully.", req.Name)
	return err
}
