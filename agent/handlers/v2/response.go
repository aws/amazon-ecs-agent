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

package v2

import (
	"github.com/aws/aws-sdk-go/aws/awserr"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	tmdsresponse "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	tmdsv2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// Agent versions >= 1.2.0: Null, zero, and CPU values of 1
// are passed to Docker as two CPU shares
const minimumCPUUnit = 2

// NewTaskResponse creates a new response object for the task
func NewTaskResponse(
	taskARN string,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	cluster string,
	az string,
	containerInstanceArn string,
	propagateTags bool,
	includeV4Metadata bool,
) (*tmdsv2.TaskResponse, error) {
	task, ok := state.TaskByArn(taskARN)
	if !ok {
		return nil, errors.Errorf("v2 task response: unable to find task '%s'", taskARN)
	}

	resp := &tmdsv2.TaskResponse{
		Cluster:          cluster,
		TaskARN:          task.Arn,
		Family:           task.Family,
		Revision:         task.Version,
		DesiredStatus:    task.GetDesiredStatus().String(),
		KnownStatus:      task.GetKnownStatus().String(),
		AvailabilityZone: az,
	}
	if includeV4Metadata {
		resp.LaunchType = task.LaunchType
	}

	taskCPU := task.CPU
	taskMemory := task.Memory
	if taskCPU != 0 || taskMemory != 0 {
		taskLimits := &tmdsv2.LimitsResponse{}
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

	for _, dockerContainer := range containerNameToDockerContainer {
		containerResponse := NewContainerResponse(dockerContainer, task.GetPrimaryENI(), includeV4Metadata)
		resp.Containers = append(resp.Containers, containerResponse)
	}

	if propagateTags {
		propagateTagsToMetadata(ecsClient, containerInstanceArn, taskARN, resp, includeV4Metadata)
	}

	return resp, nil
}

// propagateTagsToMetadata retrieves container instance and task tags from ECS
func propagateTagsToMetadata(ecsClient ecs.ECSClient, containerInstanceARN, taskARN string, resp *tmdsv2.TaskResponse, includeV4Metadata bool) {
	containerInstanceTags, err := ecsClient.GetResourceTags(containerInstanceARN)

	if err == nil {
		resp.ContainerInstanceTags = make(map[string]string)
		for _, tag := range containerInstanceTags {
			resp.ContainerInstanceTags[*tag.Key] = *tag.Value
		}
	} else {
		metadataErrorHandling(resp, err, "ContainerInstanceTags", containerInstanceARN, includeV4Metadata)
	}

	taskTags, err := ecsClient.GetResourceTags(taskARN)
	if err == nil {
		resp.TaskTags = make(map[string]string)
		for _, tag := range taskTags {
			resp.TaskTags[*tag.Key] = *tag.Value
		}
	} else {
		metadataErrorHandling(resp, err, "TaskTags", taskARN, includeV4Metadata)
	}
}

// NewContainerResponseFromState creates a new container response based on container id
// TODO: remove includeV4Metadata from NewContainerResponseFromState/NewContainerResponse
func NewContainerResponseFromState(
	containerID string,
	state dockerstate.TaskEngineState,
	includeV4Metadata bool,
) (*tmdsv2.ContainerResponse, error) {
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

	resp := NewContainerResponse(dockerContainer, task.GetPrimaryENI(), includeV4Metadata)
	return &resp, nil
}

// NewContainerResponse creates a new container response
// TODO: remove includeV4Metadata from NewContainerResponse
func NewContainerResponse(
	dockerContainer *apicontainer.DockerContainer,
	eni *ni.NetworkInterface,
	includeV4Metadata bool,
) tmdsv2.ContainerResponse {
	container := dockerContainer.Container
	resp := tmdsv2.ContainerResponse{
		ID:            dockerContainer.DockerID,
		Name:          container.Name,
		DockerName:    dockerContainer.DockerName,
		Image:         container.Image,
		ImageID:       container.ImageID,
		DesiredStatus: container.GetDesiredStatus().String(),
		KnownStatus:   container.GetKnownStatus().String(),
		Limits: tmdsv2.LimitsResponse{
			CPU:    aws.Float64(float64(container.CPU)),
			Memory: aws.Int64(int64(container.Memory)),
		},
		Type:     container.Type.String(),
		ExitCode: container.GetKnownExitCode(),
		Labels:   container.GetLabels(),
	}

	if container.CPU < minimumCPUUnit {
		defaultCPU := func(val float64) *float64 { return &val }(minimumCPUUnit)
		resp.Limits.CPU = defaultCPU
	}

	// V4 metadata endpoint calls this function for consistency across versions,
	// but needs additional metadata only available at this scope.
	if includeV4Metadata {
		resp.LogDriver = container.GetLogDriver()
		resp.LogOptions = container.GetLogOptions()
		resp.ContainerARN = container.ContainerArn
	}

	// Write the container health status inside the container
	if dockerContainer.Container.HealthStatusShouldBeReported() {
		health := dockerContainer.Container.GetHealthStatus()
		resp.Health = dockerContainerHealthToV2Health(health)
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

	for _, binding := range container.GetKnownPortBindings() {
		port := tmdsresponse.PortResponse{
			ContainerPort: binding.ContainerPort,
			Protocol:      binding.Protocol.String(),
		}
		if eni == nil {
			port.HostPort = binding.HostPort
		} else {
			port.HostPort = port.ContainerPort
		}
		if includeV4Metadata {
			port.HostIp = binding.BindIP
		}

		resp.Ports = append(resp.Ports, port)
	}

	if eni != nil {
		resp.Networks = []tmdsresponse.Network{
			{
				NetworkMode:   utils.NetworkModeAWSVPC,
				IPv4Addresses: eni.GetIPV4Addresses(),
				IPv6Addresses: eni.GetIPV6Addresses(),
			},
		}
	}

	resp.Volumes = v1.NewVolumesResponse(dockerContainer)
	return resp
}

// Converts apicontainer HealthStatus type to v2 Metadata HealthStatus type
func dockerContainerHealthToV2Health(health apicontainer.HealthStatus) *tmdsv2.HealthStatus {
	status := health.Status.String()
	if health.Status == apicontainerstatus.ContainerHealthUnknown {
		// Skip sending status if it is unknown
		status = ""
	}
	return &tmdsv2.HealthStatus{
		Status:   status,
		Since:    health.Since,
		ExitCode: health.ExitCode,
		Output:   health.Output,
	}
}

// metadataErrorHandling writes an error to the logger, and append an error response
// to V4 metadata endpoint task response
func metadataErrorHandling(resp *tmdsv2.TaskResponse, err error, field, resourceARN string, includeV4Metadata bool) {
	seelog.Errorf("Task Metadata error: unable to get '%s' for '%s': %s", field, resourceARN, err.Error())
	if includeV4Metadata {
		errResp := newErrorResponse(err, field, resourceARN)
		resp.Errors = append(resp.Errors, *errResp)
	}
}

// newErrorResponse creates a new error response
func newErrorResponse(err error, field, resourceARN string) *tmdsv2.ErrorResponse {
	errResp := &tmdsv2.ErrorResponse{
		ErrorField:   field,
		ErrorMessage: err.Error(),
		ResourceARN:  resourceARN,
	}

	if awsErr, ok := err.(awserr.Error); ok {
		errResp.ErrorCode = awsErr.Code()
		errResp.ErrorMessage = awsErr.Message()
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			errResp.StatusCode = reqErr.StatusCode()
			errResp.RequestId = reqErr.RequestID()
		}
	}

	return errResp
}
