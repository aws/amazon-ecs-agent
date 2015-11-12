// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

type TaskStatus int32

const (
	TaskStatusNone TaskStatus = iota
	TaskPulled
	TaskCreated
	TaskRunning
	TaskStopped
)

type ContainerStatus int32

const (
	ContainerStatusNone ContainerStatus = iota
	ContainerPulled
	ContainerCreated
	ContainerRunning
	ContainerStopped

	ContainerZombie // Impossible status to use as a virtual 'max'
)

type TransportProtocol int32

const (
	TransportProtocolTCP TransportProtocol = iota
	TransportProtocolUDP
)

func NewTransportProtocol(protocol string) (TransportProtocol, error) {
	switch protocol {
	case "tcp":
		return TransportProtocolTCP, nil
	case "udp":
		return TransportProtocolUDP, nil
	default:
		return TransportProtocolTCP, errors.New(protocol + " is not a recognized transport protocol")
	}
}

func (tp *TransportProtocol) String() string {
	if tp == nil {
		return "tcp"
	}
	switch *tp {
	case TransportProtocolUDP:
		return "udp"
	case TransportProtocolTCP:
		return "tcp"
	default:
		log.Crit("Unknown TransportProtocol type!")
		return "tcp"
	}
}

type PortBinding struct {
	ContainerPort uint16
	HostPort      uint16
	BindIp        string
	Protocol      TransportProtocol
}

type TaskOverrides struct{}

type Task struct {
	Arn        string
	Overrides  TaskOverrides `json:"-"`
	Family     string
	Version    string
	Containers []*Container
	Volumes    []TaskVolume `json:"volumes"`

	DesiredStatus   TaskStatus
	KnownStatus     TaskStatus
	KnownStatusTime time.Time `json:"KnownTime"`

	SentStatus TaskStatus

	containersByNameLock sync.Mutex
	containersByName     map[string]*Container

	StartSequenceNumber int64
	StopSequenceNumber  int64
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

type EmptyHostVolume struct {
	HostPath string `json:"hostPath"`
}

func (e *EmptyHostVolume) SourcePath() string {
	return e.HostPath
}

type ContainerStateChange struct {
	TaskArn       string
	ContainerName string
	Status        ContainerStatus

	Reason       string
	ExitCode     *int
	PortBindings []PortBinding

	// This bit is a little hacky; a pointer to the container's sentstatus which
	// may be updated to indicate what status was sent. This is used to ensure
	// the same event is handled only once.
	SentStatus *ContainerStatus
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
	if c.SentStatus != nil {
		res += ", Known Sent: " + c.SentStatus.String()
	}
	return res
}

type TaskStateChange struct {
	TaskArn string
	Status  TaskStatus
	Reason  string

	// As above, this is the same sort of hacky.
	// This is a pointer to the task's sent-status that gives the event handler a
	// hook into storing metadata about the task on the task such that it follows
	// the lifecycle of the task and so on.
	SentStatus *TaskStatus
}

func (t *TaskStateChange) String() string {
	res := fmt.Sprintf("%s -> %s", t.TaskArn, t.Status.String())
	if t.SentStatus != nil {
		res += ", Known Sent: " + t.SentStatus.String()
	}
	return res
}

func (t *Task) String() string {
	res := fmt.Sprintf("%s:%s %s, Status: (%s->%s)", t.Family, t.Version, t.Arn, t.KnownStatus.String(), t.DesiredStatus.String())
	res += " Containers: ["
	for _, c := range t.Containers {
		res += fmt.Sprintf("%s (%s->%s),", c.Name, c.KnownStatus.String(), c.DesiredStatus.String())
	}
	return res + "]"
}

type ContainerOverrides struct {
	Command *[]string `json:"command"`
}

type Container struct {
	Name                   string
	Image                  string
	Command                []string
	Cpu                    uint
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

	DesiredStatus ContainerStatus `json:"desiredStatus"`
	KnownStatus   ContainerStatus

	// RunDependencies is a list of containers that must be run before
	// this one is created
	RunDependencies []string
	// 'Internal' containers are ones that are not directly specified by task definitions, but created by the agent
	IsInternal bool

	AppliedStatus ContainerStatus
	// ApplyingError is an error that occured trying to transition the container to its desired state
	// It is propagated to the backend in the form 'Name: ErrorString' as the 'reason' field.
	ApplyingError *DefaultNamedError

	SentStatus ContainerStatus

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

type RegistryAuthenticationData struct {
	Type        string       `json:"type"`
	ECRAuthData *ECRAuthData `json:"ecrAuthData"`
}

type ECRAuthData struct {
	EndpointOverride string `json:"endpointOverride"`
	Region           string `json:"region"`
	RegistryId       string `json:"registryId"`
}

func (c *Container) String() string {
	ret := fmt.Sprintf("%s(%s) (%s->%s)", c.Name, c.Image, c.KnownStatus.String(), c.DesiredStatus.String())
	if c.KnownExitCode != nil {
		ret += " - Exit: " + strconv.Itoa(*c.KnownExitCode)
	}
	return ret
}

type Resource struct {
	Name        string
	Type        string
	DoubleValue float64
	LongValue   int64
}

// This is a mapping between containers-as-docker-knows-them and
// containers-as-we-know-them.
// This is primarily used in DockerState, but lives here such that tasks and
// containers know how to convert themselves into Docker's desired config format
type DockerContainer struct {
	DockerId   string
	DockerName string // needed for linking

	Container *Container
}

func (dc *DockerContainer) String() string {
	if dc == nil {
		return "nil"
	}
	return fmt.Sprintf("Id: %s, Name: %s, Container: %s", dc.DockerId, dc.DockerName, dc.Container.String())
}
