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

package execcmd

import (
	"context"
	"strconv"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"

	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	hostExecDepsDir = "/var/lib/ecs/deps/execute-command"
	HostBinDir      = hostExecDepsDir + "/bin"

	ExecuteCommandAgentName    = ecs.ManagedAgentNameExecuteCommandAgent
	defaultStartRetryTimeout   = time.Minute * 10
	defaultRetryMinDelay       = time.Second * 1
	defaultRetryMaxDelay       = time.Second * 30
	defaultInspectRetryTimeout = time.Minute * 2
	maxRetries                 = 5
	retryDelayMultiplier       = 1.5
	retryJitterMultiplier      = 0.2

	Restarted RestartStatus = iota
	NotRestarted
	Unknown
)

type RestartStatus int

func (rs RestartStatus) String() string {
	switch rs {
	case Restarted:
		return "Restarted"
	case NotRestarted:
		return "NotRestarted"
	case Unknown:
		return "Unknown"
	default:
		return strconv.Itoa(int(rs))
	}
}

type StartError struct {
	error
	retryable bool
}

func (e StartError) Retry() bool {
	return e.retryable
}

type Manager interface {
	InitializeContainer(taskId string, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error
	StartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) error
	RestartAgentIfStopped(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (RestartStatus, error)
}

type manager struct {
	hostBinDir          string
	retryMaxDelay       time.Duration
	retryMinDelay       time.Duration
	startRetryTimeout   time.Duration
	inspectRetryTimeout time.Duration
}

func NewManager() *manager {
	return &manager{
		hostBinDir:          HostBinDir,
		retryMaxDelay:       defaultRetryMaxDelay,
		retryMinDelay:       defaultRetryMinDelay,
		startRetryTimeout:   defaultStartRetryTimeout,
		inspectRetryTimeout: defaultInspectRetryTimeout,
	}
}

func NewManagerWithBinDir(hostBinDir string) *manager {
	m := NewManager()
	m.hostBinDir = hostBinDir
	return m
}

func (m *manager) isAgentStarted(ma apicontainer.ManagedAgent) bool {
	return !ma.LastStartedAt.IsZero()
}

func IsExecEnabledTask(task *apitask.Task) bool {
	for _, c := range task.Containers {
		if IsExecEnabledContainer(c) {
			return true
		}
	}
	return false
}

func IsExecEnabledContainer(container *apicontainer.Container) bool {
	_, ok := container.GetManagedAgentByName(ExecuteCommandAgentName)
	return ok
}
