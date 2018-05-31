// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v2

import (
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// TaskResponse defines the schema for the task response JSON object
type TaskResponse struct {
	Cluster            string
	TaskARN            string
	Family             string
	Revision           string
	DesiredStatus      string `json:",omitempty"`
	KnownStatus        string
	Containers         []ContainerResponse `json:",omitempty"`
	Limits             *LimitsResponse     `json:",omitempty"`
	PullStartedAt      *time.Time          `json:",omitempty"`
	PullStoppedAt      *time.Time          `json:",omitempty"`
	ExecutionStoppedAt *time.Time          `json:",omitempty"`
}

// ContainerResponse defines the schema for the container response
// JSON object
type ContainerResponse struct {
	ID            string `json:"DockerId"`
	Name          string
	DockerName    string
	Image         string
	ImageID       string
	Ports         []PortResponse    `json:",omitempty"`
	Labels        map[string]string `json:",omitempty"`
	DesiredStatus string
	KnownStatus   string
	ExitCode      *int `json:",omitempty"`
	Limits        LimitsResponse
	CreatedAt     *time.Time `json:",omitempty"`
	StartedAt     *time.Time `json:",omitempty"`
	FinishedAt    *time.Time `json:",omitempty"`
	Type          string
	Networks      []containermetadata.Network `json:",omitempty"`
	Health        *apicontainer.HealthStatus  `json:",omitempty"`
}

// LimitsResponse defines the schema for task/cpu limits response
// JSON object
type LimitsResponse struct {
	CPU    *float64 `json:",omitempty"`
	Memory *int64   `json:",omitempty"`
}

// PortResponse defines the schema for portmapping response JSON
// object
type PortResponse struct {
	ContainerPort uint16
	Protocol      string
	HostPort      uint16 `json:",omitempty"`
}

// NewTaskResponse creates a new response object for the task
func NewTaskResponse(taskARN string,
	state dockerstate.TaskEngineState,
	cluster string) (*TaskResponse, error) {
	task, ok := state.TaskByArn(taskARN)
	if !ok {
		return nil, errors.Errorf("v2 task response: unable to find task '%s'", taskARN)
	}

	resp := &TaskResponse{
		Cluster:       cluster,
		TaskARN:       task.Arn,
		Family:        task.Family,
		Revision:      task.Version,
		DesiredStatus: task.GetDesiredStatus().String(),
		KnownStatus:   task.GetKnownStatus().String(),
	}

	taskCPU := task.CPU
	taskMemory := task.Memory
	if taskCPU != 0 || taskMemory != 0 {
		taskLimits := &LimitsResponse{}
		if taskCPU != 0 {
			taskLimits.CPU = &taskCPU
		}
		if taskMemory != 0 {
			taskLimits.Memory = &taskMemory
		}
		resp.Limits = taskLimits
	}

	if timestamp := task.GetPullStartedAt(); !timestamp.IsZero() {
		resp.PullStartedAt = aws.Time(timestamp.UTC())
	}
	if timestamp := task.GetPullStoppedAt(); !timestamp.IsZero() {
		resp.PullStoppedAt = aws.Time(timestamp.UTC())
	}
	if timestamp := task.GetExecutionStoppedAt(); !timestamp.IsZero() {
		resp.ExecutionStoppedAt = aws.Time(timestamp.UTC())
	}
	containerNameToDockerContainer, ok := state.ContainerMapByArn(task.Arn)
	if !ok {
		seelog.Warnf("V2 task response: unable to get container name mapping for task '%s'",
			task.Arn)
		return resp, nil
	}

	eni := task.GetTaskENI()
	for _, dockerContainer := range containerNameToDockerContainer {
		containerResponse := newContainerResponse(dockerContainer, eni, state)
		resp.Containers = append(resp.Containers, containerResponse)
	}

	return resp, nil
}

// NewContainerResponse creates a new container response based on container id
func NewContainerResponse(containerID string,
	state dockerstate.TaskEngineState) (*ContainerResponse, error) {
	dockerContainer, ok := state.ContainerByID(containerID)
	if !ok {
		return nil, errors.Errorf(
			"v2 container response: unable to find container '%s'", containerID)
	}
	task, ok := state.TaskByID(containerID)
	if !ok {
		return nil, errors.Errorf(
			"v2 container response: unable to find task for container '%s'", containerID)
	}

	resp := newContainerResponse(dockerContainer, task.GetTaskENI(), state)
	return &resp, nil
}

func newContainerResponse(dockerContainer *apicontainer.DockerContainer,
	eni *apieni.ENI,
	state dockerstate.TaskEngineState) ContainerResponse {
	container := dockerContainer.Container
	resp := ContainerResponse{
		ID:            dockerContainer.DockerID,
		Name:          container.Name,
		DockerName:    dockerContainer.DockerName,
		Image:         container.Image,
		ImageID:       container.ImageID,
		DesiredStatus: container.GetDesiredStatus().String(),
		KnownStatus:   container.GetKnownStatus().String(),
		Limits: LimitsResponse{
			CPU:    aws.Float64(float64(container.CPU)),
			Memory: aws.Int64(int64(container.Memory)),
		},
		Type:     container.Type.String(),
		ExitCode: container.GetKnownExitCode(),
		Labels:   container.GetLabels(),
	}

	// Write the container health status inside the container
	if dockerContainer.Container.HealthStatusShouldBeReported() {
		health := dockerContainer.Container.GetHealthStatus()
		resp.Health = &health
	}

	if createdAt := container.GetCreatedAt(); !createdAt.IsZero() {
		createdAt = createdAt.UTC()
		resp.CreatedAt = &createdAt
	}
	if startedAt := container.GetStartedAt(); !startedAt.IsZero() {
		startedAt = startedAt.UTC()
		resp.StartedAt = &startedAt
	}
	if finishedAt := container.GetFinishedAt(); !finishedAt.IsZero() {
		finishedAt = finishedAt.UTC()
		resp.FinishedAt = &finishedAt
	}

	for _, binding := range container.Ports {
		port := PortResponse{
			ContainerPort: binding.ContainerPort,
			Protocol:      binding.Protocol.String(),
		}
		if eni == nil {
			port.HostPort = binding.HostPort
		} else {
			port.HostPort = port.ContainerPort
		}

		resp.Ports = append(resp.Ports, port)
	}
	if eni != nil {
		resp.Networks = []containermetadata.Network{
			{
				NetworkMode:   "awsvpc",
				IPv4Addresses: eni.GetIPV4Addresses(),
				IPv6Addresses: eni.GetIPV6Addresses(),
			},
		}
	}

	return resp
}
