// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
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

	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	apiappmesh "github.com/aws/amazon-ecs-agent/agent/api/appmesh"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmauth"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	resourcetype "github.com/aws/amazon-ecs-agent/agent/taskresource/types"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/private/protocol/json/jsonutil"
	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
)

const (
	// NetworkPauseContainerName is the internal name for the pause container
	NetworkPauseContainerName = "~internal~ecs~pause"

	// NamespacePauseContainerName is the internal name for the IPC resource namespace and/or
	// PID namespace sharing pause container
	NamespacePauseContainerName = "~internal~ecs~pause~namespace"

	emptyHostVolumeName = "~internal~ecs-emptyvolume-source"

	// awsSDKCredentialsRelativeURIPathEnvironmentVariableName defines the name of the environment
	// variable in containers' config, which will be used by the AWS SDK to fetch
	// credentials.
	awsSDKCredentialsRelativeURIPathEnvironmentVariableName = "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"

	NvidiaVisibleDevicesEnvVar = "NVIDIA_VISIBLE_DEVICES"
	GPUAssociationType         = "gpu"

	ContainerOrderingCreateCondition = "CREATE"
	ContainerOrderingStartCondition  = "START"

	arnResourceSections  = 2
	arnResourceDelimiter = "/"
	// networkModeNone specifies the string used to define the `none` docker networking mode
	networkModeNone = "none"
	// dockerMappingContainerPrefix specifies the prefix string used for setting the
	// container's option (network, ipc, or pid) to that of another existing container
	dockerMappingContainerPrefix = "container:"

	// awslogsCredsEndpointOpt is the awslogs option that is used to pass in an
	// http endpoint for authentication
	awslogsCredsEndpointOpt = "awslogs-credentials-endpoint"
	// These contants identify the docker flag options
	pidModeHost     = "host"
	pidModeTask     = "task"
	ipcModeHost     = "host"
	ipcModeTask     = "task"
	ipcModeSharable = "shareable"
	ipcModeNone     = "none"

	// firelensConfigBindFormatFluentd and firelensConfigBindFormatFluentbit specify the format of the firelens
	// config file bind mount for fluentd and fluentbit firelens container respectively.
	// First placeholder is host data dir, second placeholder is taskID.
	firelensConfigBindFormatFluentd   = "%s/data/firelens/%s/config/fluent.conf:/fluentd/etc/fluent.conf"
	firelensConfigBindFormatFluentbit = "%s/data/firelens/%s/config/fluent.conf:/fluent-bit/etc/fluent-bit.conf"

	// firelensS3ConfigBindFormat specifies the format of the bind mount for the firelens config file downloaded from S3.
	// First placeholder is host data dir, second placeholder is taskID, third placeholder is the s3 config path inside
	// the firelens container.
	firelensS3ConfigBindFormat = "%s/data/firelens/%s/config/external.conf:%s"

	// firelensSocketBindFormat specifies the format for firelens container's socket directory bind mount.
	// First placeholder is host data dir, second placeholder is taskID.
	firelensSocketBindFormat = "%s/data/firelens/%s/socket/:/var/run/"
	// firelensDriverName is the log driver name for containers that want to use the firelens container to send logs.
	firelensDriverName = "awsfirelens"

	// firelensConfigVarFmt specifies the format for firelens config variable name. The first placeholder
	// is option name. The second placeholder is the index of the container in the task's container list, appended
	// for the purpose of avoiding config vars from different containers within a task collide (note: can't append
	// container name here because it may contain hyphen which will break the config var resolution (see PR 2164 for
	// details), and can't append container ID either because we need the config var in PostUnmarshalTask, which is
	// before all the containers being created).
	firelensConfigVarFmt = "%s_%d"
	// firelensConfigVarPlaceholderFmtFluentd and firelensConfigVarPlaceholderFmtFluentbit specify the config var
	// placeholder format expected by fluentd and fluentbit respectively.
	firelensConfigVarPlaceholderFmtFluentd   = "\"#{ENV['%s']}\""
	firelensConfigVarPlaceholderFmtFluentbit = "${%s}"

	// awsExecutionEnvKey is the key of the env specifying the execution environment.
	awsExecutionEnvKey = "AWS_EXECUTION_ENV"
	// ec2ExecutionEnv specifies the ec2 execution environment.
	ec2ExecutionEnv = "AWS_ECS_EC2"

	// specifies bridge type mode for a task
	BridgeNetworkMode = "bridge"

	// specifies awsvpc type mode for a task
	AWSVPCNetworkMode = "awsvpc"
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
	// Associations are the available associations for the task.
	Associations []Association `json:"associations"`
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

	// ENIs is the list of Elastic Network Interfaces assigned to this task. The
	// TaskENIs type is helpful when decoding state files which might have stored
	// ENIs as a single ENI object instead of a list.
	ENIs TaskENIs `json:"ENI"`

	// AppMesh is the service mesh specified by the task
	AppMesh *apiappmesh.AppMesh

	// MemoryCPULimitsEnabled to determine if task supports CPU, memory limits
	MemoryCPULimitsEnabled bool `json:"MemoryCPULimitsEnabled,omitempty"`

	// PlatformFields consists of fields specific to linux/windows for a task
	PlatformFields PlatformFields `json:"PlatformFields,omitempty"`

	// terminalReason should be used when we explicitly move a task to stopped.
	// This ensures the task object carries some context for why it was explicitly
	// stoppped.
	terminalReason     string
	terminalReasonOnce sync.Once

	// PIDMode is used to determine how PID namespaces are organized between
	// containers of the Task
	PIDMode string `json:"PidMode,omitempty"`

	// IPCMode is used to determine how IPC resources should be shared among
	// containers of the Task
	IPCMode string `json:"IpcMode,omitempty"`

	// NvidiaRuntime is the runtime to pass Nvidia GPU devices to containers
	NvidiaRuntime string `json:"NvidiaRuntime,omitempty"`

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

	//initialize resources map for task
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
		err := task.initializeCgroupResourceSpec(cfg.CgroupPath, cfg.CgroupCPUPeriod, resourceFields)
		if err != nil {
			seelog.Errorf("Task [%s]: could not intialize resource: %v", task.Arn, err)
			return apierrors.NewResourceInitError(task.Arn, err)
		}
	}

	err := task.initializeContainerOrderingForVolumes()
	if err != nil {
		seelog.Errorf("Task [%s]: could not initialize volumes dependency for container: %v", task.Arn, err)
		return apierrors.NewResourceInitError(task.Arn, err)
	}

	err = task.initializeContainerOrderingForLinks()
	if err != nil {
		seelog.Errorf("Task [%s]: could not initialize links dependency for container: %v", task.Arn, err)
		return apierrors.NewResourceInitError(task.Arn, err)
	}

	if task.requiresASMDockerAuthData() {
		task.initializeASMAuthResource(credentialsManager, resourceFields)
	}

	if task.requiresSSMSecret() {
		task.initializeSSMSecretResource(credentialsManager, resourceFields)
	}

	if task.requiresASMSecret() {
		task.initializeASMSecretResource(credentialsManager, resourceFields)
	}

	err = task.initializeDockerLocalVolumes(dockerClient, ctx)
	if err != nil {
		return apierrors.NewResourceInitError(task.Arn, err)
	}
	err = task.initializeDockerVolumes(cfg.SharedVolumeMatchFullConfig, dockerClient, ctx)
	if err != nil {
		return apierrors.NewResourceInitError(task.Arn, err)
	}
	if cfg.GPUSupportEnabled {
		err = task.addGPUResource()
		if err != nil {
			seelog.Errorf("Task [%s]: could not initialize GPU associations: %v", task.Arn, err)
			return apierrors.NewResourceInitError(task.Arn, err)
		}
		task.NvidiaRuntime = cfg.NvidiaRuntime
	}
	task.initializeCredentialsEndpoint(credentialsManager)
	task.initializeContainersV3MetadataEndpoint(utils.NewDynamicUUIDProvider())
	err = task.addNetworkResourceProvisioningDependency(cfg)
	if err != nil {
		seelog.Errorf("Task [%s]: could not provision network resource: %v", task.Arn, err)
		return apierrors.NewResourceInitError(task.Arn, err)
	}
	// Adds necessary Pause containers for sharing PID or IPC namespaces
	task.addNamespaceSharingProvisioningDependency(cfg)

	firelensContainer := task.GetFirelensContainer()
	if firelensContainer != nil {
		err = task.applyFirelensSetup(cfg, resourceFields, firelensContainer, credentialsManager)
		if err != nil {
			return err
		}
	}

	if task.requiresCredentialSpecResource() {
		err = task.initializeCredentialSpecResource(cfg, credentialsManager, resourceFields)
		if err != nil {
			seelog.Errorf("Task [%s]: could not initialize credentialspec resource: %v", task.Arn, err)
			return apierrors.NewResourceInitError(task.Arn, err)
		}
	}

	return nil
}

func (task *Task) applyFirelensSetup(cfg *config.Config, resourceFields *taskresource.ResourceFields,
	firelensContainer *apicontainer.Container, credentialsManager credentials.Manager) error {
	err := task.initializeFirelensResource(cfg, resourceFields, firelensContainer, credentialsManager)
	if err != nil {
		return apierrors.NewResourceInitError(task.Arn, err)
	}
	err = task.addFirelensContainerDependency()
	if err != nil {
		return errors.New("unable to add firelens container dependency")
	}

	return nil
}

func (task *Task) addGPUResource() error {
	for _, association := range task.Associations {
		// One GPU can be associated with only one container
		// That is why validating if association.Containers is of length 1
		if association.Type == GPUAssociationType {
			if len(association.Containers) == 1 {
				container, ok := task.ContainerByName(association.Containers[0])
				if !ok {
					return fmt.Errorf("could not find container with name %s for associating GPU %s",
						association.Containers[0], association.Name)
				} else {
					container.GPUIDs = append(container.GPUIDs, association.Name)
				}
			} else {
				return fmt.Errorf("could not associate multiple containers to GPU %s", association.Name)
			}
		}
	}
	task.populateGPUEnvironmentVariables()
	return nil
}

func (task *Task) isGPUEnabled() bool {
	for _, association := range task.Associations {
		if association.Type == GPUAssociationType {
			return true
		}
	}
	return false
}

func (task *Task) populateGPUEnvironmentVariables() {
	for _, container := range task.Containers {
		if len(container.GPUIDs) > 0 {
			gpuList := strings.Join(container.GPUIDs, ",")
			envVars := make(map[string]string)
			envVars[NvidiaVisibleDevicesEnvVar] = gpuList
			container.MergeEnvironmentVariables(envVars)
		}
	}
}

func (task *Task) shouldRequireNvidiaRuntime(container *apicontainer.Container) bool {
	_, ok := container.Environment[NvidiaVisibleDevicesEnvVar]
	return ok
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
		volumeMetadata := dockerClient.InspectVolume(ctx, vol.Name, dockerclient.InspectVolumeTimeout)
		if volumeMetadata.Error != nil {
			return errors.Wrapf(volumeMetadata.Error, "initialize volume: volume detection failed, volume '%s' does not exist and autoprovision is set to false", vol.Name)
		}
		return nil
	}

	// at this point we know autoprovision = true
	// check if the volume configuration matches the one exists on the instance
	volumeMetadata := dockerClient.InspectVolume(ctx, volumeConfig.DockerVolumeName, dockerclient.InspectVolumeTimeout)
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

// initializeContainersV3MetadataEndpoint generates an v3 endpoint id for each container, constructs the
// v3 metadata endpoint, and injects it as an environment variable
func (task *Task) initializeContainersV3MetadataEndpoint(uuidProvider utils.UUIDProvider) {
	for _, container := range task.Containers {
		v3EndpointID := container.GetV3EndpointID()
		if v3EndpointID == "" { // if container's v3 endpoint has not been set
			container.SetV3EndpointID(uuidProvider.New())
		}

		container.InjectV3MetadataEndpoint()
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

// requiresSSMSecret returns true if at least one container in the task
// needs to retrieve secret from SSM parameter
func (task *Task) requiresSSMSecret() bool {
	for _, container := range task.Containers {
		if container.ShouldCreateWithSSMSecret() {
			return true
		}
	}
	return false
}

// initializeSSMSecretResource builds the resource dependency map for the SSM ssmsecret resource
func (task *Task) initializeSSMSecretResource(credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) {
	ssmSecretResource := ssmsecret.NewSSMSecretResource(task.Arn, task.getAllSSMSecretRequirements(),
		task.ExecutionCredentialsID, credentialsManager, resourceFields.SSMClientCreator)
	task.AddResource(ssmsecret.ResourceName, ssmSecretResource)

	// for every container that needs ssm secret vending as env, it needs to wait all secrets got retrieved
	for _, container := range task.Containers {
		if container.ShouldCreateWithSSMSecret() {
			container.BuildResourceDependency(ssmSecretResource.GetName(),
				resourcestatus.ResourceStatus(ssmsecret.SSMSecretCreated),
				apicontainerstatus.ContainerCreated)
		}

		// Firelens container needs to depends on secret if other containers use secret log options.
		if container.GetFirelensConfig() != nil && task.firelensDependsOnSecretResource(apicontainer.SecretProviderSSM) {
			container.BuildResourceDependency(ssmSecretResource.GetName(),
				resourcestatus.ResourceStatus(ssmsecret.SSMSecretCreated),
				apicontainerstatus.ContainerCreated)
		}
	}
}

// firelensDependsOnSecret checks whether the firelens container needs to depends on a secret resource of
// a certain provider type.
func (task *Task) firelensDependsOnSecretResource(secretProvider string) bool {
	isLogDriverSecretWithGivenProvider := func(s apicontainer.Secret) bool {
		return s.Provider == secretProvider && s.Target == apicontainer.SecretTargetLogDriver
	}
	for _, container := range task.Containers {
		if container.GetLogDriver() == firelensDriverName && container.HasSecret(isLogDriverSecretWithGivenProvider) {
			return true
		}
	}
	return false
}

// getAllSSMSecretRequirements stores all secrets in a map whose key is region and value is all
// secrets in that region
func (task *Task) getAllSSMSecretRequirements() map[string][]apicontainer.Secret {
	reqs := make(map[string][]apicontainer.Secret)

	for _, container := range task.Containers {
		for _, secret := range container.Secrets {
			if secret.Provider == apicontainer.SecretProviderSSM {
				if _, ok := reqs[secret.Region]; !ok {
					reqs[secret.Region] = []apicontainer.Secret{}
				}

				reqs[secret.Region] = append(reqs[secret.Region], secret)
			}
		}
	}
	return reqs
}

// requiresASMSecret returns true if at least one container in the task
// needs to retrieve secret from AWS Secrets Manager
func (task *Task) requiresASMSecret() bool {
	for _, container := range task.Containers {
		if container.ShouldCreateWithASMSecret() {
			return true
		}
	}
	return false
}

// initializeASMSecretResource builds the resource dependency map for the asmsecret resource
func (task *Task) initializeASMSecretResource(credentialsManager credentials.Manager,
	resourceFields *taskresource.ResourceFields) {
	asmSecretResource := asmsecret.NewASMSecretResource(task.Arn, task.getAllASMSecretRequirements(),
		task.ExecutionCredentialsID, credentialsManager, resourceFields.ASMClientCreator)
	task.AddResource(asmsecret.ResourceName, asmSecretResource)

	// for every container that needs asm secret vending as envvar, it needs to wait all secrets got retrieved
	for _, container := range task.Containers {
		if container.ShouldCreateWithASMSecret() {
			container.BuildResourceDependency(asmSecretResource.GetName(),
				resourcestatus.ResourceStatus(asmsecret.ASMSecretCreated),
				apicontainerstatus.ContainerCreated)
		}

		// Firelens container needs to depends on secret if other containers use secret log options.
		if container.GetFirelensConfig() != nil && task.firelensDependsOnSecretResource(apicontainer.SecretProviderASM) {
			container.BuildResourceDependency(asmSecretResource.GetName(),
				resourcestatus.ResourceStatus(asmsecret.ASMSecretCreated),
				apicontainerstatus.ContainerCreated)
		}
	}
}

// getAllASMSecretRequirements stores secrets in a task in a map
func (task *Task) getAllASMSecretRequirements() map[string]apicontainer.Secret {
	reqs := make(map[string]apicontainer.Secret)

	for _, container := range task.Containers {
		for _, secret := range container.Secrets {
			if secret.Provider == apicontainer.SecretProviderASM {
				secretKey := secret.GetSecretResourceCacheKey()
				if _, ok := reqs[secretKey]; !ok {
					reqs[secretKey] = secret
				}
			}
		}
	}
	return reqs
}

// GetFirelensContainer returns the firelens container in the task, if there is one.
func (task *Task) GetFirelensContainer() *apicontainer.Container {
	for _, container := range task.Containers {
		if container.GetFirelensConfig() != nil { // This is a firelens container.
			return container
		}
	}
	return nil
}

// initializeFirelensResource initializes the firelens task resource and adds it as a dependency of the
// firelens container.
func (task *Task) initializeFirelensResource(config *config.Config, resourceFields *taskresource.ResourceFields,
	firelensContainer *apicontainer.Container, credentialsManager credentials.Manager) error {
	if firelensContainer.GetFirelensConfig() == nil {
		return errors.New("firelens container config doesn't exist")
	}

	containerToLogOptions := make(map[string]map[string]string)
	// Collect plain text log options.
	err := task.collectFirelensLogOptions(containerToLogOptions)
	if err != nil {
		return errors.Wrap(err, "unable to initialize firelens resource")
	}

	// Collect secret log options.
	err = task.collectFirelensLogEnvOptions(containerToLogOptions, firelensContainer.FirelensConfig.Type)
	if err != nil {
		return errors.Wrap(err, "unable to initialize firelens resource")
	}

	var firelensResource *firelens.FirelensResource
	for _, container := range task.Containers {
		firelensConfig := container.GetFirelensConfig()
		if firelensConfig != nil {
			var ec2InstanceID string
			if container.Environment != nil && container.Environment[awsExecutionEnvKey] == ec2ExecutionEnv {
				ec2InstanceID = resourceFields.EC2InstanceID
			}

			var networkMode string
			if task.IsNetworkModeAWSVPC() {
				networkMode = AWSVPCNetworkMode
			} else if container.GetNetworkModeFromHostConfig() == "" || container.GetNetworkModeFromHostConfig() == BridgeNetworkMode {
				networkMode = BridgeNetworkMode
			} else {
				networkMode = container.GetNetworkModeFromHostConfig()
			}
			firelensResource, err = firelens.NewFirelensResource(config.Cluster, task.Arn, task.Family+":"+task.Version,
				ec2InstanceID, config.DataDir, firelensConfig.Type, config.AWSRegion, networkMode, firelensConfig.Options, containerToLogOptions,
				credentialsManager, task.ExecutionCredentialsID)
			if err != nil {
				return errors.Wrap(err, "unable to initialize firelens resource")
			}
			task.AddResource(firelens.ResourceName, firelensResource)
			container.BuildResourceDependency(firelensResource.GetName(), resourcestatus.ResourceCreated,
				apicontainerstatus.ContainerCreated)
			return nil
		}
	}

	return errors.New("unable to initialize firelens resource because there's no firelens container")
}

// addFirelensContainerDependency adds a START dependency between each container using awsfirelens log driver
// and the firelens container.
func (task *Task) addFirelensContainerDependency() error {
	var firelensContainer *apicontainer.Container
	for _, container := range task.Containers {
		if container.GetFirelensConfig() != nil {
			firelensContainer = container
		}
	}

	if firelensContainer == nil {
		return errors.New("unable to add firelens container dependency because there's no firelens container")
	}

	if firelensContainer.HasContainerDependencies() {
		// If firelens container has any container dependency, we don't add internal container dependency that depends
		// on it in order to be safe (otherwise we need to deal with circular dependency).
		seelog.Warnf("Not adding container dependency to let firelens container %s start first, because it has dependency on other containers.", firelensContainer.Name)
		return nil
	}

	for _, container := range task.Containers {
		containerHostConfig := container.GetHostConfig()
		if containerHostConfig == nil {
			continue
		}

		// Firelens container itself could be using awsfirelens log driver. Don't add container dependency in this case.
		if container.Name == firelensContainer.Name {
			continue
		}

		hostConfig := &dockercontainer.HostConfig{}
		err := json.Unmarshal([]byte(*containerHostConfig), hostConfig)
		if err != nil {
			return errors.Wrapf(err, "unable to decode host config of container %s", container.Name)
		}

		if hostConfig.LogConfig.Type == firelensDriverName {
			// If there's no dependency between the app container and the firelens container, make firelens container
			// start first to be the default behavior by adding a START container depdendency.
			if !container.DependsOnContainer(firelensContainer.Name) {
				seelog.Infof("Adding a START container dependency on firelens container %s for container %s",
					firelensContainer.Name, container.Name)
				container.AddContainerDependency(firelensContainer.Name, ContainerOrderingStartCondition)
			}
		}
	}

	return nil
}

// collectFirelensLogOptions collects the log options for all the containers that use the firelens container
// as the log driver.
// containerToLogOptions is a nested map. Top level key is the container name. Second level is a map storing
// the log option key and value of the container.
func (task *Task) collectFirelensLogOptions(containerToLogOptions map[string]map[string]string) error {
	for _, container := range task.Containers {
		if container.DockerConfig.HostConfig == nil {
			continue
		}

		hostConfig := &dockercontainer.HostConfig{}
		err := json.Unmarshal([]byte(*container.DockerConfig.HostConfig), hostConfig)
		if err != nil {
			return errors.Wrapf(err, "unable to decode host config of container %s", container.Name)
		}

		if hostConfig.LogConfig.Type == firelensDriverName {
			if containerToLogOptions[container.Name] == nil {
				containerToLogOptions[container.Name] = make(map[string]string)
			}
			for k, v := range hostConfig.LogConfig.Config {
				containerToLogOptions[container.Name][k] = v
			}
		}
	}

	return nil
}

// collectFirelensLogEnvOptions collects all the log secret options. Each secret log option will have a value
// of a config file variable (e.g. "${config_var_name}") and we will pass the secret value as env to the firelens
// container and it will resolve the config file variable from the env.
// Each config variable name has a format of log-option-key_container-name. We need the container name because options
// from different containers using awsfirelens log driver in a task will be presented in the same firelens config file.
func (task *Task) collectFirelensLogEnvOptions(containerToLogOptions map[string]map[string]string, firelensConfigType string) error {
	placeholderFmt := ""
	switch firelensConfigType {
	case firelens.FirelensConfigTypeFluentd:
		placeholderFmt = firelensConfigVarPlaceholderFmtFluentd
	case firelens.FirelensConfigTypeFluentbit:
		placeholderFmt = firelensConfigVarPlaceholderFmtFluentbit
	default:
		return errors.Errorf("unsupported firelens config type %s", firelensConfigType)
	}

	for _, container := range task.Containers {
		for _, secret := range container.Secrets {
			if secret.Target == apicontainer.SecretTargetLogDriver {
				if containerToLogOptions[container.Name] == nil {
					containerToLogOptions[container.Name] = make(map[string]string)
				}

				idx := task.GetContainerIndex(container.Name)
				if idx < 0 {
					return errors.Errorf("can't find container %s in task %s", container.Name, task.Arn)
				}
				containerToLogOptions[container.Name][secret.Name] = fmt.Sprintf(placeholderFmt,
					fmt.Sprintf(firelensConfigVarFmt, secret.Name, idx))
			}
		}
	}
	return nil
}

// AddFirelensContainerBindMounts adds config file bind mount and socket directory bind mount to the firelens
// container's host config.
func (task *Task) AddFirelensContainerBindMounts(firelensConfig *apicontainer.FirelensConfig, hostConfig *dockercontainer.HostConfig,
	config *config.Config) *apierrors.HostConfigError {
	// TODO: fix task.GetID(). It's currently incorrect when opted in task long arn format.
	fields := strings.Split(task.Arn, "/")
	taskID := fields[len(fields)-1]

	var configBind, s3ConfigBind, socketBind string
	switch firelensConfig.Type {
	case firelens.FirelensConfigTypeFluentd:
		configBind = fmt.Sprintf(firelensConfigBindFormatFluentd, config.DataDirOnHost, taskID)
		s3ConfigBind = fmt.Sprintf(firelensS3ConfigBindFormat, config.DataDirOnHost, taskID, firelens.S3ConfigPathFluentd)
	case firelens.FirelensConfigTypeFluentbit:
		configBind = fmt.Sprintf(firelensConfigBindFormatFluentbit, config.DataDirOnHost, taskID)
		s3ConfigBind = fmt.Sprintf(firelensS3ConfigBindFormat, config.DataDirOnHost, taskID, firelens.S3ConfigPathFluentbit)
	default:
		return &apierrors.HostConfigError{Msg: fmt.Sprintf("encounter invalid firelens configuration type %s",
			firelensConfig.Type)}
	}
	socketBind = fmt.Sprintf(firelensSocketBindFormat, config.DataDirOnHost, taskID)

	hostConfig.Binds = append(hostConfig.Binds, configBind, socketBind)

	// Add the s3 config bind mount if firelens container is using a config file from S3.
	if firelensConfig.Options != nil && firelensConfig.Options[firelens.ExternalConfigTypeOption] == firelens.ExternalConfigTypeS3 {
		hostConfig.Binds = append(hostConfig.Binds, s3ConfigBind)
	}
	return nil
}

// BuildCNIConfig builds a list of CNI network configurations for the task.
// If includeIPAMConfig is set to true, the list also includes the bridge IPAM configuration.
func (task *Task) BuildCNIConfig(includeIPAMConfig bool, cniConfig *ecscni.Config) (*ecscni.Config, error) {
	if !task.IsNetworkModeAWSVPC() {
		return nil, errors.New("task config: task network mode is not AWSVPC")
	}

	var netconf *libcni.NetworkConfig
	var ifName string
	var err error

	// Build a CNI network configuration for each ENI.
	for _, eni := range task.ENIs {
		switch eni.InterfaceAssociationProtocol {
		// If the association protocol is set to "default" or unset (to preserve backwards
		// compatibility), consider it a "standard" ENI attachment.
		case "", apieni.DefaultInterfaceAssociationProtocol:
			cniConfig.ID = eni.MacAddress
			ifName, netconf, err = ecscni.NewENINetworkConfig(eni, cniConfig)
		case apieni.VLANInterfaceAssociationProtocol:
			cniConfig.ID = eni.MacAddress
			ifName, netconf, err = ecscni.NewBranchENINetworkConfig(eni, cniConfig)
		default:
			err = errors.Errorf("task config: unknown interface association type: %s",
				eni.InterfaceAssociationProtocol)
		}

		if err != nil {
			return nil, err
		}

		cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
			IfName:           ifName,
			CNINetworkConfig: netconf,
		})
	}

	// Build the bridge CNI network configuration.
	// All AWSVPC tasks have a bridge network.
	ifName, netconf, err = ecscni.NewBridgeNetworkConfig(cniConfig, includeIPAMConfig)
	if err != nil {
		return nil, err
	}
	cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
		IfName:           ifName,
		CNINetworkConfig: netconf,
	})

	// Build a CNI network configuration for AppMesh if enabled.
	appMeshConfig := task.GetAppMesh()
	if appMeshConfig != nil {
		ifName, netconf, err = ecscni.NewAppMeshConfig(appMeshConfig, cniConfig)
		if err != nil {
			return nil, err
		}
		cniConfig.NetworkConfigs = append(cniConfig.NetworkConfigs, &ecscni.NetworkConfig{
			IfName:           ifName,
			CNINetworkConfig: netconf,
		})
	}

	return cniConfig, nil
}

// IsNetworkModeAWSVPC checks if the task is configured to use the AWSVPC task networking feature.
func (task *Task) IsNetworkModeAWSVPC() bool {
	return len(task.ENIs) > 0
}

func (task *Task) addNetworkResourceProvisioningDependency(cfg *config.Config) error {
	if !task.IsNetworkModeAWSVPC() {
		return nil
	}
	pauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerResourcesProvisioned)
	pauseContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	pauseContainer.Name = NetworkPauseContainerName
	pauseContainer.Image = fmt.Sprintf("%s:%s", cfg.PauseContainerImageName, cfg.PauseContainerTag)
	pauseContainer.Essential = true
	pauseContainer.Type = apicontainer.ContainerCNIPause

	// Set pauseContainer user the same as proxy container user when image name is not DefaultPauseContainerImageName
	if task.GetAppMesh() != nil && cfg.PauseContainerImageName != config.DefaultPauseContainerImageName {
		appMeshConfig := task.GetAppMesh()

		// Validation is done when registering task to make sure there is one container name matching
		for _, container := range task.Containers {
			if container.Name != appMeshConfig.ContainerName {
				continue
			}

			if container.DockerConfig.Config == nil {
				return errors.Errorf("user needs to be specified for proxy container")
			}
			containerConfig := &dockercontainer.Config{}
			err := json.Unmarshal([]byte(aws.StringValue(container.DockerConfig.Config)), &containerConfig)
			if err != nil {
				return errors.Errorf("unable to decode given docker config: %s", err.Error())
			}

			if containerConfig.User == "" {
				return errors.Errorf("user needs to be specified for proxy container")
			}

			pauseConfig := dockercontainer.Config{
				User:  containerConfig.User,
				Image: fmt.Sprintf("%s:%s", cfg.PauseContainerImageName, cfg.PauseContainerTag),
			}

			bytes, _ := json.Marshal(pauseConfig)
			serializedConfig := string(bytes)
			pauseContainer.DockerConfig = apicontainer.DockerConfig{
				Config: &serializedConfig,
			}
			break
		}
	}

	task.Containers = append(task.Containers, pauseContainer)

	for _, container := range task.Containers {
		if container.IsInternal() {
			continue
		}
		container.BuildContainerDependency(NetworkPauseContainerName, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerPulled)
		pauseContainer.BuildContainerDependency(container.Name, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped)
	}
	return nil
}

func (task *Task) addNamespaceSharingProvisioningDependency(cfg *config.Config) {
	// Pause container does not need to be created if no namespace sharing will be done at task level
	if task.getIPCMode() != ipcModeTask && task.getPIDMode() != pidModeTask {
		return
	}
	namespacePauseContainer := apicontainer.NewContainerWithSteadyState(apicontainerstatus.ContainerRunning)
	namespacePauseContainer.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	namespacePauseContainer.Name = NamespacePauseContainerName
	namespacePauseContainer.Image = fmt.Sprintf("%s:%s", config.DefaultPauseContainerImageName, config.DefaultPauseContainerTag)
	namespacePauseContainer.Essential = true
	namespacePauseContainer.Type = apicontainer.ContainerNamespacePause
	task.Containers = append(task.Containers, namespacePauseContainer)

	for _, container := range task.Containers {
		if container.IsInternal() {
			continue
		}
		container.BuildContainerDependency(NamespacePauseContainerName, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerPulled)
		namespacePauseContainer.BuildContainerDependency(container.Name, apicontainerstatus.ContainerStopped, apicontainerstatus.ContainerStopped)
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
func (task *Task) UpdateMountPoints(cont *apicontainer.Container, vols []types.MountPoint) {
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
	seelog.Debugf("api/task: Updating task's known status, task: %s", task.String())
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
	seelog.Debugf("api/task: Container with earliest known container is [%s] for task: %s",
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
		seelog.Infof("api/task: Updating task's known status to: %s, task: %s",
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
// the Docker SDK 'Config' struct
func (task *Task) DockerConfig(container *apicontainer.Container, apiVersion dockerclient.DockerVersion) (*dockercontainer.Config, *apierrors.DockerClientConfigError) {
	return task.dockerConfig(container, apiVersion)
}

func (task *Task) dockerConfig(container *apicontainer.Container, apiVersion dockerclient.DockerVersion) (*dockercontainer.Config, *apierrors.DockerClientConfigError) {
	dockerEnv := make([]string, 0, len(container.Environment))
	for envKey, envVal := range container.Environment {
		dockerEnv = append(dockerEnv, envKey+"="+envVal)
	}

	var entryPoint []string
	if container.EntryPoint != nil {
		entryPoint = *container.EntryPoint
	}

	containerConfig := &dockercontainer.Config{
		Image:        container.Image,
		Cmd:          container.Command,
		Entrypoint:   entryPoint,
		ExposedPorts: task.dockerExposedPorts(container),
		Env:          dockerEnv,
	}

	if container.DockerConfig.Config != nil {
		err := json.Unmarshal([]byte(aws.StringValue(container.DockerConfig.Config)), &containerConfig)
		if err != nil {
			return nil, &apierrors.DockerClientConfigError{Msg: "Unable decode given docker config: " + err.Error()}
		}
	}
	if container.HealthCheckType == apicontainer.DockerHealthCheckType && containerConfig.Healthcheck == nil {
		return nil, &apierrors.DockerClientConfigError{
			Msg: "docker health check is nil while container health check type is DOCKER"}
	}

	if containerConfig.Labels == nil {
		containerConfig.Labels = make(map[string]string)
	}

	if container.Type == apicontainer.ContainerCNIPause {
		// apply hostname to pause container's docker config
		return task.applyENIHostname(containerConfig), nil
	}

	return containerConfig, nil
}

func (task *Task) dockerExposedPorts(container *apicontainer.Container) nat.PortSet {
	dockerExposedPorts := make(map[nat.Port]struct{})

	for _, portBinding := range container.Ports {
		dockerPort := nat.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/" + portBinding.Protocol.String())
		dockerExposedPorts[dockerPort] = struct{}{}
	}
	return dockerExposedPorts
}

// DockerHostConfig construct the configuration recognized by docker
func (task *Task) DockerHostConfig(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer, apiVersion dockerclient.DockerVersion) (*dockercontainer.HostConfig, *apierrors.HostConfigError) {
	return task.dockerHostConfig(container, dockerContainerMap, apiVersion)
}

// ApplyExecutionRoleLogsAuth will check whether the task has execution role
// credentials, and add the genereated credentials endpoint to the associated HostConfig
func (task *Task) ApplyExecutionRoleLogsAuth(hostConfig *dockercontainer.HostConfig, credentialsManager credentials.Manager) *apierrors.HostConfigError {
	id := task.GetExecutionCredentialsID()
	if id == "" {
		// No execution credentials set for the task. Do not inject the endpoint environment variable.
		return &apierrors.HostConfigError{Msg: "No execution credentials set for the task"}
	}

	executionRoleCredentials, ok := credentialsManager.GetTaskCredentials(id)
	if !ok {
		// Task has credentials id set, but credentials manager is unaware of
		// the id. This should never happen as the payload handler sets
		// credentialsId for the task after adding credentials to the
		// credentials manager
		return &apierrors.HostConfigError{Msg: "Unable to get execution role credentials for task"}
	}
	credentialsEndpointRelativeURI := executionRoleCredentials.IAMRoleCredentials.GenerateCredentialsEndpointRelativeURI()
	if hostConfig.LogConfig.Config == nil {
		hostConfig.LogConfig.Config = map[string]string{}
	}
	hostConfig.LogConfig.Config[awslogsCredsEndpointOpt] = credentialsEndpointRelativeURI
	return nil
}

func (task *Task) dockerHostConfig(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer, apiVersion dockerclient.DockerVersion) (*dockercontainer.HostConfig, *apierrors.HostConfigError) {
	dockerLinkArr, err := task.dockerLinks(container, dockerContainerMap)
	if err != nil {
		return nil, &apierrors.HostConfigError{Msg: err.Error()}
	}

	dockerPortMap := task.dockerPortMap(container)

	volumesFrom, err := task.dockerVolumesFrom(container, dockerContainerMap)
	if err != nil {
		return nil, &apierrors.HostConfigError{Msg: err.Error()}
	}

	binds, err := task.dockerHostBinds(container)
	if err != nil {
		return nil, &apierrors.HostConfigError{Msg: err.Error()}
	}

	resources := task.getDockerResources(container)

	// Populate hostConfig
	hostConfig := &dockercontainer.HostConfig{
		Links:        dockerLinkArr,
		Binds:        binds,
		PortBindings: dockerPortMap,
		VolumesFrom:  volumesFrom,
		Resources:    resources,
	}

	if task.isGPUEnabled() && task.shouldRequireNvidiaRuntime(container) {
		if task.NvidiaRuntime == "" {
			return nil, &apierrors.HostConfigError{Msg: "Runtime is not set for GPU containers"}
		}
		seelog.Debugf("Setting runtime as %s for container %s", task.NvidiaRuntime, container.Name)
		hostConfig.Runtime = task.NvidiaRuntime
	}

	if container.DockerConfig.HostConfig != nil {
		err := json.Unmarshal([]byte(*container.DockerConfig.HostConfig), hostConfig)
		if err != nil {
			return nil, &apierrors.HostConfigError{Msg: "Unable to decode given host config: " + err.Error()}
		}
	}

	err = task.platformHostConfigOverride(hostConfig)
	if err != nil {
		return nil, &apierrors.HostConfigError{Msg: err.Error()}
	}

	// Determine if network mode should be overridden and override it if needed
	ok, networkMode := task.shouldOverrideNetworkMode(container, dockerContainerMap)
	if ok {
		hostConfig.NetworkMode = dockercontainer.NetworkMode(networkMode)
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
	}

	ok, pidMode := task.shouldOverridePIDMode(container, dockerContainerMap)
	if ok {
		hostConfig.PidMode = dockercontainer.PidMode(pidMode)
	}

	ok, ipcMode := task.shouldOverrideIPCMode(container, dockerContainerMap)
	if ok {
		hostConfig.IpcMode = dockercontainer.IpcMode(ipcMode)
	}

	return hostConfig, nil
}

// Requires an *apicontainer.Container and returns the Resources for the HostConfig struct
func (task *Task) getDockerResources(container *apicontainer.Container) dockercontainer.Resources {
	// Convert MB to B and set Memory
	dockerMem := int64(container.Memory * 1024 * 1024)
	if dockerMem != 0 && dockerMem < apicontainer.DockerContainerMinimumMemoryInBytes {
		seelog.Warnf("Task %s container %s memory setting is too low, increasing to %d bytes",
			task.Arn, container.Name, apicontainer.DockerContainerMinimumMemoryInBytes)
		dockerMem = apicontainer.DockerContainerMinimumMemoryInBytes
	}
	// Set CPUShares
	cpuShare := task.dockerCPUShares(container.CPU)
	resources := dockercontainer.Resources{
		Memory:    dockerMem,
		CPUShares: cpuShare,
	}
	return resources
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
	if !task.IsNetworkModeAWSVPC() {
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
	return true, dockerMappingContainerPrefix + pauseContainer.DockerID
}

// overrideDNS overrides a container's host config if the following conditions are
// true:
// 1. Task has an ENI associated with it
// 2. ENI has custom DNS IPs and search list associated with it
// This should only be done for the pause container as other containers inherit
// /etc/resolv.conf of this container (they share the network namespace)
func (task *Task) overrideDNS(hostConfig *dockercontainer.HostConfig) *dockercontainer.HostConfig {
	eni := task.GetPrimaryENI()
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
func (task *Task) applyENIHostname(dockerConfig *dockercontainer.Config) *dockercontainer.Config {
	eni := task.GetPrimaryENI()
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
	eni := task.GetPrimaryENI()
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

// shouldOverridePIDMode returns true if the PIDMode of the container needs
// to be overridden. It also returns the override string in this case. It returns
// false otherwise
func (task *Task) shouldOverridePIDMode(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer) (bool, string) {
	// If the container is an internal container (ContainerEmptyHostVolume,
	// ContainerCNIPause, or ContainerNamespacePause), then PID namespace for
	// the container itself should be private (default Docker option)
	if container.IsInternal() {
		return false, ""
	}

	switch task.getPIDMode() {
	case pidModeHost:
		return true, pidModeHost

	case pidModeTask:
		pauseCont, ok := task.ContainerByName(NamespacePauseContainerName)
		if !ok {
			seelog.Criticalf("Namespace Pause container not found in the task: %s; Setting Task's Desired Status to Stopped", task.Arn)
			task.SetDesiredStatus(apitaskstatus.TaskStopped)
			return false, ""
		}
		pauseDockerID, ok := dockerContainerMap[pauseCont.Name]
		if !ok || pauseDockerID == nil {
			// Docker container shouldn't be nil or not exist if the Container definition within task exists; implies code-bug
			seelog.Criticalf("Namespace Pause docker container not found in the task: %s; Setting Task's Desired Status to Stopped", task.Arn)
			task.SetDesiredStatus(apitaskstatus.TaskStopped)
			return false, ""
		}
		return true, dockerMappingContainerPrefix + pauseDockerID.DockerID

		// If PIDMode is not Host or Task, then no need to override
	default:
		return false, ""
	}
}

// shouldOverrideIPCMode returns true if the IPCMode of the container needs
// to be overridden. It also returns the override string in this case. It returns
// false otherwise
func (task *Task) shouldOverrideIPCMode(container *apicontainer.Container, dockerContainerMap map[string]*apicontainer.DockerContainer) (bool, string) {
	// All internal containers do not need the same IPCMode. The NamespaceContainerPause
	// needs to be "shareable" if ipcMode is "task". All other internal containers should
	// defer to the Docker daemon default option (either shareable or private depending on
	// version and configuration)
	if container.IsInternal() {
		if container.Type == apicontainer.ContainerNamespacePause {
			// Setting NamespaceContainerPause to be sharable with other containers
			if task.getIPCMode() == ipcModeTask {
				return true, ipcModeSharable
			}
		}
		// Defaulting to Docker daemon default option
		return false, ""
	}

	switch task.getIPCMode() {
	// No IPCMode provided in Task Definition, no need to override
	case "":
		return false, ""

		// IPCMode is none - container will have own private namespace with /dev/shm not mounted
	case ipcModeNone:
		return true, ipcModeNone

	case ipcModeHost:
		return true, ipcModeHost

	case ipcModeTask:
		pauseCont, ok := task.ContainerByName(NamespacePauseContainerName)
		if !ok {
			seelog.Criticalf("Namespace Pause container not found in the task: %s; Setting Task's Desired Status to Stopped", task.Arn)
			task.SetDesiredStatus(apitaskstatus.TaskStopped)
			return false, ""
		}
		pauseDockerID, ok := dockerContainerMap[pauseCont.Name]
		if !ok || pauseDockerID == nil {
			// Docker container shouldn't be nill or not exist if the Container definition within task exists; implies code-bug
			seelog.Criticalf("Namespace Pause container not found in the task: %s; Setting Task's Desired Status to Stopped", task.Arn)
			task.SetDesiredStatus(apitaskstatus.TaskStopped)
			return false, ""
		}
		return true, dockerMappingContainerPrefix + pauseDockerID.DockerID

	default:
		return false, ""
	}
}

func (task *Task) initializeContainerOrderingForVolumes() error {
	for _, container := range task.Containers {
		if len(container.VolumesFrom) > 0 {
			for _, volume := range container.VolumesFrom {
				if _, ok := task.ContainerByName(volume.SourceContainer); !ok {
					return fmt.Errorf("could not find container with name %s", volume.SourceContainer)
				}
				dependOn := apicontainer.DependsOn{ContainerName: volume.SourceContainer, Condition: ContainerOrderingCreateCondition}
				container.SetDependsOn(append(container.GetDependsOn(), dependOn))
			}
		}
	}
	return nil
}

func (task *Task) initializeContainerOrderingForLinks() error {
	for _, container := range task.Containers {
		if len(container.Links) > 0 {
			for _, link := range container.Links {
				linkParts := strings.Split(link, ":")
				if len(linkParts) > 2 {
					return fmt.Errorf("Invalid link format")
				}
				linkName := linkParts[0]
				if _, ok := task.ContainerByName(linkName); !ok {
					return fmt.Errorf("could not find container with name %s", linkName)
				}
				dependOn := apicontainer.DependsOn{ContainerName: linkName, Condition: ContainerOrderingStartCondition}
				container.SetDependsOn(append(container.GetDependsOn(), dependOn))
			}
		}
	}
	return nil
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

func (task *Task) dockerPortMap(container *apicontainer.Container) nat.PortMap {
	dockerPortMap := nat.PortMap{}

	for _, portBinding := range container.Ports {
		dockerPort := nat.Port(strconv.Itoa(int(portBinding.ContainerPort)) + "/" + portBinding.Protocol.String())
		currentMappings, existing := dockerPortMap[dockerPort]
		if existing {
			dockerPortMap[dockerPort] = append(currentMappings, nat.PortBinding{HostPort: strconv.Itoa(int(portBinding.HostPort))})
		} else {
			dockerPortMap[dockerPort] = []nat.PortBinding{{HostPort: strconv.Itoa(int(portBinding.HostPort))}}
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
			seelog.Infof("api/task: Updating task desired status to stopped because of container: [%s]; task: [%s]",
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

// AddTaskENI adds ENI information to the task.
func (task *Task) AddTaskENI(eni *apieni.ENI) {
	task.lock.Lock()
	defer task.lock.Unlock()

	if task.ENIs == nil {
		task.ENIs = make([]*apieni.ENI, 0)
	}
	task.ENIs = append(task.ENIs, eni)
}

// GetTaskENIs returns the list of ENIs for the task.
func (task *Task) GetTaskENIs() []*apieni.ENI {
	// TODO: what's the point of locking if we are returning a pointer?
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.ENIs
}

// GetPrimaryENI returns the primary ENI of the task. Since ACS can potentially send
// multiple ENIs to the agent, the first ENI in the list is considered as the primary ENI.
func (task *Task) GetPrimaryENI() *apieni.ENI {
	task.lock.RLock()
	defer task.lock.RUnlock()

	if len(task.ENIs) == 0 {
		return nil
	}
	return task.ENIs[0]
}

// SetAppMesh sets the app mesh config of the task
func (task *Task) SetAppMesh(appMesh *apiappmesh.AppMesh) {
	task.lock.Lock()
	defer task.lock.Unlock()

	task.AppMesh = appMesh
}

// GetAppMesh returns the app mesh config of the task
func (task *Task) GetAppMesh() *apiappmesh.AppMesh {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.AppMesh
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
		res += fmt.Sprintf("%s (%s->%s),",
			container.Name,
			container.GetKnownStatus().String(),
			container.GetDesiredStatus().String())
	}
	res += "]"

	if len(task.ENIs) > 0 {
		res += " ENIs: ["
		for _, eni := range task.ENIs {
			res += fmt.Sprintf("%s,", eni.String())
		}
		res += "]"
	}

	return res
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

		// Converts the first letter of terminal reason into capital letter
		words := strings.Fields(reason)
		words[0] = strings.Title(words[0])
		task.terminalReason = strings.Join(words, " ")
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

// getSSMSecretsResource retrieves ssmsecret resource from resource map
func (task *Task) getSSMSecretsResource() ([]taskresource.TaskResource, bool) {
	task.lock.RLock()
	defer task.lock.RUnlock()

	res, ok := task.ResourcesMapUnsafe[ssmsecret.ResourceName]
	return res, ok
}

// PopulateSecrets appends secrets to container's env var map and hostconfig section
func (task *Task) PopulateSecrets(hostConfig *dockercontainer.HostConfig, container *apicontainer.Container) *apierrors.DockerClientConfigError {
	var ssmRes *ssmsecret.SSMSecretResource
	var asmRes *asmsecret.ASMSecretResource

	if container.ShouldCreateWithSSMSecret() {
		resource, ok := task.getSSMSecretsResource()
		if !ok {
			return &apierrors.DockerClientConfigError{Msg: "task secret data: unable to fetch SSM Secrets resource"}
		}
		ssmRes = resource[0].(*ssmsecret.SSMSecretResource)
	}

	if container.ShouldCreateWithASMSecret() {
		resource, ok := task.getASMSecretsResource()
		if !ok {
			return &apierrors.DockerClientConfigError{Msg: "task secret data: unable to fetch ASM Secrets resource"}
		}
		asmRes = resource[0].(*asmsecret.ASMSecretResource)
	}

	populateContainerSecrets(hostConfig, container, ssmRes, asmRes)
	return nil
}

func populateContainerSecrets(hostConfig *dockercontainer.HostConfig, container *apicontainer.Container,
	ssmRes *ssmsecret.SSMSecretResource, asmRes *asmsecret.ASMSecretResource) {
	envVars := make(map[string]string)

	logDriverTokenName := ""
	logDriverTokenSecretValue := ""

	for _, secret := range container.Secrets {
		secretVal := ""

		if secret.Provider == apicontainer.SecretProviderSSM {
			k := secret.GetSecretResourceCacheKey()
			if secretValue, ok := ssmRes.GetCachedSecretValue(k); ok {
				secretVal = secretValue
			}
		}

		if secret.Provider == apicontainer.SecretProviderASM {
			k := secret.GetSecretResourceCacheKey()
			if secretValue, ok := asmRes.GetCachedSecretValue(k); ok {
				secretVal = secretValue
			}
		}

		if secret.Type == apicontainer.SecretTypeEnv {
			envVars[secret.Name] = secretVal
			continue
		}

		if secret.Target == apicontainer.SecretTargetLogDriver {
			// Log driver secrets for container using awsfirelens log driver won't be saved in log config and passed to
			// Docker here. They will only be used to configure the firelens container.
			if container.GetLogDriver() == firelensDriverName {
				continue
			}

			logDriverTokenName = secret.Name
			logDriverTokenSecretValue = secretVal

			// Check if all the name and secret value for the log driver do exist
			// And add the secret value for this log driver into container's HostConfig
			if hostConfig.LogConfig.Type != "" && logDriverTokenName != "" && logDriverTokenSecretValue != "" {
				if hostConfig.LogConfig.Config == nil {
					hostConfig.LogConfig.Config = map[string]string{}
				}
				hostConfig.LogConfig.Config[logDriverTokenName] = logDriverTokenSecretValue
			}
		}
	}

	container.MergeEnvironmentVariables(envVars)
}

// PopulateSecretLogOptionsToFirelensContainer collects secret log option values for awsfirelens log driver from task
// resource and specified then as envs of firelens container. Firelens container will use the envs to resolve config
// file variables constructed for secret log options when loading the config file.
func (task *Task) PopulateSecretLogOptionsToFirelensContainer(firelensContainer *apicontainer.Container) *apierrors.DockerClientConfigError {
	firelensENVs := make(map[string]string)

	var ssmRes *ssmsecret.SSMSecretResource
	var asmRes *asmsecret.ASMSecretResource

	resource, ok := task.getSSMSecretsResource()
	if ok {
		ssmRes = resource[0].(*ssmsecret.SSMSecretResource)
	}

	resource, ok = task.getASMSecretsResource()
	if ok {
		asmRes = resource[0].(*asmsecret.ASMSecretResource)
	}

	for _, container := range task.Containers {
		if container.GetLogDriver() != firelensDriverName {
			continue
		}

		logDriverSecretData, err := collectLogDriverSecretData(container.Secrets, ssmRes, asmRes)
		if err != nil {
			return &apierrors.DockerClientConfigError{
				Msg: fmt.Sprintf("unable to generate config to create firelens container: %v", err),
			}
		}

		idx := task.GetContainerIndex(container.Name)
		if idx < 0 {
			return &apierrors.DockerClientConfigError{
				Msg: fmt.Sprintf("unable to generate config to create firelens container because container %s is not found in task", container.Name),
			}
		}
		for key, value := range logDriverSecretData {
			envKey := fmt.Sprintf(firelensConfigVarFmt, key, idx)
			firelensENVs[envKey] = value
		}
	}

	firelensContainer.MergeEnvironmentVariables(firelensENVs)
	return nil
}

// collectLogDriverSecretData collects all the secret values for log driver secrets.
func collectLogDriverSecretData(secrets []apicontainer.Secret, ssmRes *ssmsecret.SSMSecretResource,
	asmRes *asmsecret.ASMSecretResource) (map[string]string, error) {
	secretData := make(map[string]string)
	for _, secret := range secrets {
		if secret.Target != apicontainer.SecretTargetLogDriver {
			continue
		}

		secretVal := ""
		cacheKey := secret.GetSecretResourceCacheKey()
		if secret.Provider == apicontainer.SecretProviderSSM {
			if ssmRes == nil {
				return nil, errors.Errorf("missing secret value for secret %s", secret.Name)
			}

			if secretValue, ok := ssmRes.GetCachedSecretValue(cacheKey); ok {
				secretVal = secretValue
			}
		} else if secret.Provider == apicontainer.SecretProviderASM {
			if asmRes == nil {
				return nil, errors.Errorf("missing secret value for secret %s", secret.Name)
			}

			if secretValue, ok := asmRes.GetCachedSecretValue(cacheKey); ok {
				secretVal = secretValue
			}
		}

		secretData[secret.Name] = secretVal
	}

	return secretData, nil
}

// getASMSecretsResource retrieves asmsecret resource from resource map
func (task *Task) getASMSecretsResource() ([]taskresource.TaskResource, bool) {
	task.lock.RLock()
	defer task.lock.RUnlock()

	res, ok := task.ResourcesMapUnsafe[asmsecret.ResourceName]
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

// Retrieves a Task's PIDMode
func (task *Task) getPIDMode() string {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.PIDMode
}

// Retrieves a Task's IPCMode
func (task *Task) getIPCMode() string {
	task.lock.RLock()
	defer task.lock.RUnlock()

	return task.IPCMode
}

// AssociationByTypeAndContainer gets a list of names of all the associations associated with a container and of a
// certain type
func (task *Task) AssociationsByTypeAndContainer(associationType, containerName string) []string {
	task.lock.RLock()
	defer task.lock.RUnlock()

	var associationNames []string
	for _, association := range task.Associations {
		if association.Type == associationType {
			for _, associatedContainerName := range association.Containers {
				if associatedContainerName == containerName {
					associationNames = append(associationNames, association.Name)
				}
			}
		}
	}

	return associationNames
}

// AssociationByTypeAndName gets an association of a certain type and name
func (task *Task) AssociationByTypeAndName(associationType, associationName string) (*Association, bool) {
	task.lock.RLock()
	defer task.lock.RUnlock()

	for _, association := range task.Associations {
		if association.Type == associationType && association.Name == associationName {
			return &association, true
		}
	}

	return nil, false
}

// GetContainerIndex returns the index of the container in the container list. This doesn't count internal container.
func (task *Task) GetContainerIndex(containerName string) int {
	task.lock.RLock()
	defer task.lock.RUnlock()

	idx := 0
	for _, container := range task.Containers {
		if container.IsInternal() {
			continue
		}
		if container.Name == containerName {
			return idx
		}
		idx++
	}
	return -1
}
