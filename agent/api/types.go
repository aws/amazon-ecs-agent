// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// TaskStatus is an enumeration of valid states in the task lifecycle
type TaskStatus int32

const (
	// TaskStatusNone is the zero state of a task; this task has been received but no further progress has completed
	TaskStatusNone TaskStatus = iota
	// TaskPulled represents a task which has had all its container images pulled, but not all have yet progressed passed pull
	TaskPulled
	// TaskCreated represents a task which has had all its containers created
	TaskCreated
	// TaskRunning represents a task which has had all its containers started
	TaskRunning
	// TaskStopped represents a task in which all containers are stopped
	TaskStopped
)

// ContainerStatus is an enumeration of valid states in the container lifecycle
type ContainerStatus int32

const (
	// ContainerStatusNone is the zero state of a container; this container has not completed pull
	ContainerStatusNone ContainerStatus = iota
	// ContainerPulled represents a container which has had the image pulled
	ContainerPulled
	// ContainerCreated represents a container that has been created
	ContainerCreated
	// ContainerRunning represents a container that has started
	ContainerRunning
	// ContainerStopped represents a container that has stopped
	ContainerStopped

	// ContainerZombie is an "impossible" state that is used as the maximum
	ContainerZombie
)

// TransportProtocol is an enumeration of valid transport protocols
type TransportProtocol int32

const (
	// TransportProtocolTCP represents TCP
	TransportProtocolTCP TransportProtocol = iota
	// TransportProtocolUDP represents UDP
	TransportProtocolUDP
)

const (
	tcp = "tcp"
	udp = "udp"
)

// NewTransportProtocol returns a TransportProtocol from a string in the task
func NewTransportProtocol(protocol string) (TransportProtocol, error) {
	switch protocol {
	case tcp:
		return TransportProtocolTCP, nil
	case udp:
		return TransportProtocolUDP, nil
	default:
		return TransportProtocolTCP, errors.New(protocol + " is not a recognized transport protocol")
	}
}

func (tp *TransportProtocol) String() string {
	if tp == nil {
		return tcp
	}
	switch *tp {
	case TransportProtocolUDP:
		return udp
	case TransportProtocolTCP:
		return tcp
	default:
		log.Crit("Unknown TransportProtocol type!")
		return tcp
	}
}

// PortBinding represents a port binding for a container
type PortBinding struct {
	// ContainerPort is the port inside the container
	ContainerPort uint16
	// HostPort is the port exposed on the host
	HostPort uint16
	// BindIP is the IP address to which the port is bound
	BindIP string `json:"BindIp"`
	// Protocol is the protocol of the port
	Protocol TransportProtocol
}

// TaskOverrides are the overrides applied to a task
type TaskOverrides struct{}

// Task is the internal representation of a task in the ECS agent
type Task struct {
	// Arn is the unique identifer for the task
	Arn string
	// Overrides are the overrides applied to a task
	Overrides TaskOverrides `json:"-"`
	// Family is the name of the task definition family
	Family string
	// Version is the version of the task definition
	Version string
	// Containers are the containers for the task
	Containers []*Container
	// Volumes are the volumes for the task
	Volumes []TaskVolume `json:"volumes"`

	// DesiredStatus represents the state where the task should go.  Generally the desired status is informed by the ECS
	// backend as a result of either API calls made to ECS or decisions made by the ECS service scheduler.  The
	// DesiredStatus is almost always either TaskRunning or TaskStopped.  Do not access DesiredStatus directly.  Instead,
	// use `UpdateStatus`, `UpdateDesiredStatus`, `SetDesiredStatus`, and `SetDesiredStatus`.
	// TODO DesiredStatus should probably be private with appropriately written setter/getter.  When this is done, we need
	// to ensure that the UnmarshalJSON is handled properly so that the state storage continues to work.
	DesiredStatus     TaskStatus
	desiredStatusLock sync.RWMutex

	// KnownStatus represents the state where the task is.  This is generally the minimum of equivalent status types for
	// the containers in the task; if one container is at ContainerRunning and another is at ContainerPulled, the task
	// KnownStatus would be TaskPulled.  Do not access KnownStatus directly.  Instead, use `UpdateStatus`,
	// `UpdateKnownStatusAndTime`,  and `GetKnownStatus`.
	// TODO KnownStatus should probably be private with appropriately written setter/getter.  When this is done, we need
	// to ensure that the UnmarshalJSON is handled properly so that the state storage continues to work.
	KnownStatus     TaskStatus
	knownStatusLock sync.RWMutex
	// KnownStatusTime captures the time when the KnownStatus was last updated.  Do not access KnownStatusTime directly,
	// instead use `GetKnownStatusTime`.
	KnownStatusTime     time.Time `json:"KnownTime"`
	knownStatusTimeLock sync.RWMutex

	// SentStatus represents the last KnownStatus that was sent to the ECS SubmitTaskStateChange API.
	// TODO(samuelkarp) SentStatus needs a lock and setters/getters.
	// TODO SentStatus should probably be private with appropriately written setter/getter.  When this is done, we need
	// to ensure that the UnmarshalJSON is handled properly so that the state storage continues to work.
	SentStatus     TaskStatus
	sentStatusLock sync.RWMutex

	StartSequenceNumber int64
	StopSequenceNumber  int64

	// credentialsID is used to set the CredentialsId field for the
	// IAMRoleCredentials object associated with the task. This id can be
	// used to look up the credentials for task in the credentials manager
	credentialsID     string
	credentialsIDLock sync.RWMutex
}

// TaskVolume is a definition of all the volumes available for containers to
// reference within a task. It must be named.
type TaskVolume struct {
	Name   string `json:"name"`
	Volume HostVolume
}

// MountPoint describes the in-container location of a Volume and references
// that Volume by name.
type MountPoint struct {
	SourceVolume  string `json:"sourceVolume"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

// HostVolume is an interface for something that may be used as the host half of a
// docker volume mount
type HostVolume interface {
	SourcePath() string
}

// FSHostVolume is a simple type of HostVolume which references an arbitrary
// location on the host as the Volume.
type FSHostVolume struct {
	FSSourcePath string `json:"sourcePath"`
}

// SourcePath returns the path on the host filesystem that should be mounted
func (fs *FSHostVolume) SourcePath() string {
	return fs.FSSourcePath
}

// EmptyHostVolume represents a volume without a specified host path
type EmptyHostVolume struct {
	HostPath string `json:"hostPath"`
}

// SourcePath returns the generated host path for the volume
func (e *EmptyHostVolume) SourcePath() string {
	return e.HostPath
}

// ContainerStateChange represents a state change that needs to be sent to the
// SubmitContainerStateChange API
type ContainerStateChange struct {
	// TaskArn is the unique identifier for the task
	TaskArn string
	// ContainerName is the name of the container
	ContainerName string
	// Status is the status to send
	Status ContainerStatus

	// Reason may contain details of why the container stopped
	Reason string
	// ExitCode is the exit code of the container, if available
	ExitCode *int
	// PortBindings are the details of the host ports picked for the specified
	// container ports
	PortBindings []PortBinding

	// Container is a pointer to the container involved in the state change that gives the event handler a hook into
	// storing what status was sent.  This is used to ensure the same event is handled only once.
	Container *Container
}

func (c *ContainerStateChange) String() string {
	res := fmt.Sprintf("%s %s -> %s", c.TaskArn, c.ContainerName, c.Status.String())
	if c.ExitCode != nil {
		res += ", Exit " + strconv.Itoa(*c.ExitCode) + ", "
	}
	if c.Reason != "" {
		res += ", Reason " + c.Reason
	}
	if len(c.PortBindings) != 0 {
		res += fmt.Sprintf(", Ports %v", c.PortBindings)
	}
	if c.Container != nil {
		res += ", Known Sent: " + c.Container.GetSentStatus().String()
	}
	return res
}

// TaskStateChange represents a state change that needs to be sent to the
// SubmitTaskStateChange API
type TaskStateChange struct {
	// TaskArn is the unique identifier for the task
	TaskArn string
	// Status is the status to send
	Status TaskStatus
	// Reason may contain details of why the task stopped
	Reason string

	// Task is a pointer to the task involved in the state change that gives the event handler a hook into storing
	// what status was sent.  This is used to ensure the same event is handled only once.
	Task *Task
}

func (t *TaskStateChange) String() string {
	res := fmt.Sprintf("%s -> %s", t.TaskArn, t.Status.String())
	if t.Task != nil {
		res += ", Known Sent: " + t.Task.GetSentStatus().String()
	}
	return res
}

func (t *Task) String() string {
	res := fmt.Sprintf("%s:%s %s, Status: (%s->%s)", t.Family, t.Version, t.Arn, t.GetKnownStatus().String(), t.GetDesiredStatus().String())
	res += " Containers: ["
	for _, c := range t.Containers {
		res += fmt.Sprintf("%s (%s->%s),", c.Name, c.GetKnownStatus().String(), c.GetDesiredStatus().String())
	}
	return res + "]"
}

// ContainerOverrides are overrides applied to the container
type ContainerOverrides struct {
	Command *[]string `json:"command"`
}

// Container is the internal representation of a container in the ECS agent
type Container struct {
	// Name is the name of the container specified in the task definition
	Name string
	// Image is the image name specified in the task definition
	Image string
	// ImageID is the local ID of the image used in the container
	ImageID string

	Command                []string
	CPU                    uint `json:"Cpu"`
	Memory                 uint
	Links                  []string
	VolumesFrom            []VolumeFrom  `json:"volumesFrom"`
	MountPoints            []MountPoint  `json:"mountPoints"`
	Ports                  []PortBinding `json:"portMappings"`
	Essential              bool
	EntryPoint             *[]string
	Environment            map[string]string           `json:"environment"`
	Overrides              ContainerOverrides          `json:"overrides"`
	DockerConfig           DockerConfig                `json:"dockerConfig"`
	RegistryAuthentication *RegistryAuthenticationData `json:"registryAuthentication"`

	// DesiredStatus represents the state where the container should go.  Generally the desired status is informed by the
	// ECS backend as a result of either API calls made to ECS or decisions made by the ECS service scheduler, though the
	// agent may also set the DesiredStatus if a different "essential" container in the task exits.  The DesiredStatus is
	// almost always either ContainerRunning or ContainerStopped.  Do not access DesiredStatus directly.  Instead,
	// use `GetDesiredStatus` and `SetDesiredStatus`.
	// TODO DesiredStatus should probably be private with appropriately written setter/getter.  When this is done, we need
	// to ensure that the UnmarshalJSON is handled properly so that the state storage continues to work.
	DesiredStatus     ContainerStatus `json:"desiredStatus"`
	desiredStatusLock sync.RWMutex

	// KnownStatus represents the state where the container is.  Do not access KnownStatus directly.  Instead, use
	// `GetKnownStatus` and `SetKnownStatus`.
	// TODO KnownStatus should probably be private with appropriately written setter/getter.  When this is done, we need
	// to ensure that the UnmarshalJSON is handled properly so that the state storage continues to work.
	KnownStatus     ContainerStatus
	knownStatusLock sync.RWMutex

	// RunDependencies is a list of containers that must be run before
	// this one is created
	RunDependencies []string
	// 'Internal' containers are ones that are not directly specified by task definitions, but created by the agent
	IsInternal bool

	// AppliedStatus is the status that has been "applied" (e.g., we've called Pull, Create, Start, or Stop) but we don't
	// yet know that the application was successful.
	AppliedStatus ContainerStatus
	// ApplyingError is an error that occured trying to transition the container to its desired state
	// It is propagated to the backend in the form 'Name: ErrorString' as the 'reason' field.
	ApplyingError *DefaultNamedError

	// SentStatus represents the last KnownStatus that was sent to the ECS SubmitContainerStateChange API.
	// TODO SentStatus should probably be private with appropriately written setter/getter.  When this is done, we need
	// to ensure that the UnmarshalJSON is handled properly so that the state storage continues to work.
	SentStatus     ContainerStatus
	sentStatusLock sync.RWMutex

	KnownExitCode     *int
	KnownPortBindings []PortBinding

	// Not upstream; todo move this out into a wrapper type
	StatusLock sync.Mutex
}

// DockerConfig represents additional metadata about a container to run. It's
// remodeled from the `ecsacs` api model file. Eventually it should not exist
// once this remodeling is refactored out.
type DockerConfig struct {
	Config     *string `json:"config"`
	HostConfig *string `json:"hostConfig"`
	Version    *string `json:"version"`
}

// VolumeFrom is a volume which references another container as its source.
type VolumeFrom struct {
	SourceContainer string `json:"sourceContainer"`
	ReadOnly        bool   `json:"readOnly"`
}

// RegistryAuthenticationData is the authentication data sent by the ECS backend.  Currently, the only supported
// authentication data is for ECR.
type RegistryAuthenticationData struct {
	Type        string       `json:"type"`
	ECRAuthData *ECRAuthData `json:"ecrAuthData"`
}

// ECRAuthData is the authentication details for ECR specifying the region, registryID, and possible endpoint override
type ECRAuthData struct {
	EndpointOverride string `json:"endpointOverride"`
	Region           string `json:"region"`
	RegistryID       string `json:"registryId"`
}

func (c *Container) String() string {
	ret := fmt.Sprintf("%s(%s) (%s->%s)", c.Name, c.Image, c.GetKnownStatus().String(), c.GetDesiredStatus().String())
	if c.KnownExitCode != nil {
		ret += " - Exit: " + strconv.Itoa(*c.KnownExitCode)
	}
	return ret
}

// Resource is an on-host resource
type Resource struct {
	Name        string
	Type        string
	DoubleValue float64
	LongValue   int64
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

func (dc *DockerContainer) String() string {
	if dc == nil {
		return "nil"
	}
	return fmt.Sprintf("Id: %s, Name: %s, Container: %s", dc.DockerID, dc.DockerName, dc.Container.String())
}
