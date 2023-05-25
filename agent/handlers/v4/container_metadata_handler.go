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
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	tmdsresponse "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/pkg/errors"
)

// GetContainerNetworkMetadata returns the network metadata for the container
func GetContainerNetworkMetadata(containerID string, state dockerstate.TaskEngineState) ([]tmdsv4.Network, error) {
	dockerContainer, ok := state.ContainerByID(containerID)
	if !ok {
		return nil, errors.Errorf("unable to find container '%s'", containerID)
	}
	// the logic here has been reused from
	// https://github.com/aws/amazon-ecs-agent/blob/0c8913ba33965cf6ffdd6253fad422458d9346bd/agent/containermetadata/parse_metadata.go#L123
	settings := dockerContainer.Container.GetNetworkSettings()
	if settings == nil {
		return nil, errors.Errorf("unable to generate network response for container '%s'", containerID)
	}
	// This metadata is the information provided in older versions of the API
	// We get the NetworkMode (Network interface name) from the HostConfig because this
	// this is the network with which the container is created
	ipv4AddressFromSettings := settings.IPAddress
	networkModeFromHostConfig := dockerContainer.Container.GetNetworkMode()

	// Extensive Network information is not available for Docker API versions 1.17-1.20
	// Instead we only get the details of the first network
	networks := make([]tmdsv4.Network, 0)
	if len(settings.Networks) > 0 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkMode := modeFromSettings
			ipv4Addresses := []string{containerNetwork.IPAddress}
			network := tmdsv4.Network{Network: tmdsresponse.Network{NetworkMode: networkMode, IPv4Addresses: ipv4Addresses}}
			networks = append(networks, network)
		}
	} else {
		ipv4Addresses := []string{ipv4AddressFromSettings}
		network := tmdsv4.Network{Network: tmdsresponse.Network{NetworkMode: networkModeFromHostConfig, IPv4Addresses: ipv4Addresses}}
		networks = append(networks, network)
	}
	return networks, nil
}
