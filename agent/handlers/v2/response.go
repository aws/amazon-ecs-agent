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
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// TaskResponse defines the schema for the task response JSON object
type TaskResponse struct {
	Cluster               string              `json:"Cluster"`
	TaskARN               string              `json:"TaskARN"`
	Family                string              `json:"Family"`
	Revision              string              `json:"Revision"`
	DesiredStatus         string              `json:"DesiredStatus,omitempty"`
	KnownStatus           string              `json:"KnownStatus"`
	Containers            []ContainerResponse `json:"Containers,omitempty"`
	Limits                *LimitsResponse     `json:"Limits,omitempty"`
	PullStartedAt         *time.Time          `json:"PullStartedAt,omitempty"`
	PullStoppedAt         *time.Time          `json:"PullStoppedAt,omitempty"`
	ExecutionStoppedAt    *time.Time          `json:"ExecutionStoppedAt,omitempty"`
	AvailabilityZone      string              `json:"AvailabilityZone,omitempty"`
	TaskTags              map[string]string   `json:"TaskTags,omitempty"`
	ContainerInstanceTags map[string]string   `json:"ContainerInstanceTags,omitempty"`
	LaunchType            string              `json:"LaunchType,omitempty"`
	Errors                []ErrorResponse     `json:"Errors,omitempty"`
}

// ContainerResponse defines the schema for the container response
// JSON object
type ContainerResponse struct {
	ID            string                      `json:"DockerId"`
	Name          string                      `json:"Name"`
	DockerName    string                      `json:"DockerName"`
	Image         string                      `json:"Image"`
	ImageID       string                      `json:"ImageID"`
	Ports         []v1.PortResponse           `json:"Ports,omitempty"`
	Labels        map[string]string           `json:"Labels,omitempty"`
	DesiredStatus string                      `json:"DesiredStatus"`
	KnownStatus   string                      `json:"KnownStatus"`
	ExitCode      *int                        `json:"ExitCode,omitempty"`
	Limits        LimitsResponse              `json:"Limits"`
	CreatedAt     *time.Time                  `json:"CreatedAt,omitempty"`
	StartedAt     *time.Time                  `json:"StartedAt,omitempty"`
	FinishedAt    *time.Time                  `json:"FinishedAt,omitempty"`
	Type          string                      `json:"Type"`
	Networks      []containermetadata.Network `json:"Networks,omitempty"`
	Health        *apicontainer.HealthStatus  `json:"Health,omitempty"`
	Volumes       []v1.VolumeResponse         `json:"Volumes,omitempty"`
	LogDriver     string                      `json:"LogDriver,omitempty"`
	LogOptions    map[string]string           `json:"LogOptions,omitempty"`
	ContainerARN  string                      `json:"ContainerARN,omitempty"`
}

// LimitsResponse defines the schema for task/cpu limits response
// JSON object
type LimitsResponse struct {
	CPU    *float64 `json:"CPU,omitempty"`
	Memory *int64   `json:"Memory,omitempty"`
}

// ErrorResponse defined the schema for error response
// JSON object
type ErrorResponse struct {
	ErrorField   string `json:"ErrorField,omitempty"`
	ErrorCode    string `json:"ErrorCode,omitempty"`
	ErrorMessage string `json:"ErrorMessage,omitempty"`
	StatusCode   int    `json:"StatusCode,omitempty"`
	RequestId    string `json:"RequestId,omitempty"`
	ResourceARN  string `json:"ResourceARN,omitempty"`
}

// Agent versions >= 1.2.0: Null, zero, and CPU values of 1
// are passed to Docker as two CPU shares
const minimumCPUUnit = 2

// NewTaskResponse creates a new response object for the task
func NewTaskResponse(
	taskARN string,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	cluster string,
	az string,
	containerInstanceArn string,
	propagateTags bool,
	includeV4Metadata bool,
) (*TaskResponse, error) {
	task, ok := state.TaskByArn(taskARN)
	if !ok {
		return nil, errors.Errorf("v2 task response: unable to find task '%s'", taskARN)
	}

	resp := &TaskResponse{
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
func propagateTagsToMetadata(ecsClient api.ECSClient, containerInstanceARN, taskARN string, resp *TaskResponse, includeV4Metadata bool) {
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
) (*ContainerResponse, error) {
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
	eni *apieni.ENI,
	includeV4Metadata bool,
) ContainerResponse {
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

	for _, binding := range container.GetKnownPortBindings() {
		port := v1.PortResponse{
			ContainerPort: aws.Uint16Value(binding.ContainerPort),
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
		resp.Networks = []containermetadata.Network{
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

// metadataErrorHandling writes an error to the logger, and append an error response
// to V4 metadata endpoint task response
func metadataErrorHandling(resp *TaskResponse, err error, field, resourceARN string, includeV4Metadata bool) {
	seelog.Errorf("Task Metadata error: unable to get '%s' for '%s': %s", field, resourceARN, err.Error())
	if includeV4Metadata {
		errResp := newErrorResponse(err, field, resourceARN)
		resp.Errors = append(resp.Errors, *errResp)
	}
}

// newErrorResponse creates a new error response
func newErrorResponse(err error, field, resourceARN string) *ErrorResponse {
	errResp := &ErrorResponse{
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
