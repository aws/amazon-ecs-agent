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
	"strconv"
	"sync"
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

// Create implements ECSVolumeDriver's Create volume method
func (e *ECSVolumeDriver) Create(r *CreateRequest) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	if _, ok := e.volumeMounts[r.Name]; ok {
		return fmt.Errorf("volume already exists: %s", r.Name)
	}

	mnt := &MountHelper{}
	var opts string
	for k, v := range r.Options {
		if opts != "" {
			opts += ", "
		}
		opts += k + ":" + v
		switch k {
		case "type":
			mnt.MountType = v
		case "netns":
			pid, _ := strconv.Atoi(v)
			mnt.NetNSPid = pid
		case "o":
			mnt.Options = v
		case "device":
			mnt.Device = v
		}
	}
	mnt.Target = r.Path
	opts += ", target:" + r.Path

	if err := mnt.Validate(); err != nil {
		return err
	}

	err := mnt.Mount()
	if err != nil {
		return err
	}
	e.volumeMounts[r.Name] = mnt
	return nil
}

// Remove implements ECSVolumeDriver's Remove volume method
func (e *ECSVolumeDriver) Remove(req *RemoveRequest) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	mnt, ok := e.volumeMounts[req.Name]
	if !ok {
		return fmt.Errorf("volume %s not found", req.Name)
	}
	err := mnt.Unmount()
	if err != nil {
		return err
	}
	delete(e.volumeMounts, req.Name)
	return err
}
