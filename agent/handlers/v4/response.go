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

package v4

import (
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	tmdsresponse "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/pkg/errors"
)

// NewTaskResponse creates a new v4 response object for the task. It augments v2 task response
// with additional fields for the v4 response.
func NewTaskResponse(
	taskARN string,
	state dockerstate.TaskEngineState,
	ecsClient ecs.ECSClient,
	cluster string,
	az string,
	vpcID string,
	containerInstanceARN string,
	serviceName string,
	propagateTags bool,
) (*tmdsv4.TaskResponse, error) {
	// Construct the v2 response first.
	v2Resp, err := v2.NewTaskResponse(taskARN, state, ecsClient, cluster, az,
		containerInstanceARN, propagateTags, true)
	if err != nil {
		return nil, err
	}
	var containers []tmdsv4.ContainerResponse
	// Convert each container response into v4 container response.
	for i, container := range v2Resp.Containers {
		networks, err := toV4NetworkResponse(container.Networks, func() (*apitask.Task, bool) {
			return state.TaskByArn(taskARN)
		})
		if err != nil {
			return nil, err
		}
		v4Response := &tmdsv4.ContainerResponse{
			ContainerResponse: &v2Resp.Containers[i],
			Networks:          networks,
		}
		v4Response = augmentContainerResponse(container.ID, state, v4Response)
		containers = append(containers, *v4Response)
	}

	return &tmdsv4.TaskResponse{
		TaskResponse: v2Resp,
		Containers:   containers,
		VPCID:        vpcID,
		ServiceName:  serviceName,
	}, nil
}

// NewContainerResponse creates a new v4 container response based on container id. It augments
// v2 container response with additional fields for v4.
func NewContainerResponse(
	containerID string,
	state dockerstate.TaskEngineState,
) (*tmdsv4.ContainerResponse, error) {
	// Construct the v2 response first.
	container, err := v2.NewContainerResponseFromState(containerID, state, true)
	if err != nil {
		return nil, err
	}
	// Convert v2 network responses into v4 network responses.
	networks, err := toV4NetworkResponse(container.Networks, func() (*apitask.Task, bool) {
		return state.TaskByID(containerID)
	})
	if err != nil {
		return nil, err
	}
	v4Response := tmdsv4.ContainerResponse{
		ContainerResponse: container,
		Networks:          networks,
	}
	return augmentContainerResponse(containerID, state, &v4Response), nil
}

// toV4NetworkResponse converts v2 network response to v4. Additional fields are only
// added if the networking mode is 'awsvpc'. The `lookup` function pointer is used to
// look up the task information in the local state based on the id, which could be
// either task arn or contianer id.
func toV4NetworkResponse(
	networks []tmdsresponse.Network,
	lookup func() (*apitask.Task, bool),
) ([]tmdsv4.Network, error) {
	var resp []tmdsv4.Network
	for _, network := range networks {
		respNetwork := tmdsv4.Network{Network: network}
		if network.NetworkMode == utils.NetworkModeAWSVPC {
			task, ok := lookup()
			if !ok {
				return nil, errors.New("v4 task response: unable to find task")
			}
			props, err := newNetworkInterfaceProperties(task)
			if err != nil {
				return nil, err
			}
			respNetwork.NetworkInterfaceProperties = props
		}
		resp = append(resp, respNetwork)
	}

	return resp, nil
}

// augmentContainerResponse augments the container response with additional fields.
func augmentContainerResponse(
	containerID string,
	state dockerstate.TaskEngineState,
	v4Response *tmdsv4.ContainerResponse,
) *tmdsv4.ContainerResponse {
	dockerContainer, ok := state.ContainerByID(containerID)
	if !ok {
		// did not find container, continue on and try next container(s)
		// we don't return error here to avoid disrupting all of a TMDS response
		// on a single missing container.
		logger.Warn("V4 container response: unable to find container in internal state",
			logger.Fields{
				field.RuntimeID: containerID,
			})
		return v4Response
	}
	if dockerContainer.Container.RestartPolicyEnabled() {
		restartCount := dockerContainer.Container.RestartTracker.GetRestartCount()
		v4Response.RestartCount = &restartCount
	}
	return v4Response
}

// newNetworkInterfaceProperties creates the NetworkInterfaceProperties object for a given
// task.
func newNetworkInterfaceProperties(task *apitask.Task) (tmdsv4.NetworkInterfaceProperties, error) {
	eni := task.GetPrimaryENI()

	var attachmentIndexPtr *int
	if task.IsNetworkModeAWSVPC() {
		var vpcIndex = 0
		attachmentIndexPtr = &vpcIndex
	}

	var subnetGatewayIPv6Address string
	if eni.IPv6Only() {
		// Only populate SubnetGatewayIPv6Address for IPv6-only task ENIs as we do not
		// populate it for dual-stack ENIs for historical reasons
		subnetGatewayIPv6Address = eni.SubnetGatewayIPV6Address
	}
	return tmdsv4.NetworkInterfaceProperties{
		// TODO this is hard-coded to `0` for now. Once backend starts populating
		// `Index` field for an ENI, we should set it as per that. Since we
		// only support 1 ENI per task anyway, setting it to `0` is acceptable
		AttachmentIndex:          attachmentIndexPtr,
		IPV4SubnetCIDRBlock:      eni.GetIPv4SubnetCIDRBlock(),
		IPv6SubnetCIDRBlock:      eni.GetIPv6SubnetCIDRBlock(),
		MACAddress:               eni.MacAddress,
		DomainNameServers:        eni.DomainNameServers,
		DomainNameSearchList:     eni.DomainNameSearchList,
		PrivateDNSName:           eni.PrivateDNSName,
		SubnetGatewayIPV4Address: eni.SubnetGatewayIPV4Address,
		SubnetGatewayIPV6Address: subnetGatewayIPv6Address,
	}, nil
}

// NewPulledContainerResponse creates a new v4 container response for a pulled container.
// It augments v4 container response with an additional empty network interface field.
func NewPulledContainerResponse(
	dockerContainer *apicontainer.DockerContainer,
	eni *ni.NetworkInterface,
) tmdsv4.ContainerResponse {
	resp := v2.NewContainerResponse(dockerContainer, eni, true)
	return tmdsv4.ContainerResponse{
		ContainerResponse: &resp,
	}
}
