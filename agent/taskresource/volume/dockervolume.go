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
	"context"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/cihub/seelog"

	"encoding/json"
	"sync"
	"time"
)

// VolumeResource represents volume resource
type VolumeResource struct {
	// Name is the name of docker volume
	Name string
	// VolumeConfig contains docker specific volume fields
	VolumeConfig        DockerVolumeConfig
	createdAtUnsafe     time.Time
	desiredStatusUnsafe VolumeStatus
	knownStatusUnsafe   VolumeStatus
	client              dockerapi.DockerClient
	ctx                 context.Context
	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex
}

// DockerVolumeConfig represents docker volume configuration
// See https://tinyurl.com/zmexdw2
type DockerVolumeConfig struct {
	// Scope represents lifetime of the volume: "task" or "shared"
	Scope string `json:"scope"`
	// Autoprovision is true if agent needs to create the volume,
	// false if it is pre-provisioned outside of ECS
	Autoprovision bool `json:"autoprovision"`
	// Mountpoint is a read-only field returned from docker
	Mountpoint string            `json:"mountPoint"`
	Driver     string            `json:"driver"`
	DriverOpts map[string]string `json:"driverOpts"`
	Labels     map[string]string `json:"labels"`
}

// NewVolumeResource returns a docker volume wrapper object
func NewVolumeResource(name string,
	scope string,
	autoprovision bool,
	driver string,
	driverOptions map[string]string,
	labels map[string]string,
	client dockerapi.DockerClient,
	ctx context.Context) *VolumeResource {

	return &VolumeResource{
		Name: name,
		VolumeConfig: DockerVolumeConfig{
			Scope:         scope,
			Autoprovision: autoprovision,
			Driver:        driver,
			DriverOpts:    driverOptions,
			Labels:        labels,
		},
		client: client,
		ctx:    ctx,
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

	vol.VolumeConfig.Mountpoint = mountPoint
}

// GetMountPoint gets the mountpoint of the created volume.
func (vol *VolumeResource) GetMountPoint() string {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.VolumeConfig.Mountpoint
}

// Create performs resource creation
func (vol *VolumeResource) Create() error {
	seelog.Debugf("Creating volume with name %s using driver %s", vol.Name, vol.VolumeConfig.Driver)
	volumeResponse := vol.client.CreateVolume(
		vol.ctx,
		vol.Name,
		vol.VolumeConfig.Driver,
		vol.VolumeConfig.DriverOpts,
		vol.VolumeConfig.Labels,
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
	err := vol.client.RemoveVolume(vol.ctx, vol.Name, dockerapi.RemoveVolumeTimeout)

	if err != nil {
		return err
	}
	return nil
}

// volumeResourceJSON duplicates VolumeResource fields, only for marshalling and unmarshalling purposes
type volumeResourceJSON struct {
	Name          string             `json:"name"`
	VolumeConfig  DockerVolumeConfig `json:"dockerVolumeConfiguration"`
	CreatedAt     time.Time          `json:"createdAt"`
	DesiredStatus *VolumeStatus      `json:"desiredStatus"`
	KnownStatus   *VolumeStatus      `json:"knownStatus"`
}

// MarshalJSON marshals VolumeResource object using duplicate struct VolumeResourceJSON
func (vol *VolumeResource) MarshalJSON() ([]byte, error) {
	if vol == nil {
		return nil, nil
	}
	return json.Marshal(volumeResourceJSON{
		vol.Name,
		vol.VolumeConfig,
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
	vol.VolumeConfig = temp.VolumeConfig
	if temp.DesiredStatus != nil {
		vol.SetDesiredStatus(*temp.DesiredStatus)
	}
	if temp.KnownStatus != nil {
		vol.SetKnownStatus(*temp.KnownStatus)
	}
	return nil
}
