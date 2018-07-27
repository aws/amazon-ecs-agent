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

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
	"github.com/cihub/seelog"
	"github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

const (
	// PauseContainerName is the internal name for the pause container
	PauseContainerName = "~internal~ecs~pause"

	emptyHostVolumeName = "~internal~ecs-emptyvolume-source"

	// awsSDKCredentialsRelativeURIPathEnvironmentVariableName defines the name of the environment
	// variable containers' config, which will be used by the AWS SDK to fetch
	// credentials.
	awsSDKCredentialsRelativeURIPathEnvironmentVariableName = "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"

	arnResourceSections  = 2
	arnResourceDelimiter = "/"
	// networkModeNone specifies the string used to define the `none` docker networking mode
	networkModeNone = "none"
	// networkModeContainerPrefix specifies the prefix string used for setting the
	// container's network mode to be mapped to that of another existing container
	networkModeContainerPrefix = "container:"

	// awslogsCredsEndpointOpt is the awslogs option that is used to pass in an
	// http endpoint for authentication
	awslogsCredsEndpointOpt = "awslogs-credentials-endpoint"
)

// TaskOverrides are the overrides applied to a task
type TaskOverrides struct{}

// Task is the internal representation of a task in the ECS agent
type Task struct {
	// Arn is the unique identifier for the task
	Arn string
	// Overrides are the overrides applied to a task
	Overrides TaskOverrides `json:"-"`
	// Family is the name of the task definition family
	Family string
	// Version is the version of the task definition
	Version string
	// Containers are the containers for the task
	Containers []*apicontainer.Container
	// ResourcesMapUnsafe is the map of resource type to corresponding resources
	ResourcesMapUnsafe resourcetype.ResourcesMap `json:"resources"`
	// Volumes are the volumes for the task
	Volumes []TaskVolume `json:"volumes"`
	// CPU is a task-level limit for compute resources. A value of 1 means that
	// the task may access 100% of 1 vCPU on the instance
	CPU float64 `json:"Cpu,omitempty"`
	// Memory is a task-level limit for memory resources in bytes
	Memory int64 `json:"Memory,omitempty"`
	// DesiredStatusUnsafe represents the state where the task should go. Generally,
	// the desired status is informed by the ECS backend as a result of either
	// API calls made to ECS or decisions made by the ECS service scheduler.
	// The DesiredStatusUnsafe is almost always either apitaskstatus.TaskRunning or apitaskstatus.TaskStopped.
	// NOTE: Do not access DesiredStatusUnsafe directly.  Instead, use `UpdateStatus`,
	// `UpdateDesiredStatus`, `SetDesiredStatus`, and `SetDesiredStatus`.
	// TODO DesiredStatusUnsafe should probably be private with appropriately written
	// setter/getter.  When this is done, we need to ensure that the UnmarshalJSON
	// is handled properly so that the state storage continues to work.
	DesiredStatusUnsafe apitaskstatus.TaskStatus `json:"DesiredStatus"`

	// KnownStatusUnsafe represents the state where the task is.  This is generally
	// the minimum of equivalent status types for the containers in the task;
	// if one container is at ContainerRunning and another is at ContainerPulled,
	// the task KnownStatusUnsafe would be TaskPulled.
	// NOTE: Do not access KnownStatusUnsafe directly.  Instead, use `UpdateStatus`,
	// and `GetKnownStatus`.
	// TODO KnownStatusUnsafe should probably be private with appropriately written
	// setter/getter.  When this is done, we need to ensure that the UnmarshalJSON
	// is handled properly so that the state storage continues to work.
	KnownStatusUnsafe apitaskstatus.TaskStatus `json:"KnownStatus"`
	// KnownStatusTimeUnsafe captures the time when the KnownStatusUnsafe was last updated.
	// NOTE: Do not access KnownStatusTime directly, instead use `GetKnownStatusTime`.
	KnownStatusTimeUnsafe time.Time `json:"KnownTime"`

	// PullStartedAtUnsafe is the timestamp when the task start pulling the first container,
	// it won't be set if the pull never happens
	PullStartedAtUnsafe time.Time `json:"PullStartedAt"`
	// PullStoppedAtUnsafe is the timestamp when the task finished pulling the last container,
	// it won't be set if the pull never happens
	PullStoppedAtUnsafe time.Time `json:"PullStoppedAt"`
	// ExecutionStoppedAtUnsafe is the timestamp when the task desired status moved to stopped,
	// which is when the any of the essential containers stopped
	ExecutionStoppedAtUnsafe time.Time `json:"ExecutionStoppedAt"`

	// SentStatusUnsafe represents the last KnownStatusUnsafe that was sent to the ECS SubmitTaskStateChange API.
	// TODO(samuelkarp) SentStatusUnsafe needs a lock and setters/getters.
	// TODO SentStatusUnsafe should probably be private with appropriately written
	// setter/getter.  When this is done, we need to ensure that the UnmarshalJSON
	// is handled properly so that the state storage continues to work.
	SentStatusUnsafe apitaskstatus.TaskStatus `json:"SentStatus"`

	StartSequenceNumber int64
	StopSequenceNumber  int64

	// ExecutionCredentialsID is the ID of credentials that are used by agent to
	// perform some action at the task level, such as pulling image from ECR
	ExecutionCredentialsID string `json:"executionCredentialsID"`

	// credentialsID is used to set the CredentialsId field for the
	// IAMRoleCredentials object associated with the task. This id can be
	// used to look up the credentials for task in the credentials manager
	credentialsID string

	// ENI is the elastic network interface specified by this task
	ENI *apieni.ENI

	// MemoryCPULimitsEnabled to determine if task supports CPU, memory limits
	MemoryCPULimitsEnabled bool `json:"MemoryCPULimitsEnabled,omitempty"`

	// PlatformFields consists of fields specific to linux/windows for a task
	PlatformFields PlatformFields `json:"PlatformFields,omitempty"`

	// terminalReason should be used when we explicitly move a task to stopped.
	// This ensures the task object carries some context for why it was explicitly
	// stoppped.
	terminalReason     string
	terminalReasonOnce sync.Once

	// lock is for protecting all fields in the task struct
	lock sync.RWMutex
}

// TaskFromACS translates ecsacs.Task to apitask.Task by first marshaling the received
// ecsacs.Task to json and unmarshaling it as apitask.Task
func TaskFromACS(acsTask *ecsacs.Task, envelope *ecsacs.PayloadMessage) (*Task, error) {
	data, err := jsonutil.BuildJSON(acsTask)
	if err != nil {
		return nil, err
	}
	task := &Task{}
	err = json.Unmarshal(data, task)
	if err != nil {
		return nil, err
	}
	if task.GetDesiredStatus() == apitaskstatus.TaskRunning && envelope.SeqNum != nil {
		task.StartSequenceNumber = *envelope.SeqNum
	} else if task.GetDesiredStatus() == apitaskstatus.TaskStopped && envelope.SeqNum != nil {
		task.StopSequenceNumber = *envelope.SeqNum
	}

	// Overrides the container command if it's set
	for _, container := range task.Containers {
		if (container.Overrides != apicontainer.ContainerOverrides{}) && container.Overrides.Command != nil {
			container.Command = *container.Overrides.Command
		}
		container.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	}
	// initialize resources map for task
	task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	return task, nil
}

// PostUnmarshalTask is run after a task has been unmarshalled, but before it has been
// run. It is possible it will be subsequently called after that and should be
// able to handle such an occurrence appropriately (e.g. behave idempotently).
func (task *Task) PostUnmarshalTask(cfg *config.Config,
	credentialsManager credentials.Manager, resourceFields *taskresource.ResourceFields,
	dockerClient dockerapi.DockerClient, ctx context.Context) error {
	// TODO, add rudimentary plugin support and call any plugins that want to
	// hook into this
	task.adjustForPlatform(cfg)
	if task.MemoryCPULimitsEnabled {
		err := task.initializeCgroupResourceSpec(cfg.CgroupPath, resourceFields)
		if err != nil {
			seelog.Errorf("Task [%s]: could not intialize resource: %v", task.Arn, err)
			return apierrors.NewResourceInitError(task.Arn, err)
		}
	}

	if task.requiresASMDockerAuthData() {
		task.initializeASMAuthResource(credentialsManager, resourceFields)
	}

	err := task.initializeDockerLocalVolumes(dockerClient, ctx)
	if err != nil {
		return apierrors.NewResourceInitError(task.Arn, err)
	}
	err = task.initializeDockerVolumes(cfg.SharedVolumeMatchFullConfig, dockerClient, ctx)
	if err != nil {
		return apierrors.NewResourceInitError(task.Arn, err)
	}

	task.initializeCredentialsEndpoint(credentialsManager)
	task.addNetworkResourceProvisioningDependency(cfg)
	return nil
}

func (task *Task) initializeDockerLocalVolumes(dockerClient dockerapi.DockerClient, ctx context.Context) error {
	var requiredLocalVolumes []string
	for _, container := range task.Containers {
		for _, mountPoint := range container.MountPoints {
			vol, ok := task.HostVolumeByName(mountPoint.SourceVolume)
			if !ok {
				continue
			}
			if localVolume, ok := vol.(*taskresourcevolume.LocalDockerVolume); ok {
				localVolume.HostPath = task.volumeName(mountPoint.SourceVolume)
				container.BuildResourceDependency(mountPoint.SourceVolume,
					resourcestatus.ResourceStatus(taskresourcevolume.VolumeCreated),
					apicontainerstatus.ContainerPulled)
				requiredLocalVolumes = append(requiredLocalVolumes, mountPoint.SourceVolume)

			}
		}
	}

	if len(requiredLocalVolumes) == 0 {
		// No need to create the auxiliary local driver volumes
		return nil
	}

	// if we have required local volumes, create one with default local drive
	for _, volumeName := range requiredLocalVolumes {
		vol, _ := task.HostVolumeByName(volumeName)
		// BUG(samuelkarp) On Windows, volumes with names that differ only by case will collide
		scope := taskresourcevolume.TaskScope
		localVolume, err := taskresourcevolume.NewVolumeResource(ctx, volumeName,
			vol.Source(), scope, false,
			taskresourcevolume.DockerLocalVolumeDriver,
			make(map[string]string), make(map[string]string), dockerClient)

		if err != nil {
			return err
		}

		task.AddResource(resourcetype.DockerVolumeKey, localVolume)
	}
	return nil
}

func (task *Task) volumeName(name string) string {
	return "ecs-" + task.Family + "-" + task.Version + "-" + name + "-" + utils.RandHex()
}

// initializeDockerVolumes checks the volume resource in the task to determine if the agent
// should create the volume before creating the container
func (task *Task) initializeDockerVolumes(sharedVolumeMatchFullConfig bool, dockerClient dockerapi.DockerClient, ctx context.Context) error {
	for i, vol := range task.Volumes {
		// No need to do this for non-docker volume, eg: host bind/empty volume
		if vol.Type != DockerVolumeType {
			continue
		}

		dockerVolume, ok := vol.Volume.(*taskresourcevolume.DockerVolumeConfig)
		if !ok {
			return errors.New("task volume: volume configuration does not match the type 'docker'")
		}
		// Agent needs to create task-scoped volume
		if dockerVolume.Scope == taskresourcevolume.TaskScope {
			err := task.addTaskScopedVolumes(ctx, dockerClient, &task.Volumes[i])
			if err != nil {
				return err
			}
		} else {
			// Agent needs to create shared volume if that's auto provisioned
			err := task.addSharedVolumes(sharedVolumeMatchFullConfig, ctx, dockerClient, &task.Volumes[i])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// addTaskScopedVolumes adds the task scoped volume into task resources and updates container dependency
func (task *Task) addTaskScopedVolumes(ctx context.Context, dockerClient dockerapi.DockerClient,
	vol *TaskVolume) error {

	volumeConfig := vol.Volume.(*taskresourcevolume.DockerVolumeConfig)
	volumeResource, err := taskresourcevolume.NewVolumeResource(
		ctx,
		vol.Name,
		task.volumeName(vol.Name),
		volumeConfig.Scope, volumeConfig.Autoprovision,
		volumeConfig.Driver, volumeConfig.DriverOpts,
		volumeConfig.Labels, dockerClient)
	if err != nil {
		return err
	}

	vol.Volume = &volumeResource.VolumeConfig
	task.AddResource(resourcetype.DockerVolumeKey, volumeResource)
	task.updateContainerVolumeDependency(vol.Name)
	return nil
}

// addSharedVolumes adds shared volume into task resources and updates container dependency
func (task *Task) addSharedVolumes(SharedVolumeMatchFullConfig bool, ctx context.Context, dockerClient dockerapi.DockerClient,
	vol *TaskVolume) error {

	volumeConfig := vol.Volume.(*taskresourcevolume.DockerVolumeConfig)
	volumeConfig.DockerVolumeName = vol.Name
	// if autoprovision == true, we will auto-provision the volume if it does not exist already
	// else the named volume must exist
	if !volumeConfig.Autoprovision {
		volumeMetadata := dockerClient.InspectVolume(ctx, vol.Name, dockerapi.InspectVolumeTimeout)
		if volumeMetadata.Error != nil {
			return errors.Wrapf(volumeMetadata.Error, "initialize volume: volume detection failed, volume '%s' does not exist and autoprovision is set to false", vol.Name)
		}
		return nil
	}

	// at this point we know autoprovision = true
	// check if the volume configuration matches the one exists on the instance
	volumeMetadata := dockerClient.InspectVolume(ctx, volumeConfig.DockerVolumeName, dockerapi.InspectVolumeTimeout)
	if volumeMetadata.Error != nil {
		// Inspect the volume timed out, fail the task
		if _, ok := volumeMetadata.Error.(*dockerapi.DockerTimeoutError); ok {
			return volumeMetadata.Error
		}

		seelog.Infof("initialize volume: Task [%s]: non-autoprovisioned volume not found, adding to task resource %q", task.Arn, vol.Name)
		// this resource should be created by agent
		volumeResource, err := taskresourcevolume.NewVolumeResource(
			ctx,
			vol.Name,
			vol.Name,
			volumeConfig.Scope, volumeConfig.Autoprovision,
			volumeConfig.Driver, volumeConfig.DriverOpts,
			volumeConfig.Labels, dockerClient)
		if err != nil {
			return err
		}

		task.AddResource(resourcetype.DockerVolumeKey, volumeResource)
		task.updateContainerVolumeDependency(vol.Name)
		return nil
	}

	seelog.Infof("initialize volume: Task [%s]: volume [%s] already exists", task.Arn, volumeConfig.DockerVolumeName)
	if !SharedVolumeMatchFullConfig {
		seelog.Infof("initialize volume: Task [%s]: ECS_SHARED_VOLUME_MATCH_FULL_CONFIG is set to false and volume with name [%s] is found", task.Arn, volumeConfig.DockerVolumeName)
		return nil
	}

	// validate all the volume metadata fields match to the configuration
	if len(volumeMetadata.DockerVolume.Labels) == 0 && len(volumeMetadata.DockerVolume.Labels) == len(volumeConfig.Labels) {
		seelog.Infof("labels are both empty or null: Task [%s]: volume [%s]", task.Arn, volumeConfig.DockerVolumeName)
	} else if !reflect.DeepEqual(volumeMetadata.DockerVolume.Labels, volumeConfig.Labels) {
		return errors.Errorf("intialize volume: non-autoprovisioned volume does not match existing volume labels: existing: %v, expected: %v",
			volumeMetadata.DockerVolume.Labels, volumeConfig.Labels)
	}

	if len(volumeMetadata.DockerVolume.Options) == 0 && len(volumeMetadata.DockerVolume.Options) == len(volumeConfig.DriverOpts) {
		seelog.Infof("driver options are both empty or null: Task [%s]: volume [%s]", task.Arn, volumeConfig.DockerVolumeName)
	} else if !reflect.DeepEqual(volumeMetadata.DockerVolume.Options, volumeConfig.DriverOpts) {
		return errors.Errorf("initialize volume: non-autoprovisioned volume does not match existing volume options: existing: %v, expected: %v",
			volumeMetadata.DockerVolume.Options, volumeConfig.DriverOpts)
	}
	// Right now we are not adding shared, autoprovision = true volume to task as resource if it already exists (i.e. when this task didn't create the volume).
	// if we need to change that, make a call to task.AddResource here.
	return nil
}

// updateContainerVolumeDependency adds the volume resource to container dependency
func (task *Task) updateContainerVolumeDependency(name string) {
	// Find all the container that depends on the volume
	for _, container := range task.Containers {
		for _, mountpoint := range container.MountPoints {
			if mountpoint.SourceVolume == name {
				container.BuildResourceDependency(name,
					resourcestatus.ResourceCreated,
					apicontainerstatus.ContainerPulled)
			}
		}
	}
}

// initializeCredentialsEndpoint sets the credentials endpoint for all containers in a task if needed.
func (task *Task) initializeCredentialsEndpoint(credentialsManager credentials.Manager) {
	id := task.GetCredentialsID()
	if id == "" {
		// No credentials set for the task. Do not inject the endpoint environment variable.
		return
	}
	taskCredentials, ok := credentialsManager.GetTaskCredentials(id)
	if !ok {
		// Task has credentials id set, but credentials manager is unaware of
		// the id. This should never happen as the payload handler sets
		// credentialsId for the task after adding credentials to the
		// credentials manager
		seelog.Errorf("Unable to get credentials for task: %s", task.Arn)
		return
	}

	credentialsEndpointRelativeURI := taskCredentials.IAMRoleCredentials.GenerateCredentialsEndpointRelativeURI()
	for _, container := range task.Containers {
		// container.Environment map would not be initialized if there are
		// no environment variables to be set or overridden in the container
		// config. Check if that's the case and initilialize if needed
		if container.Environment == nil {
			container.Environment = make(map[string]string)
		}
		container.Environment[awsSDKCredentialsRelativeURIPathEnvironmentVariableName] = credentialsEndpointRelativeURI
	}

}

// requiresASMDockerAuthData returns true if atleast one container in the task
// needs to retrieve private registry authentication data from ASM
func (task *Task) requiresASMDockerAuthData() bool {
	for _, container := range task.Containers {
		if container.ShouldPullWithASMAuth() {
			return true
		}
	}
	return false
}

// initializeASMAuthResource builds the resource dependency map for the ASM auth resource
func (task *Task) initializeASMAuthResource(credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) {
	asmAuthResource := asmauth.NewASMAuthResource(task.Arn, task.getAllASMAuthDataRequirements(),
		task.ExecutionCredentialsID, credentialsManager, resourceFields.ASMClientCreator)
	task.AddResource(asmauth.ResourceName, asmAuthResource)
	for _, container := range task.Containers {
		if container.ShouldPullWithASMAuth() {
			container.BuildResourceDependency(asmAuthResource.GetName(),
				resourcestatus.ResourceStatus(asmauth.ASMAuthStatusCreated),
				apicontainerstatus.ContainerPulled)
		}
	}
}

func (task *Task) getAllASMAuthDataRequirements() []*apicontainer.ASMAuthData {
	var reqs []*apicontainer.ASMAuthData
	for _, container := range task.Containers {
		if container.ShouldPullWithASMAuth() {
			reqs = append(reqs, container.RegistryAuthentication.ASMAuthData)
		}
	}
	return reqs
}

// BuildCNIConfig constructs the cni configuration from eni
func (task *Task) BuildCNIConfig() (*ecscni.Config, error) {
	if !task.isNetworkModeVPC() {
		return nil, errors.New("task config: task has no ENIs associated with it, unable to generate cni config")
	}

	cfg := &ecscni.Config{}
	eni := task.GetTaskENI()

	cfg.ENIID = eni.ID
	cfg.ID = eni.MacAddress
	cfg.ENIMACAddress = eni.MacAddress
	cfg.SubnetGatewayIPV4Address = eni.GetSubnetGatewayIPV4Address()
	for _, ipv4 := range eni.IPV4Addresses {
		if ipv4.Primary {
			cfg.ENIIPV4Address = ipv4.Address
			break
		}
	}

	// If there is ipv6 assigned to eni then set it
	if len(eni.IPV6Addresses) > 0 {
		cfg.ENIIPV6Address = eni.IPV6Addresses[0].Address
	}

	return cfg, nil
}

// isNetworkModeVPC checks if the task is configured to use task-networking feature
func (task *Task) isNetworkModeVPC() bool {
	if task.GetTaskENI() == nil {
		return false
	}

	return true
}

func (task *Task) addNetworkResourceProvisioningDependency(cfg *config.Config) {
	if !task.isNetworkModeVPC() {
		return
	}
	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerResourcesProvisioned)
	pauseContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	pauseContainer.Name = PauseContainerName
	pauseContainer.Image = fmt.Sprintf("%s:%s", cfg.PauseContainerImageName, cfg.PauseContainerTag)
	pauseContainer.Essential = true
	pauseContainer.Type = apicontainer.ContainerCNIPause
	task.Containers = append(task.Containers, pauseContainer)

	for _, container := range task.Containers {
		if container.IsInternal() {
			continue
		}
		container.BuildContainerDependency(PauseContainerName, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerPulled)
		pauseContainer.BuildContainerDependency(container.Name, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped)
	}
}

// ContainerByName returns the *Container for the given name
func (task *Task) ContainerByName(name string) (*apicontainer.Container, bool) {
	for _, container := range task.Containers {
		if container.Name == name {
			return container, true
		}
	}
	return nil, false
}

// HostVolumeByName returns the task Volume for the given a volume name in that
// task. The second return value indicates the presence of that volume
func (task *Task) HostVolumeByName(name string) (taskresourcevolume.Volume, bool) {
	for _, v := range task.Volumes {
		if v.Name == name {
			return v.Volume, true
		}
	}
	return nil, false
}

// UpdateMountPoints updates the mount points of volumes that were created
// without specifying a host path.  This is used as part of the empty host
// volume feature.
func (task *Task) UpdateMountPoints(cont *apicontainer.Container, vols []docker.Mount) {
	for _, mountPoint := range cont.MountPoints {
		containerPath := getCanonicalPath(mountPoint.ContainerPath)
		for _, vol := range vols {
			if strings.Compare(vol.Destination, containerPath) == 0 ||
				// /path/ -> /path or \path\ -> \path
				strings.Compare(vol.Destination, strings.TrimRight(containerPath, string(filepath.Separator))) == 0 {
				if hostVolume, exists := task.HostVolumeByName(mountPoint.SourceVolume); exists {
					if empty, ok := hostVolume.(*taskresourcevolume.LocalDockerVolume); ok {
						empty.HostPath = vol.Source
					}
				}
			}
		}
	}
}

// updateTaskKnownStatus updates the given task's status based on its container's status.
// It updates to the minimum of all containers no matter what
// It returns a TaskStatus indicating what change occurred or TaskStatusNone if
// there was no change
// Invariant: task known status is the minimum of container known status
func (task *Task) updateTaskKnownStatus() (newStatus apitaskstatus.TaskStatus) {
	seelog.Debugf("Updating task's known status, task: %s", task.String())
	// Set to a large 'impossible' status that can't be the min
	containerEarliestKnownStatus := apicontainerstatus.ContainerZombie
	var earliestKnownStatusContainer *apicontainer.Container
	essentialContainerStopped := false
	for _, container := range task.Containers {
		containerKnownStatus := container.GetKnownStatus()
		if containerKnownStatus == apicontainerstatus.ContainerStopped && container.Essential {
			essentialContainerStopped = true
		}
		if containerKnownStatus < containerEarliestKnownStatus {
			containerEarliestKnownStatus = containerKnownStatus
			earliestKnownStatusContainer = container
		}
	}
	if earliestKnownStatusContainer == nil {
		seelog.Criticalf(
			"Impossible state found while updating tasks's known status, earliest state recorded as %s for task [%v]",
			containerEarliestKnownStatus.String(), task)
		return apitaskstatus.TaskStatusNone
	}
	seelog.Debugf("Container with earliest known container is [%s] for task: %s",
		earliestKnownStatusContainer.String(), task.String())
	// If the essential container is stopped while other containers may be running
	// don't update the task status until the other containers are stopped.
	if earliestKnownStatusContainer.IsKnownSteadyState() && essentialContainerStopped {
		seelog.Debugf(
			"Essential container is stopped while other containers are running, not updating task status for task: %s",
			task.String())
		return apitaskstatus.TaskStatusNone
	}
	// We can't rely on earliest container known status alone for determining if the
	// task state needs to be updated as containers can have different steady states
	// defined. Instead we should get the task status for all containers' known
	// statuses and compute the min of this
	earliestKnownTaskStatus := task.getEarliestKnownTaskStatusForContainers()
	if task.GetKnownStatus() < earliestKnownTaskStatus {
		seelog.Debugf("Updating task's known status to: %s, task: %s",
			earliestKnownTaskStatus.String(), task.String())
		task.SetKnownStatus(earliestKnownTaskStatus)
		return task.GetKnownStatus()
	}
	return apitaskstatus.TaskStatusNone
}

// getEarliestKnownTaskStatusForContainers gets the lowest (earliest) task status
// based on the known statuses of all containers in the task
func (task *Task) getEarliestKnownTaskStatusForContainers() apitaskstatus.TaskStatus {
	if len(task.Containers) == 0 {
		seelog.Criticalf("No containers in the task: %s", task.String())
		return apitaskstatus.TaskStatusNone
	}
	// Set earliest container status to an impossible to reach 'high' task status
	earliest := apitaskstatus.TaskZombie
	for _, container := range task.Containers {
		containerTaskStatus := apitaskstatus.MapContainerToTaskStatus(container.GetKnownStatus(), container.GetSteadyStateStatus())
		if containerTaskStatus < earliest {
			earliest = containerTaskStatus
		}
	}

	return earliest
}

// DockerConfig converts the given container in this task to the format of
// GoDockerClient's 'Config' struct
func (task *Task) DockerConfig(container *apicontainer.Container, apiVersion dockerclient.DockerVersion) (*docker.Config, *apierrors.DockerClientConfigError) {
	return task.dockerConfig(container, apiVersion)
}

func (task *Task) dockerConfig(container *apicontainer.Container, apiVersion dockerclient.DockerVersion) (*docker.Config, *apierrors.DockerClientConfigError) {
	dockerEnv := make([]string, 0, len(container.Environment))
	for envKey, envVal := range container.Environment {
		dockerEnv = append(dockerEnv, envKey+"="+envVal)
	}

	// Convert MB to B
	dockerMem := int64(container.Memory * 1024 * 1024)
	if dockerMem != 0 && dockerMem < apicontainer.DockerContainerMinimumMemoryInBytes {
		dockerMem = apicontainer.DockerContainerMinimumMemoryInBytes
	}

	var entryPoint []string
	if container.EntryPoint != nil {
		entryPoint = *container.EntryPoint
	}

	config := &docker.Config{
		Image:        container.Image,
		Cmd:          container.Command,
		Entrypoint:   entryPoint,
		ExposedPorts: task.dockerExposedPorts(container),
		Env:          dockerEnv,
	}

	err := task.SetConfigHostconfigBasedOnVersion(container, config, nil, apiVersion)
	if err != nil {
		return nil, &apierrors.DockerClientConfigError{"setting docker config failed, err: " + err.Error()}
	}

	if container.DockerConfig.Config != nil {
		err := json.Unmarshal([]byte(aws.StringValue(container.DockerConfig.Config)), &config)
		if err != nil {
			return nil, &apierrors.DockerClientConfigError{"Unable decode given docker config: " + err.Error()}
		}
	}
	if container.HealthCheckType == apicontainer.DockerHealthCheckType && config.Healthcheck == nil {
		return nil, &apierrors.DockerClientConfigError{
			"docker health check is nil while container health check type is DOCKER"}
	}

	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	if container.Type == apicontainer.ContainerCNIPause {
		// apply hostname to pause container's docker config
		return task.applyENIHostname(config), nil
	}

	return config, nil
}

// SetConfigHostconfigBasedOnVersion sets the fields in both Config and HostConfig based on api version for backward compatibility
func (task *Task) SetConfigHostconfigBasedOnVersion(container *apicontainer.Container, config *docker.Config, hc *docker.HostConfig, apiVersion dockerclient.DockerVersion) error {
	// Convert MB to B
	dockerMem := int64(container.Memory * 1024 * 1024)
	if dockerMem != 0 && dockerMem < apicontainer.DockerContainerMinimumMemoryInBytes {
		seelog.Warnf("Task %s container %s memory setting is too low, increasing to %d bytes",
			task.Arn, container.Name, apicontainer.DockerContainerMinimumMemoryInBytes)
		dockerMem = apicontainer.DockerContainerMinimumMemoryInBytes
	}
	cpuShare := task.dockerCPUShares(container.CPU)

	// Docker copied Memory and cpu field into hostconfig in 1.6 with api version(1.18)
	// https://github.com/moby/moby/commit/837eec064d2d40a4d86acbc6f47fada8263e0d4c
	dockerAPIVersion, err := docker.NewAPIVersion(string(apiVersion))
	if err != nil {
		seelog.Errorf("Creating docker api version failed, err: %v", err)
		return err
	}

	dockerAPIVersion_1_18 := docker.APIVersion([]int{1, 18})
	if dockerAPIVersion.GreaterThanOrEqualTo(dockerAPIVersion_1_18) {
		// Set the memory and cpu in host config
		if hc != nil {
			hc.Memory = dockerMem
			hc.CPUShares = cpuShare
		}
		return nil
	}

	// Set the memory and cpu in config
	if config != nil {
		config.Memory = dockerMem
		config.CPUShares = cpuShare
	}

	return nil
}

func (task *Task) dockerExposedPorts(container *apicontainer.Container) map[docker.Port]struct{} {
	dockerExposedPorts := make(map[docker.Port]struct{})

	for _, portBinding := range container.Ports {
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/" + portBinding.Protocol.String())
		dockerExposedPorts[dockerPort] = struct{}{}
	}
	return dockerExposedPorts
}

// DockerHostConfig construct the configuration recognized by docker
func (task *Task) DockerHostConfig(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer, apiVersion dockerclient.DockerVersion) (*docker.HostConfig, *apierrors.HostConfigError) {
	return task.dockerHostConfig(container, dockerContainerMap, apiVersion)
}

// ApplyExecutionRoleLogsAuth will check whether the task has execution role
// credentials, and add the genereated credentials endpoint to the associated HostConfig
func (task *Task) ApplyExecutionRoleLogsAuth(hostConfig *docker.HostConfig, credentialsManager credentials.Manager) *apierrors.HostConfigError {
	id := task.GetExecutionCredentialsID()
	if id == "" {
		// No execution credentials set for the task. Do not inject the endpoint environment variable.
		return &apierrors.HostConfigError{"No execution credentials set for the task"}
	}

	executionRoleCredentials, ok := credentialsManager.GetTaskCredentials(id)
	if !ok {
		// Task has credentials id set, but credentials manager is unaware of
		// the id. This should never happen as the payload handler sets
		// credentialsId for the task after adding credentials to the
		// credentials manager
		return &apierrors.HostConfigError{"Unable to get execution role credentials for task"}
	}
	credentialsEndpointRelativeURI := executionRoleCredentials.IAMRoleCredentials.GenerateCredentialsEndpointRelativeURI()
	hostConfig.LogConfig.Config[awslogsCredsEndpointOpt] = credentialsEndpointRelativeURI
	return nil
}

func (task *Task) dockerHostConfig(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer, apiVersion dockerclient.DockerVersion) (*docker.HostConfig, *apierrors.HostConfigError) {
	dockerLinkArr, err := task.dockerLinks(container, dockerContainerMap)
	if err != nil {
		return nil, &apierrors.HostConfigError{err.Error()}
	}

	dockerPortMap := task.dockerPortMap(container)

	volumesFrom, err := task.dockerVolumesFrom(container, dockerContainerMap)
	if err != nil {
		return nil, &apierrors.HostConfigError{err.Error()}
	}

	binds, err := task.dockerHostBinds(container)
	if err != nil {
		return nil, &apierrors.HostConfigError{err.Error()}
	}

	// Populate hostConfig
	hostConfig := &docker.HostConfig{
		Links:        dockerLinkArr,
		Binds:        binds,
		PortBindings: dockerPortMap,
		VolumesFrom:  volumesFrom,
	}

	err = task.SetConfigHostconfigBasedOnVersion(container, nil, hostConfig, apiVersion)
	if err != nil {
		return nil, &apierrors.HostConfigError{err.Error()}
	}

	if container.DockerConfig.HostConfig != nil {
		err := json.Unmarshal([]byte(*container.DockerConfig.HostConfig), hostConfig)
		if err != nil {
			return nil, &apierrors.HostConfigError{"Unable to decode given host config: " + err.Error()}
		}
	}

	err = task.platformHostConfigOverride(hostConfig)
	if err != nil {
		return nil, &apierrors.HostConfigError{err.Error()}
	}

	// Determine if network mode should be overridden and override it if needed
	ok, networkMode := task.shouldOverrideNetworkMode(container, dockerContainerMap)
	if !ok {
		return hostConfig, nil
	}
	hostConfig.NetworkMode = networkMode
	// Override 'awsvpc' parameters if needed
	if container.Type == apicontainer.ContainerCNIPause {

		// apply ExtraHosts to HostConfig for pause container
		if hosts := task.generateENIExtraHosts(); hosts != nil {
			hostConfig.ExtraHosts = append(hostConfig.ExtraHosts, hosts...)
		}

		// Override the DNS settings for the pause container if ENI has custom
		// DNS settings
		return task.overrideDNS(hostConfig), nil
	}

	return hostConfig, nil
}

// shouldOverrideNetworkMode returns true if the network mode of the container needs
// to be overridden. It also returns the override string in this case. It returns
// false otherwise
func (task *Task) shouldOverrideNetworkMode(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer) (bool, string) {
	// TODO. We can do an early return here by determining which kind of task it is
	// Example: Does this task have ENIs in its payload, what is its networking mode etc
	if container.IsInternal() {
		// If it's an internal container, set the network mode to none.
		// Currently, internal containers are either for creating empty host
		// volumes or for creating the 'pause' container. Both of these
		// only need the network mode to be set to "none"
		return true, networkModeNone
	}

	// For other types of containers, determine if the container map contains
	// a pause container. Since a pause container is only added to the task
	// when using non docker daemon supported network modes, its existence
	// indicates the need to configure the network mode outside of supported
	// network drivers
	if task.GetTaskENI() == nil {
		return false, ""
	}

	pauseContName := ""
	for _, cont := range task.Containers {
		if cont.Type == apicontainer.ContainerCNIPause {
			pauseContName = cont.Name
			break
		}
	}
	if pauseContName == "" {
		seelog.Critical("Pause container required, but not found in the task: %s", task.String())
		return false, ""
	}
	pauseContainer, ok := dockerContainerMap[pauseContName]
	if !ok || pauseContainer == nil {
		// This should never be the case and implies a code-bug.
		seelog.Criticalf("Pause container required, but not found in container map for container: [%s] in task: %s",
			container.String(), task.String())
		return false, ""
	}
	return true, networkModeContainerPrefix + pauseContainer.DockerID
}

// overrideDNS overrides a container's host config if the following conditions are
// true:
// 1. Task has an ENI associated with it
// 2. ENI has custom DNS IPs and search list associated with it
// This should only be done for the pause container as other containers inherit
// /etc/resolv.conf of this container (they share the network namespace)
func (task *Task) overrideDNS(hostConfig *docker.HostConfig) *docker.HostConfig {
	eni := task.GetTaskENI()
	if eni == nil {
		return hostConfig
	}

	hostConfig.DNS = eni.DomainNameServers
	hostConfig.DNSSearch = eni.DomainNameSearchList

	return hostConfig
}

// applyENIHostname adds the hostname provided by the ENI message to the
// container's docker config. At the time of implmentation, we are only using it
// to configure the pause container for awsvpc tasks
func (task *Task) applyENIHostname(dockerConfig *docker.Config) *docker.Config {
	eni := task.GetTaskENI()
	if eni == nil {
		return dockerConfig
	}

	hostname := eni.GetHostname()
	if hostname == "" {
		return dockerConfig
	}

	dockerConfig.Hostname = hostname
	return dockerConfig
}

// generateENIExtraHosts returns a slice of strings of the form "hostname:ip"
// that is generated using the hostname and ip addresses allocated to the ENI
func (task *Task) generateENIExtraHosts() []string {
	eni := task.GetTaskENI()
	if eni == nil {
		return nil
	}

	hostname := eni.GetHostname()
	if hostname == "" {
		return nil
	}

	extraHosts := []string{}

	for _, ip := range eni.GetIPV4Addresses() {
		host := fmt.Sprintf("%s:%s", hostname, ip)
		extraHosts = append(extraHosts, host)
	}
	return extraHosts
}

func (task *Task) dockerLinks(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer) ([]string, error) {
	dockerLinkArr := make([]string, len(container.Links))
	for i, link := range container.Links {
		linkParts := strings.Split(link, ":")
		if len(linkParts) > 2 {
			return []string{}, errors.New("Invalid link format")
		}
		linkName := linkParts[0]
		var linkAlias string

		if len(linkParts) == 2 {
			linkAlias = linkParts[1]
		} else {
			seelog.Warnf("Link name [%s] found with no linkalias for container: [%s] in task: [%s]",
				linkName, container.String(), task.String())
			linkAlias = linkName
		}

		targetContainer, ok := dockerContainerMap[linkName]
		if !ok {
			return []string{}, errors.New("Link target not available: " + linkName)
		}
		dockerLinkArr[i] = targetContainer.DockerName + ":" + linkAlias
	}
	return dockerLinkArr, nil
}

func (task *Task) dockerPortMap(container *apicontainer.Container) map[docker.Port][]docker.PortBinding {
	dockerPortMap := make(map[docker.Port][]docker.PortBinding)

	for _, portBinding := range container.Ports {
		dockerPort := docker.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/" + portBinding.Protocol.String())
		currentMappings, existing := dockerPortMap[dockerPort]
		if existing {
			dockerPortMap[dockerPort] = append(currentMappings, docker.PortBinding{HostPort: strconv.Itoa(int(portBinding.HostPort))})
		} else {
			dockerPortMap[dockerPort] = []docker.PortBinding{{HostPort: strconv.Itoa(int(portBinding.HostPort))}}
		}
	}
	return dockerPortMap
}

func (task *Task) dockerVolumesFrom(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer) ([]string, error) {
	volumesFrom := make([]string, len(container.VolumesFrom))
	for i, volume := range container.VolumesFrom {
		targetContainer, ok := dockerContainerMap[volume.SourceContainer]
		if !ok {
			return []string{}, errors.New("Volume target not available: " + volume.SourceContainer)
		}
		if volume.ReadOnly {
			volumesFrom[i] = targetContainer.DockerName + ":ro"
		} else {
			volumesFrom[i] = targetContainer.DockerName
		}
	}
	return volumesFrom, nil
}

func (task *Task) dockerHostBinds(container *apicontainer.Container) ([]string, error) {
	if container.Name == emptyHostVolumeName {
		// emptyHostVolumes are handled as a special case in config, not
		// hostConfig
		return []string{}, nil
	}

	binds := make([]string, len(container.MountPoints))
	for i, mountPoint := range container.MountPoints {
		hv, ok := task.HostVolumeByName(mountPoint.SourceVolume)
		if !ok {
			return []string{}, errors.New("Invalid volume referenced: " + mountPoint.SourceVolume)
		}

		if hv.Source() == "" || mountPoint.ContainerPath == "" {
			seelog.Errorf(
				"Unable to resolve volume mounts for container [%s]; invalid path: [%s]; [%s] -> [%s] in task: [%s]",
				container.Name, mountPoint.SourceVolume, hv.Source(), mountPoint.ContainerPath, task.String())
			return []string{}, errors.Errorf("Unable to resolve volume mounts; invalid path: %s %s; %s -> %s",
				container.Name, mountPoint.SourceVolume, hv.Source(), mountPoint.ContainerPath)
		}

		bind := hv.Source() + ":" + mountPoint.ContainerPath
		if mountPoint.ReadOnly {
			bind += ":ro"
		}
		binds[i] = bind
	}

	return binds, nil
}

// UpdateStatus updates a task's known and desired statuses to be compatible
// with all of its containers
// It will return a bool indicating if there was a change
func (task *Task) UpdateStatus() bool {
	change := task.updateTaskKnownStatus()
	// DesiredStatus can change based on a new known status
	task.UpdateDesiredStatus()
	return change != apitaskstatus.TaskStatusNone
}

// UpdateDesiredStatus sets the known status of the task
func (task *Task) UpdateDesiredStatus() {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.updateTaskDesiredStatusUnsafe()
	task.updateContainerDesiredStatusUnsafe(task.DesiredStatusUnsafe)
	task.updateResourceDesiredStatusUnsafe(task.DesiredStatusUnsafe)
}

// updateTaskDesiredStatusUnsafe determines what status the task should properly be at based on the containers' statuses
// Invariant: task desired status must be stopped if any essential container is stopped
func (task *Task) updateTaskDesiredStatusUnsafe() {
	seelog.Debugf("Updating task: [%s]", task.stringUnsafe())

	// A task's desired status is stopped if any essential container is stopped
	// Otherwise, the task's desired status is unchanged (typically running, but no need to change)
	for _, cont := range task.Containers {
		if cont.Essential && (cont.KnownTerminal() || cont.DesiredTerminal()) {
			seelog.Debugf("Updating task desired status to stopped because of container: [%s]; task: [%s]",
				cont.Name, task.stringUnsafe())
			task.DesiredStatusUnsafe = apitaskstatus.TaskStopped
		}
	}
}

// updateContainerDesiredStatusUnsafe sets all container's desired status's to the
// task's desired status
// Invariant: container desired status is <= task desired status converted to container status
// Note: task desired status and container desired status is typically only RUNNING or STOPPED
func (task *Task) updateContainerDesiredStatusUnsafe(taskDesiredStatus apitaskstatus.TaskStatus) {
	for _, container := range task.Containers {
		taskDesiredStatusToContainerStatus := apitaskstatus.MapTaskToContainerStatus(taskDesiredStatus, container.GetSteadyStateStatus())
		if container.GetDesiredStatus() < taskDesiredStatusToContainerStatus {
			container.SetDesiredStatus(taskDesiredStatusToContainerStatus)
		}
	}
}

// updateResourceDesiredStatusUnsafe sets all resources' desired status depending on the
// task's desired status
// TODO: Create a mapping of resource status to the corresponding task status and use it here
func (task *Task) updateResourceDesiredStatusUnsafe(taskDesiredStatus apitaskstatus.TaskStatus) {
	resources := task.getResourcesUnsafe()
	for _, r := range resources {
		if taskDesiredStatus == apitaskstatus.TaskRunning {
			if r.GetDesiredStatus() < r.SteadyState() {
				r.SetDesiredStatus(r.SteadyState())
			}
		} else {
			if r.GetDesiredStatus() < r.TerminalStatus() {
				r.SetDesiredStatus(r.TerminalStatus())
			}
		}
	}
}

// SetKnownStatus sets the known status of the task
func (task *Task) SetKnownStatus(status apitaskstatus.TaskStatus) {
	task.setKnownStatus(status)
	task.updateKnownStatusTime()
}

func (task *Task) setKnownStatus(status apitaskstatus.TaskStatus) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.KnownStatusUnsafe = status
}

func (task *Task) updateKnownStatusTime() {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.KnownStatusTimeUnsafe = ttime.Now()
}

// GetKnownStatus gets the KnownStatus of the task
func (task *Task) GetKnownStatus() apitaskstatus.TaskStatus {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.KnownStatusUnsafe
}

// GetKnownStatusTime gets the KnownStatusTime of the task
func (task *Task) GetKnownStatusTime() time.Time {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.KnownStatusTimeUnsafe
}

// SetCredentialsID sets the credentials ID for the task
func (task *Task) SetCredentialsID(id string) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.credentialsID = id
}

// GetCredentialsID gets the credentials ID for the task
func (task *Task) GetCredentialsID() string {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.credentialsID
}

// SetExecutionRoleCredentialsID sets the ID for the task execution role credentials
func (task *Task) SetExecutionRoleCredentialsID(id string) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.ExecutionCredentialsID = id
}

// GetExecutionCredentialsID gets the credentials ID for the task
func (task *Task) GetExecutionCredentialsID() string {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.ExecutionCredentialsID
}

// GetDesiredStatus gets the desired status of the task
func (task *Task) GetDesiredStatus() apitaskstatus.TaskStatus {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.DesiredStatusUnsafe
}

// SetDesiredStatus sets the desired status of the task
func (task *Task) SetDesiredStatus(status apitaskstatus.TaskStatus) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.DesiredStatusUnsafe = status
}

// GetSentStatus safely returns the SentStatus of the task
func (task *Task) GetSentStatus() apitaskstatus.TaskStatus {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.SentStatusUnsafe
}

// SetSentStatus safely sets the SentStatus of the task
func (task *Task) SetSentStatus(status apitaskstatus.TaskStatus) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.SentStatusUnsafe = status
}

// SetTaskENI sets the eni information of the task
func (task *Task) SetTaskENI(eni *apieni.ENI) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.ENI = eni
}

// GetTaskENI returns the eni of task, for now task can only have one enis
func (task *Task) GetTaskENI() *apieni.ENI {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.ENI
}

// GetStopSequenceNumber returns the stop sequence number of a task
func (task *Task) GetStopSequenceNumber() int64 {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.StopSequenceNumber
}

// SetStopSequenceNumber sets the stop seqence number of a task
func (task *Task) SetStopSequenceNumber(seqnum int64) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.StopSequenceNumber = seqnum
}

// SetPullStartedAt sets the task pullstartedat timestamp and returns whether
// this field was updated or not
func (task *Task) SetPullStartedAt(timestamp time.Time) bool {
	task.lock.Lock()
	defer task.lock.Unlock()

	// Only set this field if it is not set
	if task.PullStartedAtUnsafe.IsZero() {
		task.PullStartedAtUnsafe = timestamp
		return true
	}
	return false
}

// GetPullStartedAt returns the PullStartedAt timestamp
func (task *Task) GetPullStartedAt() time.Time {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.PullStartedAtUnsafe
}

// SetPullStoppedAt sets the task pullstoppedat timestamp
func (task *Task) SetPullStoppedAt(timestamp time.Time) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.PullStoppedAtUnsafe = timestamp
}

// GetPullStoppedAt returns the PullStoppedAt timestamp
func (task *Task) GetPullStoppedAt() time.Time {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.PullStoppedAtUnsafe
}

// SetExecutionStoppedAt sets the ExecutionStoppedAt timestamp of the task
func (task *Task) SetExecutionStoppedAt(timestamp time.Time) bool {
	task.lock.Lock()
	defer task.lock.Unlock()

	if task.ExecutionStoppedAtUnsafe.IsZero() {
		task.ExecutionStoppedAtUnsafe = timestamp
		return true
	}
	return false
}

// GetExecutionStoppedAt returns the task executionStoppedAt timestamp
func (task *Task) GetExecutionStoppedAt() time.Time {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.ExecutionStoppedAtUnsafe
}

// String returns a human readable string representation of this object
func (task *Task) String() string {
	task.lock.Lock()
	defer task.lock.Unlock()
	return task.stringUnsafe()
}

// stringUnsafe returns a human readable string representation of this object
func (task *Task) stringUnsafe() string {
	res := fmt.Sprintf("%s:%s %s, TaskStatus: (%s->%s)",
		task.Family, task.Version, task.Arn,
		task.KnownStatusUnsafe.String(), task.DesiredStatusUnsafe.String())
	res += " Containers: ["
	for _, container := range task.Containers {
		res += fmt.Sprintf("%s (%s->%s),", container.Name, container.GetKnownStatus().String(), container.GetDesiredStatus().String())
	}

	if task.ENI != nil {
		res += fmt.Sprintf(" ENI: [%s]", task.ENI.String())
	}
	return res + "]"
}

// GetID is used to retrieve the taskID from taskARN
// Reference: http://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html#arn-syntax-ecs
func (task *Task) GetID() (string, error) {
	// Parse taskARN
	parsedARN, err := arn.Parse(task.Arn)
	if err != nil {
		return "", errors.Wrapf(err, "task get-id: malformed taskARN: %s", task.Arn)
	}

	// Get task resource section
	resource := parsedARN.Resource

	if !strings.Contains(resource, arnResourceDelimiter) {
		return "", errors.Errorf("task get-id: malformed task resource: %s", resource)
	}

	resourceSplit := strings.SplitN(resource, arnResourceDelimiter, arnResourceSections)
	if len(resourceSplit) != arnResourceSections {
		return "", errors.Errorf(
			"task get-id: invalid task resource split: %s, expected=%d, actual=%d",
			resource, arnResourceSections, len(resourceSplit))
	}

	return resourceSplit[1], nil
}

// RecordExecutionStoppedAt checks if this is an essential container stopped
// and set the task executionStoppedAt timestamps
func (task *Task) RecordExecutionStoppedAt(container *apicontainer.Container) {
	if !container.Essential {
		return
	}
	if container.GetKnownStatus() != apicontainerstatus.ContainerStopped {
		return
	}
	// If the essential container is stopped, set the ExecutionStoppedAt timestamp
	now := time.Now()
	ok := task.SetExecutionStoppedAt(now)
	if !ok {
		// ExecutionStoppedAt was already recorded. Nothing to left to do here
		return
	}
	seelog.Infof("Task [%s]: recording execution stopped time. Essential container [%s] stopped at: %s",
		task.Arn, container.Name, now.String())
}

// GetResources returns the list of task resources from ResourcesMap
func (task *Task) GetResources() []taskresource.TaskResource {
	task.lock.RLock()
	defer task.lock.RUnlock()
	return task.getResourcesUnsafe()
}

// getResourcesUnsafe returns the list of task resources from ResourcesMap
func (task *Task) getResourcesUnsafe() []taskresource.TaskResource {
	var resourceList []taskresource.TaskResource
	for _, resources := range task.ResourcesMapUnsafe {
		resourceList = append(resourceList, resources...)
	}
	return resourceList
}

// AddResource adds a resource to ResourcesMap
func (task *Task) AddResource(resourceType string, resource taskresource.TaskResource) {
	task.lock.Lock()
	defer task.lock.Unlock()
	task.ResourcesMapUnsafe[resourceType] = append(task.ResourcesMapUnsafe[resourceType], resource)
}

// SetTerminalReason sets the terminalReason string and this can only be set
// once per the task's lifecycle. This field does not accept updates.
func (task *Task) SetTerminalReason(reason string) {
	seelog.Infof("Task [%s]: attempting to set terminal reason for task [%s]", task.Arn, reason)
	task.terminalReasonOnce.Do(func() {
		seelog.Infof("Task [%s]: setting terminal reason for task [%s]", task.Arn, reason)
		task.terminalReason = reason
	})
}

// GetTerminalReason retrieves the terminalReason string
func (task *Task) GetTerminalReason() string {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.terminalReason
}

// PopulateASMAuthData sets docker auth credentials for a container
func (task *Task) PopulateASMAuthData(container *apicontainer.Container) error {
	secretID := container.RegistryAuthentication.ASMAuthData.CredentialsParameter
	resource, ok := task.getASMAuthResource()
	if !ok {
		return errors.New("task auth data: unable to fetch ASM resource")
	}
	// This will cause a panic if the resource is not of ASMAuthResource type.
	// But, it's better to panic as we should have never reached condition
	// unless we released an agent without any testing around that code path
	asmResource := resource[0].(*asmauth.ASMAuthResource)
	dac, ok := asmResource.GetASMDockerAuthConfig(secretID)
	if !ok {
		return errors.Errorf("task auth data: unable to fetch docker auth config [%s]", secretID)
	}
	container.SetASMDockerAuthConfig(dac)
	return nil
}

func (task *Task) getASMAuthResource() ([]taskresource.TaskResource, bool) {
	task.lock.RLock()
	defer task.lock.RUnlock()

	res, ok := task.ResourcesMapUnsafe[asmauth.ResourceName]
	return res, ok
}

// InitializeResources initializes the required field in the task on agent restart
// Some of the fields in task isn't saved in the agent state file, agent needs
// to initialize these fields before processing the task, eg: docker client in resource
func (task *Task) InitializeResources(resourceFields *taskresource.ResourceFields) {
	task.lock.Lock()
	defer task.lock.Unlock()

	for _, resources := range task.ResourcesMapUnsafe {
		for _, resource := range resources {
			resource.Initialize(resourceFields, task.KnownStatusUnsafe, task.DesiredStatusUnsafe)
		}
	}
}
