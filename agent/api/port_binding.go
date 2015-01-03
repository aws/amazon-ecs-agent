// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"strconv"

	"github.com/fsouza/go-dockerclient"
)

// PortBindingFromDockerPortBinding constructs a PortBinding slice from a docker
// NetworkSettings.Ports map.
func PortBindingFromDockerPortBinding(dockerPortBindings map[docker.Port][]docker.PortBinding) ([]PortBinding, error) {
	portBindings := make([]PortBinding, 0, len(dockerPortBindings))

	for port, bindings := range dockerPortBindings {
		intPort, err := strconv.Atoi(port.Port())
		if err != nil {
			return nil, err
		}
		containerPort := intPort
		for _, binding := range bindings {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil {
				return nil, err
			}
			portBindings = append(portBindings, PortBinding{
				ContainerPort: uint16(containerPort),
				HostPort:      uint16(hostPort),
				BindIp:        binding.HostIP,
			})
		}
	}
	return portBindings, nil
}
