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
	"net"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/pkg/errors"
)

// TaskResponse is the v4 Task response. It augments the v4 Container response
// with the v2 task response object.
type TaskResponse struct {
	*v2.TaskResponse
	Containers []ContainerResponse `json:"Containers,omitempty"`
}

// ContainerResponse is the v4 Container response. It augments the v4 Network response
// with the v2 container response object.
type ContainerResponse struct {
	*v2.ContainerResponse
	Networks []Network `json:"Networks,omitempty"`
}

// Network is the v4 Network response. It adds a bunch of information about network
// interface(s) on top of what is supported by v4.
// See `NetworkInterfaceProperties` for more details.
type Network struct {
	containermetadata.Network
	// NetworkInterfaceProperties specifies additional properties of the network
	// of the network interface that are exposed via the metadata server.
	// We currently populate this only for the `awsvpc` networking mode.
	NetworkInterfaceProperties
}

// NetworkInterfaceProperties represents additional properties we may want to expose via
// task metadata about the network interface that's attached to the container/task. We
// mostly use this to populate data about ENIs for tasks launched with `awsvpc` mode.
type NetworkInterfaceProperties struct {
	// AttachmentIndex reflects the `index` specified by the customer (if any)
	// while creating the task with `awsvpc` mode.
	AttachmentIndex int `json:"AttachmentIndex"`
	// IPV4SubnetCIDRBlock is the subnet CIDR netmask associated with network interface.
	IPV4SubnetCIDRBlock string `json:"IPv4SubnetCIDRBlock,ommitempty"`
	// MACAddress is the mac address of the network interface.
	MACAddress string `json:"MACAddress,ommitempty"`
	// DomainNameServers specifies the nameserver IP addresses for the network interface.
	DomainNameServers []string `json:"DomainNameServers,omitempty"`
	// DomainNameSearchList specifies the search list for the domain name lookup for
	// the network interface.
	DomainNameSearchList []string `json:"DomainNameSearchList,omitempty"`
	// PrivateDNSName is the dns name assigned to this network interface.
	PrivateDNSName string `json:"PrivateDNSName,omitempty"`
	// SubnetGatewayIPV4Address is the gateway address for the network interface.
	SubnetGatewayIPV4Address string `json:"SubnetGatewayIpv4Address,ommitempty"`
}

// NewTaskResponse creates a new v4 response object for the task. It augments v2 task response
// with additional network interface fields.
func NewTaskResponse(
	taskARN string,
	state dockerstate.TaskEngineState,
	ecsClient api.ECSClient,
	cluster string,
	az string,
	containerInstanceARN string,
	propagateTags bool,
) (*TaskResponse, error) {
	// Construct the v2 response first.
	v2Resp, err := v2.NewTaskResponse(taskARN, state, ecsClient, cluster, az,
		containerInstanceARN, propagateTags)
	if err != nil {
		return nil, err
	}
	var containers []ContainerResponse
	// Convert each container response into v4 container response.
	for _, container := range v2Resp.Containers {
		networks, err := toV4NetworkResponse(container.Networks, func() (*apitask.Task, bool) {
			return state.TaskByArn(taskARN)
		})
		if err != nil {
			return nil, err
		}
		containers = append(containers, ContainerResponse{
			ContainerResponse: &container,
			Networks:          networks,
		})
	}

	return &TaskResponse{
		TaskResponse: v2Resp,
		Containers:   containers,
	}, nil
}

// NewContainerResponse creates a new v4 container response based on container id.  It augments
// v4 container response with additional network interface fields.
func NewContainerResponse(
	containerID string,
	state dockerstate.TaskEngineState,
) (*ContainerResponse, error) {
	// Construct the v2 response first.
	container, err := v2.NewContainerResponse(containerID, state)
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
	return &ContainerResponse{
		ContainerResponse: container,
		Networks:          networks,
	}, nil
}

// toV4NetworkResponse converts v2 network response to v4. Additional fields are only
// added if the networking mode is 'awsvpc'. The `lookup` function pointer is used to
// look up the task information in the local state based on the id, which could be
// either task arn or contianer id.
func toV4NetworkResponse(
	networks []containermetadata.Network,
	lookup func() (*apitask.Task, bool),
) ([]Network, error) {
	var resp []Network
	for _, network := range networks {
		respNetwork := Network{Network: network}
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

// newNetworkInterfaceProperties creates the NetworkInterfaceProperties object for a given
// task.
func newNetworkInterfaceProperties(task *apitask.Task) (NetworkInterfaceProperties, error) {
	eni := task.GetPrimaryENI()
	_, ipv4Net, err := net.ParseCIDR(eni.SubnetGatewayIPV4Address)
	if err != nil {
		return NetworkInterfaceProperties{}, errors.Wrapf(err,
			"v4 metadata response: unable to parse subnet ipv4 address '%s'",
			eni.SubnetGatewayIPV4Address)
	}
	return NetworkInterfaceProperties{
		// TODO this is hard-coded to `0` for now. Once backend starts populating
		// `Index` field for an ENI, we should set it as per that. Since we
		// only support 1 ENI per task anyway, setting it to `0` is acceptable.
		AttachmentIndex:          0,
		IPV4SubnetCIDRBlock:      ipv4Net.String(),
		MACAddress:               eni.MacAddress,
		DomainNameServers:        eni.DomainNameServers,
		DomainNameSearchList:     eni.DomainNameSearchList,
		PrivateDNSName:           eni.PrivateDNSName,
		SubnetGatewayIPV4Address: eni.SubnetGatewayIPV4Address,
	}, nil
}
