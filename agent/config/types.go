// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

package config

import (
	"time"

	cniTypes "github.com/containernetworking/cni/pkg/types"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
)

// ImagePullBehaviorType is an enum variable type corresponding to different agent pull
// behaviors including default, always, never and once.
type ImagePullBehaviorType int8

// ContainerInstancePropagateTagsFromType is an enum variable type corresponding to different
// ways to propagate tags, it includes none (default) and ec2_instance.
type ContainerInstancePropagateTagsFromType int8

type Config struct {
	// DEPRECATED
	// ClusterArn is the Name or full ARN of a Cluster to register into. It has
	// been deprecated (and will eventually be removed) in favor of Cluster
	ClusterArn string `deprecated:"Please use Cluster instead"`

	// Cluster can either be the Name or full ARN of a Cluster. This is the
	// cluster the agent should register this ContainerInstance into. If this
	// value is not set, it will default to "default"
	Cluster string `trim:"true"`
	// APIEndpoint is the endpoint, such as "ecs.us-east-1.amazonaws.com", to
	// make calls against. If this value is not set, it will default to the
	// endpoint for your current AWSRegion
	APIEndpoint string `trim:"true"`
	// DockerEndpoint is the address the agent will attempt to connect to the
	// Docker daemon at. This should have the same value as "DOCKER_HOST"
	// normally would to interact with the daemon. It defaults to
	// unix:///var/run/docker.sock
	DockerEndpoint string
	// AWSRegion is the region to run in (such as "us-east-1"). This value will
	// be inferred from the EC2 metadata service, but if it cannot be found this
	// will be fatal.
	AWSRegion string `missing:"fatal" trim:"true"`

	// ReservedPorts is an array of ports which should be registered as
	// unavailable. If not set, they default to [22,2375,2376,51678].
	ReservedPorts []uint16
	// ReservedPortsUDP is an array of UDP ports which should be registered as
	// unavailable. If not set, it defaults to [].
	ReservedPortsUDP []uint16

	// DataDir is the directory data is saved to in order to preserve state
	// across agent restarts.
	// It is also used to keep the metadata of containers managed by the agent
	DataDir string
	// DataDirOnHost is the directory in the instance from which we mount
	// DataDir to the ecs-agent container and to agent managed containers
	DataDirOnHost string
	// Checkpoint configures whether data should be periodically to a checkpoint
	// file, in DataDir, such that on instance or agent restarts it will resume
	// as the same ContainerInstance. It defaults to false.
	Checkpoint BooleanDefaultFalse

	// EngineAuthType configures what type of data is in EngineAuthData.
	// Supported types, right now, can be found in the dockerauth package: https://godoc.org/github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerauth
	EngineAuthType string `trim:"true"`
	// EngineAuthData contains authentication data. Please see the documentation
	// for EngineAuthType for more information.
	EngineAuthData *SensitiveRawMessage

	// UpdatesEnabled specifies whether updates should be applied to this agent.
	// Default true
	UpdatesEnabled BooleanDefaultFalse
	// UpdateDownloadDir specifies where new agent versions should be placed
	// within the container in order for the external updating process to
	// correctly handle them.
	UpdateDownloadDir string

	// DisableMetrics configures whether task utilization metrics should be
	// sent to the ECS telemetry endpoint
	DisableMetrics BooleanDefaultFalse

	// PollMetrics configures whether metrics are constantly streamed for each container or
	// polled on interval instead.
	PollMetrics BooleanDefaultFalse

	// PollingMetricsWaitDuration configures how long a container should wait before polling metrics
	// again when PollMetrics is set to true
	PollingMetricsWaitDuration time.Duration

	// DisableDockerHealthCheck configures whether container health feature was enabled
	// on the instance
	DisableDockerHealthCheck BooleanDefaultFalse

	// ReservedMemory specifies Reduction, in MiB, of the memory capacity of the instance
	// that is reported to Amazon ECS. Used by Amazon ECS when placing tasks on container instances.
	// This doesn't reserve memory usage on the instance
	ReservedMemory uint16

	// DockerStopTimeout specifies the amount of time before a SIGKILL is issued to
	// containers managed by ECS
	DockerStopTimeout time.Duration

	// ContainerStartTimeout specifies the amount of time to wait to start a container
	ContainerStartTimeout time.Duration

	// ContainerCreateTimeout specifies the amount of time to wait to create a container
	ContainerCreateTimeout time.Duration

	// DependentContainersPullUpfront specifies whether pulling images upfront should be applied to this agent.
	// Default false
	DependentContainersPullUpfront BooleanDefaultFalse

	// ImagePullInactivityTimeout is here to override the amount of time to wait when pulling and extracting a container
	ImagePullInactivityTimeout time.Duration

	//ImagePullTimeout is here to override the timeout for PullImage API
	ImagePullTimeout time.Duration

	// AvailableLoggingDrivers specifies the logging drivers available for use
	// with Docker.  If not set, it defaults to ["json-file","none"].
	AvailableLoggingDrivers []dockerclient.LoggingDriver

	// PrivilegedDisabled specified whether the Agent is capable of launching
	// tasks with privileged containers
	PrivilegedDisabled BooleanDefaultFalse

	// SELinxuCapable specifies whether the Agent is capable of using SELinux
	// security options
	SELinuxCapable BooleanDefaultFalse

	// AppArmorCapable specifies whether the Agent is capable of using AppArmor
	// security options
	AppArmorCapable BooleanDefaultFalse

	// TaskCleanupWaitDuration specifies the time to wait after a task is stopped
	// until cleanup of task resources is started.
	TaskCleanupWaitDuration time.Duration

	// TaskCleanupWaitDurationJitter specifies a jitter for task cleanup wait duration.
	// When specified to a non-zero duration (default is zero), the task cleanup wait duration for each task
	// will be a random duration between [TaskCleanupWaitDuration, TaskCleanupWaitDuration +
	// TaskCleanupWaitDurationJitter].
	TaskCleanupWaitDurationJitter time.Duration

	// TaskIAMRoleEnabled specifies if the Agent is capable of launching
	// tasks with IAM Roles.
	TaskIAMRoleEnabled BooleanDefaultFalse

	// DeleteNonECSImagesEnabled specifies if the Agent can delete the cached, unused non-ecs images.
	DeleteNonECSImagesEnabled BooleanDefaultFalse

	// TaskCPUMemLimit specifies if Agent can launch a task with a hierarchical cgroup
	TaskCPUMemLimit BooleanDefaultTrue

	// CredentialsAuditLogFile specifies the path/filename of the audit log.
	CredentialsAuditLogFile string

	// CredentialsAuditLogEnabled specifies whether audit logging is disabled.
	CredentialsAuditLogDisabled bool

	// TaskIAMRoleEnabledForNetworkHost specifies if the Agent is capable of launching
	// tasks with IAM Roles when networkMode is set to 'host'
	TaskIAMRoleEnabledForNetworkHost bool

	// TaskENIEnabled specifies if the Agent is capable of launching task within
	// defined EC2 networks
	TaskENIEnabled BooleanDefaultFalse

	// ENITrunkingEnabled specifies if the Agent is enabled to launch awsvpc
	// task with ENI Trunking
	ENITrunkingEnabled BooleanDefaultTrue

	// ImageCleanupDisabled specifies whether the Agent will periodically perform
	// automated image cleanup
	ImageCleanupDisabled BooleanDefaultFalse

	// MinimumImageDeletionAge specifies the minimum time since it was pulled
	// before it can be deleted
	MinimumImageDeletionAge time.Duration

	// NonECSMinimumImageDeletionAge specifies the minimum time since non ecs images created before it can be deleted
	NonECSMinimumImageDeletionAge time.Duration

	// ImageCleanupInterval specifies the time to wait before performing the image
	// cleanup since last time it was executed
	ImageCleanupInterval time.Duration

	// NumImagesToDeletePerCycle specifies the num of image to delete every time
	// when Agent performs cleanup
	NumImagesToDeletePerCycle int

	// NumNonECSContainersToDeletePerCycle specifies the num of NonECS containers to delete every time
	// when Agent performs cleanup
	NumNonECSContainersToDeletePerCycle int

	// ImagePullBehavior specifies the agent's behavior for pulling image and loading
	// local Docker image cache
	ImagePullBehavior ImagePullBehaviorType

	// InstanceAttributes contains key/value pairs representing
	// attributes to be associated with this instance within the
	// ECS service and used to influence behavior such as launch
	// placement.
	InstanceAttributes map[string]string

	// Set if clients validate ssl certificates. Used mainly for testing
	AcceptInsecureCert bool `json:"-"`

	// CNIPluginsPath is the path for the cni plugins
	CNIPluginsPath string

	// PauseContainerTarballPath is the path to the pause container tarball
	PauseContainerTarballPath string

	// PauseContainerImageName is the name for the pause container image.
	// Setting this value to be different from the default will disable loading
	// the image from the tarball; the referenced image must already be loaded.
	PauseContainerImageName string

	// PauseContainerTag is the tag for the pause container image.
	// Setting this value to be different from the default will disable loading
	// the image from the tarball; the referenced image must already be loaded.
	PauseContainerTag string

	// PrometheusMetricsEnabled configures whether Agent metrics should be
	// collected and published to the specified endpoint. This is disabled by
	// default.
	PrometheusMetricsEnabled bool

	// AWSVPCBlockInstanceMetdata specifies if InstanceMetadata endpoint should be blocked
	// for tasks that are launched with network mode "awsvpc" when ECS_AWSVPC_BLOCK_IMDS=true
	AWSVPCBlockInstanceMetdata BooleanDefaultFalse

	// OverrideAWSVPCLocalIPv4Address overrides the local IPv4 address chosen
	// for a task using the `awsvpc` networking mode. Using this configuration
	// will limit you to running one `awsvpc` task at a time. IPv4 addresses
	// must be specified in decimal-octet form and also specify the subnet
	// size (e.g., "169.254.172.42/22").
	OverrideAWSVPCLocalIPv4Address *cniTypes.IPNet

	// AWSVPCAdditionalLocalRoutes allows the specification of routing table
	// entries that will be added in the task's network namespace via the
	// instance bridge interface rather than via the ENI.
	AWSVPCAdditionalLocalRoutes []cniTypes.IPNet

	// ContainerMetadataEnabled specifies if the agent should provide a metadata
	// file for containers.
	ContainerMetadataEnabled BooleanDefaultFalse

	// OverrideAWSLogsExecutionRole is config option used to enable awslogs
	// driver authentication over the task's execution role
	OverrideAWSLogsExecutionRole BooleanDefaultFalse

	// CgroupPath is the path expected by the agent, defaults to
	// '/sys/fs/cgroup'
	CgroupPath string

	// PlatformVariables consists of configuration variables specific to linux/windows
	PlatformVariables PlatformVariables

	// TaskMetadataSteadyStateRate specifies the steady state throttle for the task metadata endpoint
	TaskMetadataSteadyStateRate int

	// TaskMetadataBurstRate specifies the burst rate throttle for the task metadata endpoint
	TaskMetadataBurstRate int

	// SharedVolumeMatchFullConfig is config option used to short-circuit volume validation against a
	// provisioned volume, if false (default). If true, we perform deep comparison including driver options
	// and labels. For comparing shared volume across 2 instances, this should be set to false as docker's
	// default behavior is to match name only, and does not propagate driver options and labels through volume drivers.
	SharedVolumeMatchFullConfig BooleanDefaultFalse

	// NoIID when set to true, specifies that the agent should not register the instance
	// with instance identity document. This is required in order to accomodate scenarios in
	// which ECS agent tries to register the instance where the instance id document is
	// not available or needed
	NoIID bool

	// ContainerInstancePropagateTagsFrom when set to "ec2_instance", agent will call EC2 API to
	// get the tags and register them through RegisterContainerInstance call.
	// When set to "none" (or any other string), no API call will be made.
	ContainerInstancePropagateTagsFrom ContainerInstancePropagateTagsFromType

	// ContainerInstanceTags contains key/value pairs representing
	// tags extracted from config file and will be associated with this instance
	// through RegisterContainerInstance call. Tags with the same keys from DescribeTags
	// API call will be overridden.
	ContainerInstanceTags map[string]string

	// GPUSupportEnabled specifies if the Agent is capable of launching GPU tasks
	GPUSupportEnabled bool
	// InferentiaSupportEnabled specifies whether the built-in support for inferentia task is enabled.
	InferentiaSupportEnabled bool

	// ImageCleanupExclusionList is the list of image names customers want to keep for their own use and delete automatically
	ImageCleanupExclusionList []string

	// NvidiaRuntime is the runtime to be used for passing Nvidia GPU devices to containers
	NvidiaRuntime string `trim:"true"`

	// TaskMetadataAZDisabled specifies if availability zone should be disabled in Task Metadata endpoint
	TaskMetadataAZDisabled bool

	// ENIPauseContainerCleanupDelaySeconds specifies how long to wait before cleaning up the pause container after all
	// other containers have stopped.
	ENIPauseContainerCleanupDelaySeconds int

	// CgroupCPUPeriod is config option to set different CFS quota and period values in microsecond, defaults to 100 ms
	CgroupCPUPeriod time.Duration

	// SpotInstanceDrainingEnabled, if true, agent will poll the container instance's metadata endpoint for an ec2 spot
	//   instance termination notice. If EC2 sends a spot termination notice, then agent will set the instance's state
	//   to DRAINING, which gracefully shuts down all running tasks on the instance.
	// If the instance is not spot then the poller will still run but it will never receive a termination notice.
	// Defaults to false.
	// see https://docs.aws.amazon.com/AmazonECS/latest/developerguide/container-instance-draining.html
	SpotInstanceDrainingEnabled BooleanDefaultFalse

	// GMSACapable is the config option to indicate if gMSA is supported.
	// It should be enabled by default only if the container instance is part of a valid active directory domain.
	GMSACapable BooleanDefaultFalse

	// GMSADomainlessCapable is the config option to indicate if gMSA domainless is supported.
	// It should be enabled by if the container instance has a plugin to support active directory authentication.
	GMSADomainlessCapable BooleanDefaultFalse

	// VolumePluginCapabilities specifies the capabilities of the ecs volume plugin.
	VolumePluginCapabilities []string

	// FSxWindowsFileServerCapable is the config option to indicate if fsxWindowsFileServer is supported.
	// It is enabled by default on Windows and can be overridden by the ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED environment variable.
	FSxWindowsFileServerCapable BooleanDefaultTrue

	// External specifies whether agent is running on external compute capacity (i.e. outside of aws).
	External BooleanDefaultFalse

	// InstanceENIDNSServerList stores the list of DNS servers for the primary instance ENI.
	// Currently, this field is only populated for Windows and is used during task networking setup.
	InstanceENIDNSServerList []string

	// RuntimeStatsLogFile stores the path where the golang runtime stats are periodically logged
	RuntimeStatsLogFile string

	// EnableRuntimeStats specifies if pprof should be enabled through the agent introspection port. By default, this configuration
	// is set to false and can be overridden by means of the ECS_ENABLE_RUNTIME_STATS environment variable.
	EnableRuntimeStats BooleanDefaultFalse

	// ShouldExcludeIPv6PortBinding specifies whether agent should exclude IPv6 port bindings reported from docker. This configuration
	// is set to true by default, and can be overridden by the ECS_EXCLUDE_IPV6_PORTBINDING environment variable. This is a workaround
	// for docker's bug as detailed in https://github.com/aws/amazon-ecs-agent/issues/2870.
	ShouldExcludeIPv6PortBinding BooleanDefaultTrue

	// WarmPoolsSupport specifies whether the agent should poll IMDS to check the target lifecycle state for a starting
	// instance
	WarmPoolsSupport BooleanDefaultFalse

	// DynamicHostPortRange specifies the dynamic host port range that the agent
	// uses to assign host ports from, for a container port range mapping.
	// This defaults to the platform specific ephemeral host port range
	DynamicHostPortRange string

	// TaskPidsLimit specifies the per-task pids limit cgroup setting for each
	// task launched on this container instance. This setting maps to the pids.max
	// cgroup setting at the ECS task level.
	// see https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#pid
	TaskPidsLimit int
}
