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
	"encoding/json"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	// TaskScope indicates that the volume is created and deleted with task
	TaskScope = "task"
	// SharedScope indicates that the volume's lifecycle is outside the scope of task
	SharedScope = "shared"
	// DockerLocalVolumeDriver is the name of the docker default volume driver
	DockerLocalVolumeDriver   = "local"
	resourceProvisioningError = "VolumeError: Agent could not create task's volume resources"
)

// VolumeResource represents volume resource
type VolumeResource struct {
	// Name is the name of the docker volume
	Name string
	// VolumeConfig contains docker specific volume fields
	VolumeConfig        DockerVolumeConfig
	createdAtUnsafe     time.Time
	desiredStatusUnsafe resourcestatus.ResourceStatus
	knownStatusUnsafe   resourcestatus.ResourceStatus
	// appliedStatusUnsafe is the status that has been "applied" (e.g., we've called some
	// operation such as 'Create' on the resource) but we don't yet know that the
	// application was successful, which may then change the known status. This is
	// used while progressing resource states in progressTask() of task manager
	appliedStatusUnsafe resourcestatus.ResourceStatus
	statusToTransitions map[resourcestatus.ResourceStatus]func() error
	client              dockerapi.DockerClient
	ctx                 context.Context

	// terminalReason should be set for resource creation failures. This ensures
	// the resource object carries some context for why provisoning failed.
	terminalReason     string
	terminalReasonOnce sync.Once

	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex

	// log is a custom logger with extra context specific to the volume resource struct
	log seelog.LoggerInterface
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
	// DockerVolumeName is internal docker name for this volume.
	DockerVolumeName string `json:"dockerVolumeName"`
}

// EFSVolumeConfig represents efs volume configuration.
type EFSVolumeConfig struct {
	FileSystemID  string `json:"fileSystemId"`
	RootDirectory string `json:"rootDirectory"`
	// DockerVolumeName is internal docker name for this volume.
	DockerVolumeName string `json:"dockerVolumeName"`
}

// NewVolumeResource returns a docker volume wrapper object
func NewVolumeResource(ctx context.Context,
	name string,
	dockerVolumeName string,
	scope string,
	autoprovision bool,
	driver string,
	driverOptions map[string]string,
	labels map[string]string,
	client dockerapi.DockerClient) (*VolumeResource, error) {

	if scope == TaskScope && autoprovision {
		return nil, errors.Errorf("volume [%s] : task scoped volume could not be autoprovisioned", name)
	}

	v := &VolumeResource{
		Name: name,
		VolumeConfig: DockerVolumeConfig{
			Scope:            scope,
			Autoprovision:    autoprovision,
			Driver:           driver,
			DriverOpts:       driverOptions,
			Labels:           labels,
			DockerVolumeName: dockerVolumeName,
		},
		client: client,
		ctx:    ctx,
	}
	v.initStatusToTransitions()
	v.initLog()
	return v, nil
}

func (vol *VolumeResource) initLog() {
	if vol.log == nil {
		vol.log = logger.InitLogger()
		vol.log.SetContext(map[string]string{
			"volumeName":       vol.Name,
			"dockerVolumeName": vol.VolumeConfig.DockerVolumeName,
			"dockerScope":      vol.VolumeConfig.Scope,
			"resourceName":     "volume",
		})
	}
}

func (vol *VolumeResource) Initialize(resourceFields *taskresource.ResourceFields,
	taskKnownStatus status.TaskStatus,
	taskDesiredStatus status.TaskStatus) {

	vol.ctx = resourceFields.Ctx
	vol.client = resourceFields.DockerClient
	vol.initStatusToTransitions()
}

func (vol *VolumeResource) initStatusToTransitions() {
	statusToTransitions := map[resourcestatus.ResourceStatus]func() error{
		resourcestatus.ResourceStatus(VolumeCreated): vol.Create,
	}

	vol.statusToTransitions = statusToTransitions
}

// Source returns the name of the volume resource which is used as the source of the volume mount
func (cfg *EFSVolumeConfig) Source() string {
	return cfg.DockerVolumeName
}

// Source returns the name of the volume resource which is used as the source of the volume mount
func (cfg *DockerVolumeConfig) Source() string {
	return cfg.DockerVolumeName
}

// GetName returns the name of the volume resource
func (vol *VolumeResource) GetName() string {
	return vol.Name
}

// DesiredTerminal returns true if the cgroup's desired status is REMOVED
func (vol *VolumeResource) DesiredTerminal() bool {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.desiredStatusUnsafe == resourcestatus.ResourceStatus(VolumeRemoved)
}

// GetTerminalReason returns an error string to propagate up through to task
// state change messages
func (vol *VolumeResource) GetTerminalReason() string {
	if vol.terminalReason == "" {
		return resourceProvisioningError
	}
	return vol.terminalReason
}

func (vol *VolumeResource) setTerminalReason(reason string) {
	vol.terminalReasonOnce.Do(func() {
		vol.log.Infof("setting terminal reason [%s] for volume resource", reason)
		vol.terminalReason = reason
	})
}

// SetDesiredStatus safely sets the desired status of the resource
func (vol *VolumeResource) SetDesiredStatus(status resourcestatus.ResourceStatus) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.desiredStatusUnsafe = status
}

// GetDesiredStatus safely returns the desired status of the task
func (vol *VolumeResource) GetDesiredStatus() resourcestatus.ResourceStatus {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.desiredStatusUnsafe
}

// SetKnownStatus safely sets the currently known status of the resource
func (vol *VolumeResource) SetKnownStatus(status resourcestatus.ResourceStatus) {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	vol.knownStatusUnsafe = status
}

// GetKnownStatus safely returns the currently known status of the task
func (vol *VolumeResource) GetKnownStatus() resourcestatus.ResourceStatus {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.knownStatusUnsafe
}

// KnownCreated returns true if the volume's known status is CREATED
func (vol *VolumeResource) KnownCreated() bool {
	vol.lock.RLock()
	defer vol.lock.RUnlock()

	return vol.knownStatusUnsafe == resourcestatus.ResourceStatus(VolumeCreated)
}

// TerminalStatus returns the last transition state of volume
func (vol *VolumeResource) TerminalStatus() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(VolumeRemoved)
}

// NextKnownState returns the state that the resource should
// progress to based on its `KnownState`.
func (vol *VolumeResource) NextKnownState() resourcestatus.ResourceStatus {
	return vol.GetKnownStatus() + 1
}

// SteadyState returns the transition state of the resource defined as "ready"
func (vol *VolumeResource) SteadyState() resourcestatus.ResourceStatus {
	return resourcestatus.ResourceStatus(VolumeCreated)
}

// ApplyTransition calls the function required to move to the specified status
func (vol *VolumeResource) ApplyTransition(nextState resourcestatus.ResourceStatus) error {
	transitionFunc, ok := vol.statusToTransitions[nextState]
	if !ok {
		errW := errors.Errorf("volume [%s]: transition to %s impossible", vol.Name,
			vol.StatusString(nextState))
		vol.setTerminalReason(errW.Error())
		return errW
	}
	return transitionFunc()
}

// SetAppliedStatus sets the applied status of resource and returns whether
// the resource is already in a transition
func (vol *VolumeResource) SetAppliedStatus(status resourcestatus.ResourceStatus) bool {
	vol.lock.Lock()
	defer vol.lock.Unlock()

	if vol.appliedStatusUnsafe != resourcestatus.ResourceStatus(VolumeStatusNone) {
		// return false to indicate the set operation failed
		return false
	}

	vol.appliedStatusUnsafe = status
	return true
}

// StatusString returns the string of the cgroup resource status
func (vol *VolumeResource) StatusString(status resourcestatus.ResourceStatus) string {
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
	vol.log.Infof("Creating volume using driver %s", vol.VolumeConfig.Driver)
	volumeResponse := vol.client.CreateVolume(
		vol.ctx,
		vol.VolumeConfig.DockerVolumeName,
		vol.VolumeConfig.Driver,
		vol.VolumeConfig.DriverOpts,
		vol.VolumeConfig.Labels,
		dockerclient.CreateVolumeTimeout)

	if volumeResponse.Error != nil {
		vol.setTerminalReason(volumeResponse.Error.Error())
		return volumeResponse.Error
	}

	// set readonly field after creation
	vol.setMountPoint(volumeResponse.DockerVolume.Name)
	return nil
}

// Cleanup performs resource cleanup
func (vol *VolumeResource) Cleanup() error {
	// Enable volume clean up if it's task scoped
	if vol.VolumeConfig.Scope != TaskScope {
		vol.log.Infof("Volume is shared, not removing")
		return nil
	}

	vol.log.Infof("Removing volume")
	err := vol.client.RemoveVolume(vol.ctx, vol.VolumeConfig.DockerVolumeName, dockerclient.RemoveVolumeTimeout)

	if err != nil {
		vol.setTerminalReason(err.Error())
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
	vol.VolumeConfig = temp.VolumeConfig
	if temp.DesiredStatus != nil {
		vol.SetDesiredStatus(resourcestatus.ResourceStatus(*temp.DesiredStatus))
	}
	if temp.KnownStatus != nil {
		vol.SetKnownStatus(resourcestatus.ResourceStatus(*temp.KnownStatus))
	}
	vol.initLog()
	return nil
}
