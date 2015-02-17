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
	"fmt"
	"strconv"
	"sync"
	"time"
)

type TaskStatus int32
type ContainerStatus int32

const (
	TaskStatusNone TaskStatus = iota
	TaskStatusUnknown
	TaskPulled
	TaskCreated
	TaskRunning
	TaskStopped
	TaskDead
)

const (
	ContainerStatusNone ContainerStatus = iota
	ContainerStatusUnknown
	ContainerPulled
	ContainerCreated
	ContainerRunning
	ContainerStopped
	ContainerDead

	ContainerZombie // Impossible status to use as a virtual 'max'
)

type PortBinding struct {
	ContainerPort uint16
	HostPort      uint16
	BindIp        string
}

type TaskOverrides struct{}

type Task struct {
	Arn        string
	Overrides  TaskOverrides `json:"-"`
	Family     string
	Version    string
	Containers []*Container
	Volumes    []TaskVolume `json:"volumes"`

	DesiredStatus TaskStatus
	KnownStatus   TaskStatus
	KnownTime     time.Time

	SentStatus TaskStatus

	containersByNameLock sync.Mutex
	containersByName     map[string]*Container
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
	hostPath string `json:"-"`
}

func (e *EmptyHostVolume) SourcePath() string {
	return e.hostPath
}

type ContainerStateChange struct {
	TaskArn       string
	ContainerName string
	Status        ContainerStatus

	Reason       string
	ExitCode     *int
	PortBindings []PortBinding

	TaskStatus TaskStatus // TaskStatusNone if this does not result in a task state change

	Task      *Task
	Container *Container
}

func (t *Task) String() string {
	res := fmt.Sprintf("%s-%s %s, Overrides: %s Status: %s(%s)", t.Family, t.Version, t.Arn, t.Overrides, t.KnownStatus.String(), t.DesiredStatus.String())
	res += " Containers: "
	for _, c := range t.Containers {
		res += c.Name + ","
	}
	return res
}

type ContainerOverrides struct {
	Command *[]string `json:"command"`
}

// ApplyingError is an error indicating something that went wrong transitioning
// from one state to another. For now it's very simple (and exists in part to
// ensure that there's a symetric marshal/unmarshal for the error).
type ApplyingError struct {
	Err string `json:"error"`
}

func (ae *ApplyingError) Error() string {
	return ae.Err
}
func NewApplyingError(err error) *ApplyingError {
	return &ApplyingError{err.Error()}
}

type Container struct {
	Name        string
	Image       string
	Command     []string
	Cpu         uint
	Memory      uint
	Links       []string
	VolumesFrom []VolumeFrom  `json:"volumesFrom"`
	MountPoints []MountPoint  `json:"mountPoints"`
	Ports       []PortBinding `json:"portMappings"`
	Essential   bool
	EntryPoint  *[]string
	Environment map[string]string  `json:"environment"`
	Overrides   ContainerOverrides `json:"overrides"`

	DesiredStatus ContainerStatus `json:"desiredStatus"`
	KnownStatus   ContainerStatus

	// RunDependencies is a list of containers that must be run before
	// this one is created
	RunDependencies []string
	// 'Internal' containers are ones that are not directly specified by task definitions, but created by the agent
	IsInternal bool

	AppliedStatus ContainerStatus
	ApplyingError *ApplyingError

	SentStatus ContainerStatus

	KnownExitCode     *int
	KnownPortBindings []PortBinding

	// Not upstream; todo move this out into a wrapper type
	StatusLock sync.Mutex
}

// VolumeFrom is a volume which references another container as its source.
type VolumeFrom struct {
	SourceContainer string `json:"sourceContainer"`
	ReadOnly        bool   `json:"readOnly"`
}

func (c *Container) String() string {
	ret := fmt.Sprintf("%s(%s) - Status: %v", c.Name, c.Image, c.KnownStatus.String())
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
