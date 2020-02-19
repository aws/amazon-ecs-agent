// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package v3

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	v2 "github.com/aws/amazon-ecs-agent/agent/handlers/v2"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// ContainerMetadataPath specifies the relative URI path for serving container metadata.
var ContainerMetadataPath = "/v3/" + utils.ConstructMuxVar(V3EndpointIDMuxName, utils.AnythingButSlashRegEx)

// ContainerMetadataHandler returns the handler method for handling container metadata requests.
func ContainerMetadataHandler(state dockerstate.TaskEngineState) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		containerID, err := GetContainerIDByRequest(r, state)
		if err != nil {
			responseJSON, _ := json.Marshal(
				fmt.Sprintf("V3 container metadata handler: unable to get container ID from request: %s", err.Error()))
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerMetadata)
			return
		}
		containerResponse, err := GetContainerResponse(containerID, state)
		if err != nil {
			errResponseJSON, _ := json.Marshal(err.Error())
			utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerMetadata)
			return
		}
		seelog.Infof("V3 container metadata handler: writing response for container '%s'", containerID)

		responseJSON, _ := json.Marshal(containerResponse)
		utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerMetadata)
	}
}

// GetContainerResponse gets container response for v3 metadata
func GetContainerResponse(containerID string, state dockerstate.TaskEngineState) (*v2.ContainerResponse, error) {
	containerResponse, err := v2.NewContainerResponse(containerID, state)
	if err != nil {
		return nil, errors.Errorf("Unable to generate metadata for container '%s'", containerID)
	}
	// fill in network details if not set
	if containerResponse.Networks == nil {
		if containerResponse.Networks, err = GetContainerNetworkMetadata(containerID, state); err != nil {
			return nil, err
		}
	}
	return containerResponse, nil
}

// GetContainerNetworkMetadata returns the network metadata for the container
func GetContainerNetworkMetadata(containerID string, state dockerstate.TaskEngineState) ([]containermetadata.Network, error) {
	dockerContainer, ok := state.ContainerByID(containerID)
	if !ok {
		return nil, errors.Errorf("Unable to find container '%s'", containerID)
	}
	// the logic here has been reused from
	// https://github.com/aws/amazon-ecs-agent/blob/0c8913ba33965cf6ffdd6253fad422458d9346bd/agent/containermetadata/parse_metadata.go#L123
	settings := dockerContainer.Container.GetNetworkSettings()
	if settings == nil {
		return nil, errors.Errorf("Unable to generate network response for container '%s'", containerID)
	}
	// This metadata is the information provided in older versions of the API
	// We get the NetworkMode (Network interface name) from the HostConfig because this
	// this is the network with which the container is created
	ipv4AddressFromSettings := settings.IPAddress
	networkModeFromHostConfig := dockerContainer.Container.GetNetworkMode()

	// Extensive Network information is not available for Docker API versions 1.17-1.20
	// Instead we only get the details of the first network
	networks := make([]containermetadata.Network, 0)
	if len(settings.Networks) > 0 {
		for modeFromSettings, containerNetwork := range settings.Networks {
			networkMode := modeFromSettings
			ipv4Addresses := []string{containerNetwork.IPAddress}
			network := containermetadata.Network{NetworkMode: networkMode, IPv4Addresses: ipv4Addresses}
			networks = append(networks, network)
		}
	} else {
		ipv4Addresses := []string{ipv4AddressFromSettings}
		network := containermetadata.Network{NetworkMode: networkModeFromHostConfig, IPv4Addresses: ipv4Addresses}
		networks = append(networks, network)
	}
	return networks, nil
}
