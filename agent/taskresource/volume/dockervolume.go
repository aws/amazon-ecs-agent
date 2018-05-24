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
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"encoding/json"
	"sync"
	"time"
)

const (
	//TaskScope indicates that the volume is created and deleted with task
	TaskScope = "task"
	//SharedScope indicates that the volume's lifecycle is outside the scope of task
	SharedScope = "shared"
)

// VolumeResources represents volume resource
type VolumeResource struct {
	// Name is the name of the docker volume
	Name string
	// dockerVolumeName is internal docker name for this volume.
	// Only the LocalVolume type will have dockerVolumeName different from the name above.
	DockerVolumeName string
	// VolumeConfig contains docker specific volume fields
	VolumeConfig        DockerVolumeConfig
	createdAtUnsafe     time.Time
	desiredStatusUnsafe taskresource.ResourceStatus
	knownStatusUnsafe   taskresource.ResourceStatus
	// appliedStatus is the status that has been "applied" (e.g., we've called some
	// operation such as 'Create' on the resource) but we don't yet know that the
	// application was successful, which may then change the known status. This is
	// used while progressing resource states in progressTask() of task manager
	appliedStatus                      taskresource.ResourceStatus
	resourceStatusToTransitionFunction map[taskresource.ResourceStatus]func() error
	client                             dockerapi.DockerClient
	ctx                                context.Context
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
	dockerVolumeName string,
	scope string,
	autoprovision bool,
	driver string,
	driverOptions map[string]string,
	labels map[string]string,
	client dockerapi.DockerClient,
	ctx context.Context) *VolumeResource {

	v := &VolumeResource{
		Name:             name,
		DockerVolumeName: dockerVolumeName,
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
	v.initializeResourceStatusToTransitionFunction(scope)
	return v
}

func (vol *VolumeResource) Initialize(resourceFields taskresource.ResourceFields) {
	vol.ctx = resourceFields.Context
	vol.client = resourceFields.DockerClient
}

func (vol *VolumeResource) initializeResourceStatusToTransitionFunction(scope string) {
	resourceStatusToTransitionFunction := map[taskresource.ResourceStatus]func() error{
		taskresource.ResourceStatus(VolumeCreated): vol.Create,
	}

	// Enable volume clean up if it's task scoped
	if scope == TaskScope {
		resourceStatusToTransitionFunction[taskresource.ResourceStatus(VolumeRemoved)] = vol.Cleanup
	}

	vol.resourceStatusToTransitionFunction = resourceStatusToTransitionFunction
}

// GetName returns the name of the volume resource
func (vol *VolumeResource) GetName() string {
	return vol.Name
}

// DesiredTerminal returns true if the cgroup's desired status is REMOVED
func (vol *VolumeResource) DesiredTerminal() bool {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.desiredStatusUnsafe == taskresource.ResourceStatus(VolumeRemoved)
}

// SetDesiredStatus safely sets the desired status of the resource
func (vol *VolumeResource) SetDesiredStatus(status taskresource.ResourceStatus) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (vol *VolumeResource) GetDesiredStatus() taskresource.ResourceStatus {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.desiredStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource
func (vol *VolumeResource) SetKnownStatus(status taskresource.ResourceStatus) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.knownStatusUnsafe = status
}

// GetKnownStatus safely returns the currently known status of the task
func (vol *VolumeResource) GetKnownStatus() taskresource.ResourceStatus {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.knownStatusUnsafe
}

// KnownCreated returns true if the volume's known status is CREATED
func (vol *VolumeResource) KnownCreated() bool {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.knownStatusUnsafe == taskresource.ResourceStatus(VolumeCreated)
}

// TerminalStatus returns the last transition state of volume
func (vol *VolumeResource) TerminalStatus() taskresource.ResourceStatus {
	return taskresource.ResourceStatus(VolumeRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (vol *VolumeResource) NextKnownState() taskresource.ResourceStatus {
	return vol.GetKnownStatus() + 1
}

// SteadyState returns the transition state of the resource defined as "ready"
func (vol *VolumeResource) SteadyState() taskresource.ResourceStatus {
	return taskresource.ResourceStatus(VolumeCreated)
}

// ApplyTransition calls the function required to move to the specified status
func (vol *VolumeResource) ApplyTransition(nextState taskresource.ResourceStatus) error {
	transitionFunc, ok := vol.resourceStatusToTransitionFunction[nextState]
	if !ok {
		seelog.Errorf("Volume Resource [%s]: unsupported desired state transition: %s",
			vol.Name, vol.StatusString(nextState))
		return errors.Errorf("resource [%s]: transition to %s impossible", vol.Name,
			vol.StatusString(nextState))
	}
	return transitionFunc()
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (vol *VolumeResource) SetAppliedStatus(status taskresource.ResourceStatus) bool {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	if vol.appliedStatus != taskresource.ResourceStatus(VolumeStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	vol.appliedStatus = status
	return true
}

// StatusString returns the string of the cgroup resource status
func (vol *VolumeResource) StatusString(status taskresource.ResourceStatus) string {
	return VolumeStatus(status).String()
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

// SourcePath is fulfilling the HostVolume interface
func (vol *VolumeResource) SourcePath() string {
	return vol.GetMountPoint()
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
	Name             string             `json:"name"`
	DockerVolumeName string             `json:"dockerVolumeName"`
	VolumeConfig     DockerVolumeConfig `json:"dockerVolumeConfiguration"`
	CreatedAt        time.Time          `json:"createdAt"`
	DesiredStatus    *VolumeStatus      `json:"desiredStatus"`
	KnownStatus      *VolumeStatus      `json:"knownStatus"`
}

// MarshalJSON marshals VolumeResource object using duplicate struct VolumeResourceJSON
func (vol *VolumeResource) MarshalJSON() ([]byte, error) {
	if vol == nil {
		return nil, nil
	}
	return json.Marshal(volumeResourceJSON{
		vol.Name,
		vol.DockerVolumeName,
		vol.VolumeConfig,
		vol.GetCreatedAt(),
		func() *VolumeStatus { desiredState := VolumeStatus(vol.GetDesiredStatus()); return &desiredState }(),
		func() *VolumeStatus { knownState := VolumeStatus(vol.GetKnownStatus()); return &knownState }(),
	})
}

// UnmarshalJSON unmarshals VolumeResource object using duplicate struct VolumeResourceJSON
func (vol *VolumeResource) UnmarshalJSON(b []byte) error {
	temp := &volumeResourceJSON{}

	if err := json.Unmarshal(b, &temp); err != nil {
		return err
	}

	vol.Name = temp.Name
	if temp.DockerVolumeName != "" {
		vol.DockerVolumeName = temp.DockerVolumeName
	} else {
		vol.DockerVolumeName = temp.Name
	}
	vol.VolumeConfig = temp.VolumeConfig
	if temp.DesiredStatus != nil {
		vol.SetDesiredStatus(taskresource.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		vol.SetKnownStatus(taskresource.ResourceStatus(*temp.KnownStatus))
	}
	return nil
}
