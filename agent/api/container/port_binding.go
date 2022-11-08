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

package container

import (
	"strconv"

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/go-connections/nat"
)

const (
	// UnrecognizedTransportProtocolErrorName is an error where the protocol of the binding is invalid
	UnrecognizedTransportProtocolErrorName = "UnrecognizedTransportProtocol"
	// UnparseablePortErrorName is an error where the port configuration is invalid
	UnparseablePortErrorName = "UnparsablePort"
)

// PortBinding represents a port binding for a container
type PortBinding struct {
	// ContainerPort is the port inside the container
	ContainerPort *uint16
	// ContainerPortRange is a range of ports exposed inside the container
	ContainerPortRange *string
	// HostPort is the port exposed on the host
	HostPort uint16
	// BindIP is the IP address to which the port is bound
	BindIP string `json:"BindIp"`
	// Protocol is the protocol of the port
	Protocol TransportProtocol
}

// PortBindingFromDockerPortBinding constructs a PortBinding slice from a docker
// NetworkSettings.Ports map.
func PortBindingFromDockerPortBinding(dockerPortBindings nat.PortMap) ([]PortBinding, apierrors.NamedError) {
	portBindings := make([]PortBinding, 0, len(dockerPortBindings))

	for port, bindings := range dockerPortBindings {
		containerPort, err := nat.ParsePort(port.Port())
		if err != nil {
			return nil, &apierrors.DefaultNamedError{Name: UnparseablePortErrorName, Err: "Error parsing docker port as int " + err.Error()}
		}
		protocol, err := NewTransportProtocol(port.Proto())
		if err != nil {
			return nil, &apierrors.DefaultNamedError{Name: UnrecognizedTransportProtocolErrorName, Err: err.Error()}
		}

		for _, binding := range bindings {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil {
				return nil, &apierrors.DefaultNamedError{Name: UnparseablePortErrorName, Err: "Error parsing port binding as int " + err.Error()}
			}
			portBindings = append(portBindings, PortBinding{
				ContainerPort: aws.Uint16(uint16(containerPort)),
				HostPort:      uint16(hostPort),
				BindIP:        binding.HostIP,
				Protocol:      protocol,
			})
		}
	}
	return portBindings, nil
}
