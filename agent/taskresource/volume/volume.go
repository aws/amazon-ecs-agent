// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package volume

import (
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/cihub/seelog"

	"encoding/json"
	"sync"
	"time"
)

// VolumeResource represents docker volume resource
type VolumeResource struct {
	Name string
	// mountpoint is a read-only field returned from docker
	mountpoint          string
	Driver              string
	DriverOpts          map[string]string
	Labels              map[string]string
	createdAtUnsafe     time.Time
	desiredStatusUnsafe VolumeStatus
	knownStatusUnsafe   VolumeStatus
	client              dockerapi.DockerClient
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// NewVolumeResource returns a docker volume wrapper object
func NewVolumeResource(name string,
	driver string,
	driverOptions map[string]string,
	labels map[string]string,
	client dockerapi.DockerClient) *VolumeResource {

	return &VolumeResource{
		Name:       name,
		Driver:     driver,
		DriverOpts: driverOptions,
		Labels:     labels,
		client:     client,
	}
}

// SetDesiredStatus safely sets the desired status of the resource
func (vol *VolumeResource) SetDesiredStatus(status VolumeStatus) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (vol *VolumeResource) GetDesiredStatus() VolumeStatus {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.desiredStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource
func (vol *VolumeResource) SetKnownStatus(status VolumeStatus) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.knownStatusUnsafe = status
}

// GetKnownStatus safely returns the currently known status of the task
func (vol *VolumeResource) GetKnownStatus() VolumeStatus {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.knownStatusUnsafe
}

// SetCreatedAt sets the timestamp for resource's creation time
func (vol *VolumeResource) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.createdAtUnsafe = createdAt
}

// GetCreatedAt sets the timestamp for resource's creation time
func (vol *VolumeResource) GetCreatedAt() time.Time {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.createdAtUnsafe
}

// setMountPoint sets the mountpoint of the created volume.
// This is a read-only field, hence making this a private function.
func (vol *VolumeResource) setMountPoint(mountPoint string) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.mountpoint = mountPoint
}

// GetMountPoint gets the mountpoint of the created volume.
func (vol *VolumeResource) GetMountPoint() string {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.mountpoint
}

// Create performs resource creation
func (vol *VolumeResource) Create() error {
	seelog.Debugf("Creating volume with name %s using driver %s", vol.Name, vol.Driver)
	volumeResponse := vol.client.CreateVolume(
		vol.Name,
		vol.Driver,
		vol.DriverOpts,
		vol.Labels,
		dockerapi.CreateVolumeTimeout)

	if volumeResponse.Error != nil {
		return volumeResponse.Error
	}

	// set readonly field after creation
	vol.setMountPoint(volumeResponse.DockerVolume.Mountpoint)
	return nil
}

// Cleanup performs resource cleanup
func (vol *VolumeResource) Cleanup() error {
	seelog.Debugf("Removing volume with name %s", vol.Name)
	err := vol.client.RemoveVolume(vol.Name, dockerapi.RemoveVolumeTimeout)

	if err != nil {
		return err
	}
	return nil
}

// volumeResourceJSON duplicates VolumeResource fields, only for marshalling and unmarshalling purposes
type volumeResourceJSON struct {
	Name          string            `json:"Name"`
	Mountpoint    string            `json:"MountPoint"`
	Driver        string            `json:"Driver"`
	DriverOpts    map[string]string `json:"DriverOpts"`
	Labels        map[string]string `json:"Labels"`
	CreatedAt     time.Time
	DesiredStatus *VolumeStatus `json:"DesiredStatus"`
	KnownStatus   *VolumeStatus `json:"KnownStatus"`
}

// MarshalJSON marshals VolumeResource object using duplicate struct VolumeResourceJSON
func (vol *VolumeResource) MarshalJSON() ([]byte, error) {
	if vol == nil {
		return nil, nil
	}
	return json.Marshal(volumeResourceJSON{
		vol.Name,
		vol.mountpoint,
		vol.Driver,
		vol.DriverOpts,
		vol.Labels,
		vol.GetCreatedAt(),
		func() *VolumeStatus { desiredState := vol.GetDesiredStatus(); return &desiredState }(),
		func() *VolumeStatus { knownState := vol.GetKnownStatus(); return &knownState }(),
	})
}

// UnmarshalJSON unmarshals VolumeResource object using duplicate struct VolumeResourceJSON
func (vol *VolumeResource) UnmarshalJSON(b []byte) error {
	temp := &volumeResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	vol.Name = temp.Name
	vol.mountpoint = temp.Mountpoint
	vol.Driver = temp.Driver
	vol.Labels = temp.Labels
	if temp.DesiredStatus != nil {
		vol.SetDesiredStatus(*temp.DesiredStatus)
	}
	if temp.KnownStatus != nil {
		vol.SetKnownStatus(*temp.KnownStatus)
	}
	return nil
}
