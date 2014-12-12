package api

import (
	"fmt"
	"sync"
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
)

type PortBinding struct {
	ContainerPort uint16
	HostPort      uint16
	BindIp        string
}

type TaskOverrides struct{}

type Task struct {
	Arn        string
	Overrides  TaskOverrides `json:"ignore"` // No taskOverrides right now
	Family     string
	Version    string
	Containers []*Container

	DesiredStatus TaskStatus
	KnownStatus   TaskStatus

	containersByNameLock sync.Mutex
	containersByName     map[string]*Container
}

type ContainerStateChange struct {
	TaskArn       string
	ContainerName string
	Status        ContainerStatus

	Reason       string
	ExitCode     *int
	PortBindings []PortBinding

	TaskStatus TaskStatus // TaskStatusNone if this does not result in a task state change
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

type Container struct {
	Name        string
	Image       string
	Command     []string
	Cpu         uint
	Memory      uint
	Links       []string
	BindMounts  []string
	VolumesFrom []string
	Ports       []PortBinding `json:"portMappings"`
	Essential   bool
	EntryPoint  *[]string
	Environment map[string]string  `json:"environment"`
	Overrides   ContainerOverrides `json:"overrides"`

	DesiredStatus ContainerStatus `json:"desiredStatus"`
	AppliedStatus ContainerStatus
	KnownStatus   ContainerStatus

	KnownExitCode     *int
	KnownPortBindings []PortBinding

	// Not upstream; todo move this out into a wrapper type
	StatusLock sync.Mutex
}

func (c *Container) String() string {
	return fmt.Sprintf("%s - %s '%s' '%s', %d cpu, %d memory, Links: %s, Mounts: %s, Ports: %s, Essential: %s, Environment: %s, Overrides: %s, Status: %s(%s)", c.Name, c.Image, c.EntryPoint, c.Command, c.Cpu, c.Memory, c.Links, c.BindMounts, c.Ports, c.Essential, c.Environment, c.Overrides, c.KnownStatus.String(), c.DesiredStatus.String())
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
