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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/ecs-agent/introspection"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	tmdsresponse "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	tmdsutils "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
)

// NewTaskResponse creates a TaskResponse for a task.
func NewTaskResponse(task *apitask.Task, containerMap map[string]*apicontainer.DockerContainer) *introspection.TaskResponse {
	containers := []introspection.ContainerResponse{}
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

	return &introspection.TaskResponse{
		Arn:           task.Arn,
		DesiredStatus: desiredStatus,
		KnownStatus:   knownBackendStatus,
		Family:        task.Family,
		Version:       task.Version,
		Containers:    containers,
	}
}

// NewContainerResponse creates ContainerResponse for a container.
func NewContainerResponse(dockerContainer *apicontainer.DockerContainer, eni *ni.NetworkInterface) introspection.ContainerResponse {
	container := dockerContainer.Container
	resp := introspection.ContainerResponse{
		Name:       container.Name,
		Image:      container.Image,
		ImageID:    container.ImageID,
		DockerID:   dockerContainer.DockerID,
		DockerName: dockerContainer.DockerName,
	}

	resp.Ports = NewPortBindingsResponse(dockerContainer, eni)
	resp.Volumes = NewVolumesResponse(dockerContainer)

	if eni != nil {
		resp.Networks = []tmdsresponse.Network{
			{
				NetworkMode:   tmdsutils.NetworkModeAWSVPC,
				IPv4Addresses: eni.GetIPV4Addresses(),
				IPv6Addresses: eni.GetIPV6Addresses(),
			},
		}
	}
	if createdAt := container.GetCreatedAt(); !createdAt.IsZero() {
		createdAt = createdAt.UTC()
		resp.CreatedAt = &createdAt
	}
	if startedAt := container.GetStartedAt(); !startedAt.IsZero() {
		startedAt = startedAt.UTC()
		resp.StartedAt = &startedAt
	}
	if container.RestartPolicyEnabled() {
		restartCount := container.RestartTracker.GetRestartCount()
		resp.RestartCount = &restartCount
	}
	return resp
}

// NewPortBindingsResponse creates PortResponse for a container.
func NewPortBindingsResponse(dockerContainer *apicontainer.DockerContainer, eni *ni.NetworkInterface) []tmdsresponse.PortResponse {
	container := dockerContainer.Container
	resp := []tmdsresponse.PortResponse{}

	bindings := container.GetKnownPortBindings()

	// if KnownPortBindings list is empty, then we use the port mapping
	// information that was passed down from ACS.
	if len(bindings) == 0 {
		bindings = container.Ports
	}

	for _, binding := range bindings {
		port := tmdsresponse.PortResponse{
			ContainerPort: binding.ContainerPort,
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
func NewVolumesResponse(dockerContainer *apicontainer.DockerContainer) []tmdsresponse.VolumeResponse {
	container := dockerContainer.Container
	var resp []tmdsresponse.VolumeResponse

	volumes := container.GetVolumes()

	for _, volume := range volumes {
		volResp := tmdsresponse.VolumeResponse{
			DockerName:  volume.Name,
			Source:      volume.Source,
			Destination: volume.Destination,
		}

		resp = append(resp, volResp)
	}
	return resp
}
