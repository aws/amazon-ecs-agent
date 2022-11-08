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

package container

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	// defaultContainerSteadyStateStatus defines the container status at
	// which the container is assumed to be in steady state. It is set
	// to 'ContainerRunning' unless overridden
	defaultContainerSteadyStateStatus = apicontainerstatus.ContainerRunning

	// awslogsAuthExecutionRole is the string value passed in the task payload
	// that specifies that the log driver should be authenticated using the
	// execution role
	awslogsAuthExecutionRole = "ExecutionRole"

	// DockerHealthCheckType is the type of container health check provided by docker
	DockerHealthCheckType = "docker"

	// AuthTypeECR is to use image pull auth over ECR
	AuthTypeECR = "ecr"

	// AuthTypeASM is to use image pull auth over AWS Secrets Manager
	AuthTypeASM = "asm"

	// MetadataURIEnvironmentVariableName defines the name of the environment
	// variable in containers' config, which can be used by the containers to access the
	// v3 metadata endpoint
	MetadataURIEnvironmentVariableName = "ECS_CONTAINER_METADATA_URI"

	// MetadataURIEnvVarNameV4 defines the name of the environment
	// variable in containers' config, which can be used by the containers to access the
	// v4 metadata endpoint
	MetadataURIEnvVarNameV4 = "ECS_CONTAINER_METADATA_URI_V4"

	// MetadataURIFormat defines the URI format for v4 metadata endpoint
	MetadataURIFormatV4 = "http://169.254.170.2/v4/%s"

	// AgentURIEnvVarName defines the name of the environment variable
	// injected into containers that contains the Agent endpoints.
	AgentURIEnvVarName = "ECS_AGENT_URI"

	// AgentURIFormat defines the URI format for Agent endpoints
	AgentURIFormat = "http://169.254.170.2/api/%s"

	// SecretProviderSSM is to show secret provider being SSM
	SecretProviderSSM = "ssm"

	// SecretProviderASM is to show secret provider being ASM
	SecretProviderASM = "asm"

	// SecretTypeEnv is to show secret type being ENVIRONMENT_VARIABLE
	SecretTypeEnv = "ENVIRONMENT_VARIABLE"

	// TargetLogDriver is to show secret target being "LOG_DRIVER", the default will be "CONTAINER"
	SecretTargetLogDriver = "LOG_DRIVER"

	// neuronVisibleDevicesEnvVar is the env which indicates that the container wants to use inferentia devices.
	neuronVisibleDevicesEnvVar = "AWS_NEURON_VISIBLE_DEVICES"
)

var (
	// MetadataURIFormat defines the URI format for v3 metadata endpoint. Made as a var to be able to
	// overwrite it in test.
	MetadataURIFormat = "http://169.254.170.2/v3/%s"
)

// DockerConfig represents additional metadata about a container to run. It's
// remodeled from the `ecsacs` api model file. Eventually it should not exist
// once this remodeling is refactored out.
type DockerConfig struct {
	// Config is the configuration used to create container
	Config *string `json:"config"`
	// HostConfig is the configuration of container related to host resource
	HostConfig *string `json:"hostConfig"`
	// Version specifies the docker client API version to use
	Version *string `json:"version"`
}

// HealthStatus contains the health check result returned by docker
type HealthStatus struct {
	// Status is the container health status
	Status apicontainerstatus.ContainerHealthStatus `json:"status,omitempty"`
	// Since is the timestamp when container health status changed
	Since *time.Time `json:"statusSince,omitempty"`
	// ExitCode is the exitcode of health check if failed
	ExitCode int `json:"exitCode,omitempty"`
	// Output is the output of health check
	Output string `json:"output,omitempty"`
}

type ManagedAgentState struct {
	// ID of this managed agent state
	ID string `json:"id,omitempty"`
	// TODO: [ecs-exec] Change variable name from Status to KnownStatus in future PR to avoid noise
	// Status is the managed agent health status
	Status apicontainerstatus.ManagedAgentStatus `json:"status,omitempty"`
	// SentStatus is the managed agent sent status
	SentStatus apicontainerstatus.ManagedAgentStatus `json:"sentStatus,omitempty"`
	// Reason is a placeholder for failure messaging
	Reason string `json:"reason,omitempty"`
	// LastStartedAt is the timestamp when the status last went from PENDING->RUNNING
	LastStartedAt time.Time `json:"lastStartedAt,omitempty"`
	// Metadata holds metadata about the managed agent
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// InitFailed indicates if exec agent initialization failed
	InitFailed bool `json:"initFailed,omitempty"`
}

type ManagedAgent struct {
	ManagedAgentState
	// Name is the name of this managed agent. This name is streamed down from ACS.
	Name string `json:"name,omitempty"`
	// Properties of this managed agent. Properties are streamed down from ACS.
	Properties map[string]string `json:"properties,omitempty"`
}

// Container is the internal representation of a container in the ECS agent
type Container struct {
	// Name is the name of the container specified in the task definition
	Name string
	// RuntimeID is the docker id of the container
	RuntimeID string
	// TaskARNUnsafe is the task ARN of the task that the container belongs to. Access should be
	// protected by lock i.e. via GetTaskARN and SetTaskARN.
	TaskARNUnsafe string `json:"taskARN"`
	// DependsOnUnsafe is the field which specifies the ordering for container startup and shutdown.
	DependsOnUnsafe []DependsOn `json:"dependsOn,omitempty"`
	// ManagedAgentsUnsafe presently contains only the executeCommandAgent
	ManagedAgentsUnsafe []ManagedAgent `json:"managedAgents,omitempty"`
	// V3EndpointID is a container identifier used to construct v3 metadata endpoint; it's unique among
	// all the containers managed by the agent
	V3EndpointID string
	// Image is the image name specified in the task definition
	Image string
	// ImageID is the local ID of the image used in the container
	ImageID string
	// ImageDigest is the sha-256 digest of the container image as pulled from the repository
	ImageDigest string
	// Command is the command to run in the container which is specified in the task definition
	Command []string
	// CPU is the cpu limitation of the container which is specified in the task definition
	CPU uint `json:"Cpu"`
	// GPUIDs is the list of GPU ids for a container
	GPUIDs []string
	// Memory is the memory limitation of the container which is specified in the task definition
	Memory uint
	// Links contains a list of containers to link, corresponding to docker option: --link
	Links []string
	// FirelensConfig contains configuration for a Firelens container
	FirelensConfig *FirelensConfig `json:"firelensConfiguration"`
	// VolumesFrom contains a list of container's volume to use, corresponding to docker option: --volumes-from
	VolumesFrom []VolumeFrom `json:"volumesFrom"`
	// MountPoints contains a list of volume mount paths
	MountPoints []MountPoint `json:"mountPoints"`
	// Ports contains a list of ports binding configuration
	Ports []PortBinding `json:"portMappings"`
	// Secrets contains a list of secret
	Secrets []Secret `json:"secrets"`
	// Essential denotes whether the container is essential or not
	Essential bool
	// EntryPoint is entrypoint of the container, corresponding to docker option: --entrypoint
	EntryPoint *[]string
	// Environment is the environment variable set in the container
	Environment map[string]string `json:"environment"`
	// EnvironmentFiles is the list of environmentFile used to populate environment variables
	EnvironmentFiles []EnvironmentFile `json:"environmentFiles"`
	// Overrides contains the configuration to override of a container
	Overrides ContainerOverrides `json:"overrides"`
	// DockerConfig is the configuration used to create the container
	DockerConfig DockerConfig `json:"dockerConfig"`
	// RegistryAuthentication is the auth data used to pull image
	RegistryAuthentication *RegistryAuthenticationData `json:"registryAuthentication"`
	// HealthCheckType is the mechanism to use for the container health check
	// currently it only supports 'DOCKER'
	HealthCheckType string `json:"healthCheckType,omitempty"`
	// Health contains the health check information of container health check
	Health HealthStatus `json:"-"`
	// LogsAuthStrategy specifies how the logs driver for the container will be
	// authenticated
	LogsAuthStrategy string
	// StartTimeout specifies the time value after which if a container has a dependency
	// on another container and the dependency conditions are 'SUCCESS', 'COMPLETE', 'HEALTHY',
	// then that dependency will not be resolved.
	StartTimeout uint
	// StopTimeout specifies the time value to be passed as StopContainer api call
	StopTimeout uint

	// lock is used for fields that are accessed and updated concurrently
	lock sync.RWMutex

	// DesiredStatusUnsafe represents the state where the container should go. Generally,
	// the desired status is informed by the ECS backend as a result of either
	// API calls made to ECS or decisions made by the ECS service scheduler,
	// though the agent may also set the DesiredStatusUnsafe if a different "essential"
	// container in the task exits. The DesiredStatus is almost always either
	// ContainerRunning or ContainerStopped.
	// NOTE: Do not access DesiredStatusUnsafe directly.  Instead, use `GetDesiredStatus`
	// and `SetDesiredStatus`.
	// TODO DesiredStatusUnsafe should probably be private with appropriately written
	// setter/getter.  When this is done, we need to ensure that the UnmarshalJSON
	// is handled properly so that the state storage continues to work.
	DesiredStatusUnsafe apicontainerstatus.ContainerStatus `json:"desiredStatus"`

	// KnownStatusUnsafe represents the state where the container is.
	// NOTE: Do not access `KnownStatusUnsafe` directly.  Instead, use `GetKnownStatus`
	// and `SetKnownStatus`.
	// TODO KnownStatusUnsafe should probably be private with appropriately written
	// setter/getter.  When this is done, we need to ensure that the UnmarshalJSON
	// is handled properly so that the state storage continues to work.
	KnownStatusUnsafe apicontainerstatus.ContainerStatus `json:"KnownStatus"`

	// TransitionDependenciesMap is a map of the dependent container status to other
	// dependencies that must be satisfied in order for this container to transition.
	TransitionDependenciesMap TransitionDependenciesMap `json:"TransitionDependencySet"`

	// SteadyStateDependencies is a list of containers that must be in "steady state" before
	// this one is created
	// Note: Current logic requires that the containers specified here are run
	// before this container can even be pulled.
	//
	// Deprecated: Use TransitionDependencySet instead. SteadyStateDependencies is retained for compatibility with old
	// state files.
	SteadyStateDependencies []string `json:"RunDependencies"`

	// Type specifies the container type. Except the 'Normal' type, all other types
	// are not directly specified by task definitions, but created by the agent. The
	// JSON tag is retained as this field's previous name 'IsInternal' for maintaining
	// backwards compatibility. Please see JSON parsing hooks for this type for more
	// details
	Type ContainerType `json:"IsInternal"`

	// AppliedStatus is the status that has been "applied" (e.g., we've called Pull,
	// Create, Start, or Stop) but we don't yet know that the application was successful.
	// No need to save it in the state file, as agent will synchronize the container status
	// on restart and for some operation eg: pull, it has to be recalled again.
	AppliedStatus apicontainerstatus.ContainerStatus `json:"-"`
	// ApplyingError is an error that occurred trying to transition the container
	// to its desired state. It is propagated to the backend in the form
	// 'Name: ErrorString' as the 'reason' field.
	ApplyingError *apierrors.DefaultNamedError

	// SentStatusUnsafe represents the last KnownStatusUnsafe that was sent to the ECS
	// SubmitContainerStateChange API.
	// TODO SentStatusUnsafe should probably be private with appropriately written
	// setter/getter.  When this is done, we need to ensure that the UnmarshalJSON is
	// handled properly so that the state storage continues to work.
	SentStatusUnsafe apicontainerstatus.ContainerStatus `json:"SentStatus"`

	// MetadataFileUpdated is set to true when we have completed updating the
	// metadata file
	MetadataFileUpdated bool `json:"metadataFileUpdated"`

	// KnownExitCodeUnsafe specifies the exit code for the container.
	// It is exposed outside of the package so that it's marshalled/unmarshalled in
	// the JSON body while saving the state.
	// NOTE: Do not access KnownExitCodeUnsafe directly. Instead, use `GetKnownExitCode`
	// and `SetKnownExitCode`.
	KnownExitCodeUnsafe *int `json:"KnownExitCode"`

	// KnownPortBindingsUnsafe is an array of port bindings for the container.
	KnownPortBindingsUnsafe []PortBinding `json:"KnownPortBindings"`

	// VolumesUnsafe is an array of volume mounts in the container.
	VolumesUnsafe []types.MountPoint `json:"-"`

	// NetworkModeUnsafe is the network mode in which the container is started
	NetworkModeUnsafe string `json:"-"`

	// NetworksUnsafe denotes the Docker Network Settings in the container.
	NetworkSettingsUnsafe *types.NetworkSettings `json:"-"`

	// SteadyStateStatusUnsafe specifies the steady state status for the container
	// If uninitialized, it's assumed to be set to 'ContainerRunning'. Even though
	// it's not only supposed to be set when the container is being created, it's
	// exposed outside of the package so that it's marshalled/unmarshalled in the
	// the JSON body while saving the state
	SteadyStateStatusUnsafe *apicontainerstatus.ContainerStatus `json:"SteadyStateStatus,omitempty"`

	// ContainerArn is the Arn of this container.
	ContainerArn string `json:"ContainerArn,omitempty"`

	// ContainerTornDownUnsafe is set to true when we have cleaned up this container. For now this is only used for the
	// pause container
	ContainerTornDownUnsafe bool `json:"containerTornDown"`

	createdAt  time.Time
	startedAt  time.Time
	finishedAt time.Time

	labels map[string]string

	// hasPortRange is set to true when the container has at least 1 port range requested.
	ContainerHasPortRange bool
	// ContainerPortSet is a set of singular container ports that don't belong to a containerPortRange request
	ContainerPortSet map[int]struct{}
	// ContainerPortRangeMap is a map of containerPortRange to its associated hostPortRange
	ContainerPortRangeMap map[string]string
}

type DependsOn struct {
	ContainerName string `json:"containerName"`
	Condition     string `json:"condition"`
}

// DockerContainer is a mapping between containers-as-docker-knows-them and
// containers-as-we-know-them.
// This is primarily used in DockerState, but lives here such that tasks and
// containers know how to convert themselves into Docker's desired config format
type DockerContainer struct {
	DockerID   string `json:"DockerId"`
	DockerName string // needed for linking

	Container *Container
}

type EnvironmentFile struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

// MountPoint describes the in-container location of a Volume and references
// that Volume by name.
type MountPoint struct {
	SourceVolume  string `json:"sourceVolume"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

// FirelensConfig describes the type and options of a Firelens container.
type FirelensConfig struct {
	Type    string            `json:"type"`
	Options map[string]string `json:"options"`
}

// VolumeFrom is a volume which references another container as its source.
type VolumeFrom struct {
	SourceContainer string `json:"sourceContainer"`
	ReadOnly        bool   `json:"readOnly"`
}

// Secret contains all essential attributes needed for ECS secrets vending as environment variables/tmpfs files
type Secret struct {
	Name          string `json:"name"`
	ValueFrom     string `json:"valueFrom"`
	Region        string `json:"region"`
	ContainerPath string `json:"containerPath"`
	Type          string `json:"type"`
	Provider      string `json:"provider"`
	Target        string `json:"target"`
}

// GetSecretResourceCacheKey returns the key required to access the secret
// from the ssmsecret resource
func (s *Secret) GetSecretResourceCacheKey() string {
	return s.ValueFrom + "_" + s.Region
}

// String returns a human readable string representation of DockerContainer
func (dc *DockerContainer) String() string {
	if dc == nil {
		return "nil"
	}
	return fmt.Sprintf("Id: %s, Name: %s, Container: %s", dc.DockerID, dc.DockerName, dc.Container.String())
}

// NewContainerWithSteadyState creates a new Container object with the specified
// steady state. Containers that need the non default steady state set will
// use this method instead of setting it directly
func NewContainerWithSteadyState(steadyState apicontainerstatus.ContainerStatus) *Container {
	steadyStateStatus := steadyState
	return &Container{
		SteadyStateStatusUnsafe: &steadyStateStatus,
	}
}

// KnownTerminal returns true if the container's known status is STOPPED
func (c *Container) KnownTerminal() bool {
	return c.GetKnownStatus().Terminal()
}

// DesiredTerminal returns true if the container's desired status is STOPPED
func (c *Container) DesiredTerminal() bool {
	return c.GetDesiredStatus().Terminal()
}

// GetKnownStatus returns the known status of the container
func (c *Container) GetKnownStatus() apicontainerstatus.ContainerStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.KnownStatusUnsafe
}

// SetKnownStatus sets the known status of the container and update the container
// applied status
func (c *Container) SetKnownStatus(status apicontainerstatus.ContainerStatus) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.KnownStatusUnsafe = status
	c.updateAppliedStatusUnsafe(status)
}

// GetDesiredStatus gets the desired status of the container
func (c *Container) GetDesiredStatus() apicontainerstatus.ContainerStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.DesiredStatusUnsafe
}

// SetDesiredStatus sets the desired status of the container
func (c *Container) SetDesiredStatus(status apicontainerstatus.ContainerStatus) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.DesiredStatusUnsafe = status
}

// GetSentStatus safely returns the SentStatusUnsafe of the container
func (c *Container) GetSentStatus() apicontainerstatus.ContainerStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.SentStatusUnsafe
}

// SetSentStatus safely sets the SentStatusUnsafe of the container
func (c *Container) SetSentStatus(status apicontainerstatus.ContainerStatus) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.SentStatusUnsafe = status
}

// SetKnownExitCode sets exit code field in container struct
func (c *Container) SetKnownExitCode(i *int) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.KnownExitCodeUnsafe = i
}

// GetKnownExitCode returns the container exit code
func (c *Container) GetKnownExitCode() *int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.KnownExitCodeUnsafe
}

// SetRegistryAuthCredentials sets the credentials for pulling image from ECR
func (c *Container) SetRegistryAuthCredentials(credential credentials.IAMRoleCredentials) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.RegistryAuthentication.ECRAuthData.SetPullCredentials(credential)
}

// ShouldPullWithExecutionRole returns whether this container has its own ECR credentials
func (c *Container) ShouldPullWithExecutionRole() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.RegistryAuthentication != nil &&
		c.RegistryAuthentication.Type == AuthTypeECR &&
		c.RegistryAuthentication.ECRAuthData != nil &&
		c.RegistryAuthentication.ECRAuthData.UseExecutionRole
}

// String returns a human readable string representation of this object
func (c *Container) String() string {
	ret := fmt.Sprintf("%s(%s) (%s->%s)", c.Name, c.Image,
		c.GetKnownStatus().String(), c.GetDesiredStatus().String())
	if c.GetKnownExitCode() != nil {
		ret += " - Exit: " + strconv.Itoa(*c.GetKnownExitCode())
	}
	return ret
}

// GetSteadyStateStatus returns the steady state status for the container. If
// Container.steadyState is not initialized, the default steady state status
// defined by `defaultContainerSteadyStateStatus` is returned. In awsvpc, the 'pause'
// container's steady state differs from that of other containers, as the
// 'pause' container can reach its teady state once networking resources
// have been provisioned for it, which is done in the `ContainerResourcesProvisioned`
// state. In bridge mode, pause containers are currently used exclusively for
// supporting service-connect tasks. Those pause containers will have steady state
// status "ContainerRunning" as the actual network provisioning is done by ServiceConnect
// container (aka Appnet agent)
func (c *Container) GetSteadyStateStatus() apicontainerstatus.ContainerStatus {
	if c.SteadyStateStatusUnsafe == nil {
		return defaultContainerSteadyStateStatus
	}
	return *c.SteadyStateStatusUnsafe
}

// SetSteadyStateStatusUnsafe allows setting container steady state status after they
// are initially created.
// In bridge mode, this is used by overriding the ServiceConnect container steady
// status to ContainerResourcesProvisioned because it comes with ACS task payload and will
// get ContainerRunning by default during unmarshalling. We need ServiceConnect container
// to provision network resources to support SC bridge mode
func (c *Container) SetSteadyStateStatusUnsafe(steadyState apicontainerstatus.ContainerStatus) {
	c.SteadyStateStatusUnsafe = &steadyState
}

// IsKnownSteadyState returns true if the `KnownState` of the container equals
// the `steadyState` defined for the container
func (c *Container) IsKnownSteadyState() bool {
	knownStatus := c.GetKnownStatus()
	return knownStatus == c.GetSteadyStateStatus()
}

// GetNextKnownStateProgression returns the state that the container should
// progress to based on its `KnownState`. The progression is
// incremental until the container reaches its steady state. From then on,
// it transitions to `ContainerStopped`.
//
// For example:
// a. if the steady state of the container is defined as `ContainerRunning`,
// the progression is:
// Container: None -> Pulled -> Created -> Running* -> Stopped -> Zombie
//
// b. if the steady state of the container is defined as `ContainerResourcesProvisioned`,
// the progression is:
// Container: None -> Pulled -> Created -> Running -> Provisioned* -> Stopped -> Zombie
//
// c. if the steady state of the container is defined as `ContainerCreated`,
// the progression is:
// Container: None -> Pulled -> Created* -> Stopped -> Zombie
func (c *Container) GetNextKnownStateProgression() apicontainerstatus.ContainerStatus {
	if c.IsKnownSteadyState() {
		return apicontainerstatus.ContainerStopped
	}

	return c.GetKnownStatus() + 1
}

// IsInternal returns true if the container type is `ContainerCNIPause`
// or `ContainerNamespacePause`. It returns false otherwise
func (c *Container) IsInternal() bool {
	return c.Type != ContainerNormal
}

// IsRunning returns true if the container's known status is either RUNNING
// or RESOURCES_PROVISIONED. It returns false otherwise
func (c *Container) IsRunning() bool {
	return c.GetKnownStatus().IsRunning()
}

// IsMetadataFileUpdated returns true if the metadata file has been once the
// metadata file is ready and will no longer change
func (c *Container) IsMetadataFileUpdated() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.MetadataFileUpdated
}

// SetMetadataFileUpdated sets the container's MetadataFileUpdated status to true
func (c *Container) SetMetadataFileUpdated() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.MetadataFileUpdated = true
}

// IsEssential returns whether the container is an essential container or not
func (c *Container) IsEssential() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.Essential
}

// AWSLogAuthExecutionRole returns true if the auth is by execution role
func (c *Container) AWSLogAuthExecutionRole() bool {
	return c.LogsAuthStrategy == awslogsAuthExecutionRole
}

// SetCreatedAt sets the timestamp for container's creation time
func (c *Container) SetCreatedAt(createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	c.createdAt = createdAt
}

// SetStartedAt sets the timestamp for container's start time
func (c *Container) SetStartedAt(startedAt time.Time) {
	if startedAt.IsZero() {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.startedAt = startedAt
}

// SetFinishedAt sets the timestamp for container's stopped time
func (c *Container) SetFinishedAt(finishedAt time.Time) {
	if finishedAt.IsZero() {
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	c.finishedAt = finishedAt
}

// GetCreatedAt sets the timestamp for container's creation time
func (c *Container) GetCreatedAt() time.Time {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.createdAt
}

// GetStartedAt sets the timestamp for container's start time
func (c *Container) GetStartedAt() time.Time {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.startedAt
}

// GetFinishedAt sets the timestamp for container's stopped time
func (c *Container) GetFinishedAt() time.Time {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.finishedAt
}

// SetLabels sets the labels for a container
func (c *Container) SetLabels(labels map[string]string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.labels = labels
}

// SetRuntimeID sets the DockerID for a container
func (c *Container) SetRuntimeID(RuntimeID string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.RuntimeID = RuntimeID
}

// GetRuntimeID gets the DockerID for a container
func (c *Container) GetRuntimeID() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.RuntimeID
}

// SetImageDigest sets the ImageDigest for a container
func (c *Container) SetImageDigest(ImageDigest string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.ImageDigest = ImageDigest
}

// GetImageDigest gets the ImageDigest for a container
func (c *Container) GetImageDigest() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.ImageDigest
}

// GetLabels gets the labels for a container
func (c *Container) GetLabels() map[string]string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.labels
}

// SetKnownPortBindings sets the ports for a container
func (c *Container) SetKnownPortBindings(ports []PortBinding) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.KnownPortBindingsUnsafe = ports
}

// GetKnownPortBindings gets the ports for a container
func (c *Container) GetKnownPortBindings() []PortBinding {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.KnownPortBindingsUnsafe
}

// GetManagedAgents returns the managed agents configured for this container
func (c *Container) GetManagedAgents() []ManagedAgent {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.ManagedAgentsUnsafe
}

// SetVolumes sets the volumes mounted in a container
func (c *Container) SetVolumes(volumes []types.MountPoint) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.VolumesUnsafe = volumes
}

// GetVolumes returns the volumes mounted in a container
func (c *Container) GetVolumes() []types.MountPoint {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.VolumesUnsafe
}

// SetNetworkSettings sets the networks field in a container
func (c *Container) SetNetworkSettings(networks *types.NetworkSettings) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.NetworkSettingsUnsafe = networks
}

// GetNetworkSettings returns the networks field in a container
func (c *Container) GetNetworkSettings() *types.NetworkSettings {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.NetworkSettingsUnsafe
}

// SetNetworkMode sets the network mode of the container
func (c *Container) SetNetworkMode(networkMode string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.NetworkModeUnsafe = networkMode
}

// GetNetworkMode returns the network mode of the container
func (c *Container) GetNetworkMode() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.NetworkModeUnsafe
}

// HealthStatusShouldBeReported returns true if the health check is defined in
// the task definition
func (c *Container) HealthStatusShouldBeReported() bool {
	return c.HealthCheckType == DockerHealthCheckType
}

// SetHealthStatus sets the container health status
func (c *Container) SetHealthStatus(health HealthStatus) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.Health.Status == health.Status {
		return
	}

	c.Health.Status = health.Status
	c.Health.Since = aws.Time(time.Now())
	c.Health.Output = health.Output

	// Set the health exit code if the health check failed
	if c.Health.Status == apicontainerstatus.ContainerUnhealthy {
		c.Health.ExitCode = health.ExitCode
	}
}

// GetHealthStatus returns the container health information
func (c *Container) GetHealthStatus() HealthStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// Copy the pointer to avoid race condition
	copyHealth := c.Health

	if c.Health.Since != nil {
		copyHealth.Since = aws.Time(aws.TimeValue(c.Health.Since))
	}

	return copyHealth
}

// BuildContainerDependency adds a new dependency container and satisfied status
// to the dependent container
func (c *Container) BuildContainerDependency(contName string,
	satisfiedStatus apicontainerstatus.ContainerStatus,
	dependentStatus apicontainerstatus.ContainerStatus) {
	contDep := ContainerDependency{
		ContainerName:   contName,
		SatisfiedStatus: satisfiedStatus,
	}
	if _, ok := c.TransitionDependenciesMap[dependentStatus]; !ok {
		c.TransitionDependenciesMap[dependentStatus] = TransitionDependencySet{}
	}
	deps := c.TransitionDependenciesMap[dependentStatus]
	deps.ContainerDependencies = append(deps.ContainerDependencies, contDep)
	c.TransitionDependenciesMap[dependentStatus] = deps
}

// BuildResourceDependency adds a new resource dependency by taking in the required status
// of the resource that satisfies the dependency and the dependent container status,
// whose transition is dependent on the resource.
// example: if container's PULLED transition is dependent on volume resource's
// CREATED status, then RequiredStatus=VolumeCreated and dependentStatus=ContainerPulled
func (c *Container) BuildResourceDependency(resourceName string,
	requiredStatus resourcestatus.ResourceStatus,
	dependentStatus apicontainerstatus.ContainerStatus) {

	resourceDep := ResourceDependency{
		Name:           resourceName,
		RequiredStatus: requiredStatus,
	}
	if _, ok := c.TransitionDependenciesMap[dependentStatus]; !ok {
		c.TransitionDependenciesMap[dependentStatus] = TransitionDependencySet{}
	}
	deps := c.TransitionDependenciesMap[dependentStatus]
	deps.ResourceDependencies = append(deps.ResourceDependencies, resourceDep)
	c.TransitionDependenciesMap[dependentStatus] = deps
}

// updateAppliedStatusUnsafe updates the container transitioning status
func (c *Container) updateAppliedStatusUnsafe(knownStatus apicontainerstatus.ContainerStatus) {
	if c.AppliedStatus == apicontainerstatus.ContainerStatusNone {
		return
	}

	// Check if the container transition has already finished
	if c.AppliedStatus <= knownStatus {
		c.AppliedStatus = apicontainerstatus.ContainerStatusNone
	}
}

// SetAppliedStatus sets the applied status of container and returns whether
// the container is already in a transition
func (c *Container) SetAppliedStatus(status apicontainerstatus.ContainerStatus) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.AppliedStatus != apicontainerstatus.ContainerStatusNone {
		// return false to indicate the set operation failed
		return false
	}

	c.AppliedStatus = status
	return true
}

// GetAppliedStatus returns the transitioning status of container
func (c *Container) GetAppliedStatus() apicontainerstatus.ContainerStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.AppliedStatus
}

// ShouldPullWithASMAuth returns true if this container needs to retrieve
// private registry authentication data from ASM
func (c *Container) ShouldPullWithASMAuth() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.RegistryAuthentication != nil &&
		c.RegistryAuthentication.Type == AuthTypeASM &&
		c.RegistryAuthentication.ASMAuthData != nil
}

// SetASMDockerAuthConfig add the docker auth config data to the
// RegistryAuthentication struct held by the container, this is then passed down
// to the docker client to pull the image
func (c *Container) SetASMDockerAuthConfig(dac types.AuthConfig) {
	c.RegistryAuthentication.ASMAuthData.SetDockerAuthConfig(dac)
}

// SetV3EndpointID sets the v3 endpoint id of container
func (c *Container) SetV3EndpointID(v3EndpointID string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.V3EndpointID = v3EndpointID
}

// GetV3EndpointID returns the v3 endpoint id of container
func (c *Container) GetV3EndpointID() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.V3EndpointID
}

// InjectV3MetadataEndpoint injects the v3 metadata endpoint as an environment variable for a container
func (c *Container) InjectV3MetadataEndpoint() {
	c.lock.Lock()
	defer c.lock.Unlock()

	// don't assume that the environment variable map has been initialized by others
	if c.Environment == nil {
		c.Environment = make(map[string]string)
	}

	c.Environment[MetadataURIEnvironmentVariableName] =
		fmt.Sprintf(MetadataURIFormat, c.V3EndpointID)
}

// InjectV4MetadataEndpoint injects the v4 metadata endpoint as an environment variable for a container
func (c *Container) InjectV4MetadataEndpoint() {
	c.lock.Lock()
	defer c.lock.Unlock()

	// don't assume that the environment variable map has been initialized by others
	if c.Environment == nil {
		c.Environment = make(map[string]string)
	}

	c.Environment[MetadataURIEnvVarNameV4] =
		fmt.Sprintf(MetadataURIFormatV4, c.V3EndpointID)
}

// InjectV1AgentAPIEndpoint injects the v1 Agent API endpoint into the container
// as an environment variable.
func (c *Container) InjectV1AgentAPIEndpoint() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ensureEnvironmentIsInitialized()
	c.Environment[AgentURIEnvVarName] = fmt.Sprintf(AgentURIFormat, c.V3EndpointID)
}

// Initializes Environment Map if it is nil
func (c *Container) ensureEnvironmentIsInitialized() {
	if c.Environment == nil {
		c.Environment = make(map[string]string)
	}
}

// ShouldCreateWithSSMSecret returns true if this container needs to get secret
// value from SSM Parameter Store
func (c *Container) ShouldCreateWithSSMSecret() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// Secrets field will be nil if there is no secrets for container
	if c.Secrets == nil {
		return false
	}

	for _, secret := range c.Secrets {
		if secret.Provider == SecretProviderSSM {
			return true
		}
	}
	return false
}

// ShouldCreateWithASMSecret returns true if this container needs to get secret
// value from AWS Secrets Manager
func (c *Container) ShouldCreateWithASMSecret() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// Secrets field will be nil if there is no secrets for container
	if c.Secrets == nil {
		return false
	}

	for _, secret := range c.Secrets {
		if secret.Provider == SecretProviderASM {
			return true
		}
	}
	return false
}

// ShouldCreateWithEnvFiles returns true if this container needs to
// retrieve environment variable files
func (c *Container) ShouldCreateWithEnvFiles() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.EnvironmentFiles == nil {
		return false
	}

	return len(c.EnvironmentFiles) != 0
}

// MergeEnvironmentVariables appends additional envVarName:envVarValue pairs to
// the the container's environment values structure
func (c *Container) MergeEnvironmentVariables(envVars map[string]string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// don't assume that the environment variable map has been initialized by others
	if c.Environment == nil {
		c.Environment = make(map[string]string)
	}
	for k, v := range envVars {
		c.Environment[k] = v
	}
}

// MergeEnvironmentVariablesFromEnvfiles appends environment variable pairs from
// the retrieved envfiles to the container's environment values list
// envvars from envfiles will have lower precedence than existing envvars
func (c *Container) MergeEnvironmentVariablesFromEnvfiles(envVarsList []map[string]string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	// create map if does not exist
	if c.Environment == nil {
		c.Environment = make(map[string]string)
	}

	// envVarsList is a list of map, where each map is from an envfile
	// iterate over this sequentially because the original order of the
	// environment files give precedence to the environment variables
	for _, envVars := range envVarsList {
		for k, v := range envVars {
			// existing environment variables have precedence over variables from envfile
			// only set the env var if key does not already exist
			if _, ok := c.Environment[k]; !ok {
				c.Environment[k] = v
			}
		}
	}
	return nil
}

// HasSecret returns whether a container has secret based on a certain condition.
func (c *Container) HasSecret(f func(s Secret) bool) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.Secrets == nil {
		return false
	}

	for _, secret := range c.Secrets {
		if f(secret) {
			return true
		}
	}

	return false
}

func (c *Container) GetStartTimeout() time.Duration {
	c.lock.Lock()
	defer c.lock.Unlock()

	return time.Duration(c.StartTimeout) * time.Second
}

func (c *Container) GetStopTimeout() time.Duration {
	c.lock.Lock()
	defer c.lock.Unlock()

	return time.Duration(c.StopTimeout) * time.Second
}

func (c *Container) GetDependsOn() []DependsOn {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.DependsOnUnsafe
}

func (c *Container) SetDependsOn(dependsOn []DependsOn) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.DependsOnUnsafe = dependsOn
}

// DependsOnContainer checks whether a container depends on another container.
func (c *Container) DependsOnContainer(name string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, dependsOn := range c.DependsOnUnsafe {
		if dependsOn.ContainerName == name {
			return true
		}
	}

	return false
}

// HasContainerDependencies checks whether a container has any container dependency.
func (c *Container) HasContainerDependencies() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return len(c.DependsOnUnsafe) != 0
}

// AddContainerDependency adds a container dependency to a container.
func (c *Container) AddContainerDependency(name string, condition string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.DependsOnUnsafe = append(c.DependsOnUnsafe, DependsOn{
		ContainerName: name,
		Condition:     condition,
	})
}

// GetLogDriver returns the log driver used by the container.
func (c *Container) GetLogDriver() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return ""
	}

	hostConfig := &dockercontainer.HostConfig{}
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil {
		seelog.Warnf("Encountered error when trying to get log driver for container %s: %v", c.RuntimeID, err)
		return ""
	}

	return hostConfig.LogConfig.Type
}

// GetLogOptions gets the log 'options' map passed into the task definition.
// see https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_LogConfiguration.html
func (c *Container) GetLogOptions() map[string]string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return map[string]string{}
	}

	hostConfig := &dockercontainer.HostConfig{}
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil {
		seelog.Warnf("Encountered error when trying to get log configuration for container %s: %v", c.RuntimeID, err)
		return map[string]string{}
	}

	return hostConfig.LogConfig.Config
}

// GetNetworkModeFromHostConfig returns the network mode used by the container from the host config .
func (c *Container) GetNetworkModeFromHostConfig() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return ""
	}

	hostConfig := &dockercontainer.HostConfig{}
	// TODO return error to differentiate between error and default mode .
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil {
		seelog.Warnf("Encountered error when trying to get network mode for container %s: %v", c.RuntimeID, err)
		return ""
	}

	return hostConfig.NetworkMode.NetworkName()
}

// GetHostConfig returns the container's host config.
func (c *Container) GetHostConfig() *string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.DockerConfig.HostConfig
}

// GetFirelensConfig returns the container's firelens config.
func (c *Container) GetFirelensConfig() *FirelensConfig {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.FirelensConfig
}

// GetEnvironmentFiles returns the container's environment files.
func (c *Container) GetEnvironmentFiles() []EnvironmentFile {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.EnvironmentFiles
}

// RequireNeuronRuntime checks if the container needs to use the neuron runtime.
func (c *Container) RequireNeuronRuntime() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	_, ok := c.Environment[neuronVisibleDevicesEnvVar]
	return ok
}

// SetTaskARN sets the task arn of the container.
func (c *Container) SetTaskARN(arn string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.TaskARNUnsafe = arn
}

// GetTaskARN returns the task arn of the container.
func (c *Container) GetTaskARN() string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.TaskARNUnsafe
}

// HasNotAndWillNotStart returns true if the container has never started, and is not going to start in the future.
// This is true if the following are all true:
// 1. Container's known status is earlier than running;
// 2. Container's desired status is stopped;
// 3. Container is not in the middle a transition (indicated by applied status is none status).
func (c *Container) HasNotAndWillNotStart() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.KnownStatusUnsafe < apicontainerstatus.ContainerRunning &&
		c.DesiredStatusUnsafe.Terminal() &&
		c.AppliedStatus == apicontainerstatus.ContainerStatusNone
}

// GetManagedAgentByName retrieves the managed agent with the name specified and a boolean indicating whether an agent
// was found or not.
// note: a zero value for ManagedAgent if the name is not known to this container.
func (c *Container) GetManagedAgentByName(agentName string) (ManagedAgent, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	for _, ma := range c.ManagedAgentsUnsafe {
		if ma.Name == agentName {
			return ma, true
		}
	}
	return ManagedAgent{}, false
}

// UpdateManagedAgentByName updates the state of the managed agent with the name specified. If the agent is not found,
// this method returns false.
func (c *Container) UpdateManagedAgentByName(agentName string, state ManagedAgentState) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i, ma := range c.ManagedAgentsUnsafe {
		if ma.Name == agentName {
			// It's necessary to clone the whole ManagedAgent struct
			c.ManagedAgentsUnsafe[i] = ManagedAgent{
				Name:              ma.Name,
				Properties:        ma.Properties,
				ManagedAgentState: state,
			}
			return true
		}
	}
	return false
}

// UpdateManagedAgentStatus updates the status of the managed agent with the name specified. If the agent is not found,
// this method returns false.
func (c *Container) UpdateManagedAgentStatus(agentName string, status apicontainerstatus.ManagedAgentStatus) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i, ma := range c.ManagedAgentsUnsafe {
		if ma.Name == agentName {
			c.ManagedAgentsUnsafe[i].Status = status
			return true
		}
	}
	return false
}

// UpdateManagedAgentSentStatus updates the sent status of the managed agent with the name specified. If the agent is not found,
// this method returns false.
func (c *Container) UpdateManagedAgentSentStatus(agentName string, status apicontainerstatus.ManagedAgentStatus) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i, ma := range c.ManagedAgentsUnsafe {
		if ma.Name == agentName {
			c.ManagedAgentsUnsafe[i].SentStatus = status
			return true
		}
	}
	return false
}

// RequiresCredentialSpec checks if container needs a credentialspec resource
func (c *Container) RequiresCredentialSpec() bool {
	credSpec, err := c.getCredentialSpec()
	if err != nil || credSpec == "" {
		return false
	}

	return true
}

// GetCredentialSpec is used to retrieve the current credentialspec resource
func (c *Container) GetCredentialSpec() (string, error) {
	return c.getCredentialSpec()
}

func (c *Container) getCredentialSpec() (string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return "", errors.New("empty container hostConfig")
	}

	hostConfig := &dockercontainer.HostConfig{}
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil || len(hostConfig.SecurityOpt) == 0 {
		return "", errors.New("unable to obtain security options from container hostConfig")
	}

	for _, opt := range hostConfig.SecurityOpt {
		if strings.HasPrefix(opt, "credentialspec") {
			return opt, nil
		}
	}

	return "", errors.New("unable to obtain credentialspec")
}

func (c *Container) GetManagedAgentStatus(agentName string) apicontainerstatus.ManagedAgentStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()
	for i, ma := range c.ManagedAgentsUnsafe {
		if ma.Name == agentName {
			return c.ManagedAgentsUnsafe[i].Status
		}
	}
	// we shouldn't get here because we'll always have a valid ManagedAgentName
	return apicontainerstatus.ManagedAgentStatusNone
}

func (c *Container) GetManagedAgentSentStatus(agentName string) apicontainerstatus.ManagedAgentStatus {
	c.lock.RLock()
	defer c.lock.RUnlock()
	for i, ma := range c.ManagedAgentsUnsafe {
		if ma.Name == agentName {
			return c.ManagedAgentsUnsafe[i].SentStatus
		}
	}
	// we shouldn't get here because we'll always have a valid ManagedAgentName
	return apicontainerstatus.ManagedAgentStatusNone
}

func (c *Container) SetContainerTornDown(td bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ContainerTornDownUnsafe = td
}

func (c *Container) IsContainerTornDown() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.ContainerTornDownUnsafe
}

func (c *Container) SetContainerHasPortRange(containerHasPortRange bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ContainerHasPortRange = containerHasPortRange
}

func (c *Container) GetContainerHasPortRange() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.ContainerHasPortRange
}

func (c *Container) SetContainerPortSet(containerPortSet map[int]struct{}) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ContainerPortSet = containerPortSet
}

func (c *Container) GetContainerPortSet() map[int]struct{} {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.ContainerPortSet
}

func (c *Container) SetContainerPortRangeMap(portRangeMap map[string]string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ContainerPortRangeMap = portRangeMap
}

func (c *Container) GetContainerPortRangeMap() map[string]string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.ContainerPortRangeMap
}
