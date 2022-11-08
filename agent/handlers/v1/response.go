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

package v1

import (
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"

	"github.com/aws/aws-sdk-go/aws"
)

// MetadataResponse is the schema for the metadata response JSON object
type MetadataResponse struct {
	Cluster              string  `json:"Cluster"`
	ContainerInstanceArn *string `json:"ContainerInstanceArn"`
	Version              string  `json:"Version"`
}

// TaskResponse is the schema for the task response JSON object
type TaskResponse struct {
	Arn           string              `json:"Arn"`
	DesiredStatus string              `json:"DesiredStatus,omitempty"`
	KnownStatus   string              `json:"KnownStatus"`
	Family        string              `json:"Family"`
	Version       string              `json:"Version"`
	Containers    []ContainerResponse `json:"Containers"`
}

// TasksResponse is the schema for the tasks response JSON object
type TasksResponse struct {
	Tasks []*TaskResponse `json:"Tasks"`
}

// ContainerResponse is the schema for the container response JSON object
type ContainerResponse struct {
	DockerID   string                      `json:"DockerId"`
	DockerName string                      `json:"DockerName"`
	Name       string                      `json:"Name"`
	Ports      []PortResponse              `json:"Ports,omitempty"`
	Networks   []containermetadata.Network `json:"Networks,omitempty"`
	Volumes    []VolumeResponse            `json:"Volumes,omitempty"`
}

// VolumeResponse is the schema for the volume response JSON object
type VolumeResponse struct {
	DockerName  string `json:"DockerName,omitempty"`
	Source      string `json:"Source,omitempty"`
	Destination string `json:"Destination,omitempty"`
}

// PortResponse defines the schema for portmapping response JSON
// object.
type PortResponse struct {
	ContainerPort uint16 `json:"ContainerPort,omitempty"`
	Protocol      string `json:"Protocol,omitempty"`
	HostPort      uint16 `json:"HostPort,omitempty"`
	HostIp        string `json:"HostIp,omitempty"`
}

// NewTaskResponse creates a TaskResponse for a task.
func NewTaskResponse(task *apitask.Task, containerMap map[string]*apicontainer.DockerContainer) *TaskResponse {
	containers := []ContainerResponse{}
	for _, container := range containerMap {
		if container.Container.IsInternal() {
			continue
		}
		containerResponse := NewContainerResponse(container, task.GetPrimaryENI())
		containers = append(containers, containerResponse)
	}

	knownStatus := task.GetKnownStatus()
	knownBackendStatus := knownStatus.BackendStatus()
	desiredStatusInAgent := task.GetDesiredStatus()
	desiredStatus := desiredStatusInAgent.BackendStatus()

	if (knownBackendStatus == "STOPPED" && desiredStatus != "STOPPED") || (knownBackendStatus == "RUNNING" && desiredStatus == "PENDING") {
		desiredStatus = ""
	}

	return &TaskResponse{
		Arn:           task.Arn,
		DesiredStatus: desiredStatus,
		KnownStatus:   knownBackendStatus,
		Family:        task.Family,
		Version:       task.Version,
		Containers:    containers,
	}
}

// NewContainerResponse creates ContainerResponse for a container.
func NewContainerResponse(dockerContainer *apicontainer.DockerContainer, eni *apieni.ENI) ContainerResponse {
	container := dockerContainer.Container
	resp := ContainerResponse{
		Name:       container.Name,
		DockerID:   dockerContainer.DockerID,
		DockerName: dockerContainer.DockerName,
	}

	resp.Ports = NewPortBindingsResponse(dockerContainer, eni)
	resp.Volumes = NewVolumesResponse(dockerContainer)

	if eni != nil {
		resp.Networks = []containermetadata.Network{
			{
				NetworkMode:   utils.NetworkModeAWSVPC,
				IPv4Addresses: eni.GetIPV4Addresses(),
				IPv6Addresses: eni.GetIPV6Addresses(),
			},
		}
	}
	return resp
}

// NewPortBindingsResponse creates PortResponse for a container.
func NewPortBindingsResponse(dockerContainer *apicontainer.DockerContainer, eni *apieni.ENI) []PortResponse {
	container := dockerContainer.Container
	resp := []PortResponse{}

	bindings := container.GetKnownPortBindings()

	// if KnownPortBindings list is empty, then we use the port mapping
	// information that was passed down from ACS.
	if len(bindings) == 0 {
		bindings = container.Ports
	}

	for _, binding := range bindings {
		port := PortResponse{
			ContainerPort: aws.Uint16Value(binding.ContainerPort),
			Protocol:      binding.Protocol.String(),
		}

		if eni == nil {
			port.HostPort = binding.HostPort
		} else {
			port.HostPort = port.ContainerPort
		}

		resp = append(resp, port)
	}
	return resp
}

// NewVolumesResponse creates VolumeResponse for a container
func NewVolumesResponse(dockerContainer *apicontainer.DockerContainer) []VolumeResponse {
	container := dockerContainer.Container
	var resp []VolumeResponse

	volumes := container.GetVolumes()

	for _, volume := range volumes {
		volResp := VolumeResponse{
			DockerName:  volume.Name,
			Source:      volume.Source,
			Destination: volume.Destination,
		}

		resp = append(resp, volResp)
	}
	return resp
}

// NewTasksResponse creates TasksResponse for all the tasks.
func NewTasksResponse(state dockerstate.TaskEngineState) *TasksResponse {
	allTasks := state.AllExternalTasks()
	taskResponses := make([]*TaskResponse, len(allTasks))
	for ndx, task := range allTasks {
		containerMap, _ := state.ContainerMapByArn(task.Arn)
		taskResponses[ndx] = NewTaskResponse(task, containerMap)
	}

	return &TasksResponse{Tasks: taskResponses}
}
