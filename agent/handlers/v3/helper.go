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

package v3

import (
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	"github.com/pkg/errors"
)

func GetTaskARNByRequest(r *http.Request, state dockerstate.TaskEngineState) (string, error) {
	v3EndpointID, ok := utils.GetMuxValueFromRequest(r, V3EndpointIDMuxName)
	if !ok {
		return "", errors.New("unable to get v3 endpoint ID from request")
	}

	// Get task Arn from the v3 endpoint ID.
	taskARN, ok := state.TaskARNByV3EndpointID(v3EndpointID)
	if !ok {
		return "", errors.Errorf("unable to get task Arn from v3 endpoint ID: %s", v3EndpointID)
	}

	return taskARN, nil
}

func GetContainerIDByRequest(r *http.Request, state dockerstate.TaskEngineState) (string, error) {
	v3EndpointID, ok := utils.GetMuxValueFromRequest(r, V3EndpointIDMuxName)
	if !ok {
		return "", errors.New("unable to get v3 endpoint ID from request")
	}

	// Get docker ID from the v3 endpoint ID.
	dockerID, ok := state.DockerIDByV3EndpointID(v3EndpointID)
	if !ok {
		return "", errors.Errorf("unable to get docker ID from v3 endpoint ID: %s", v3EndpointID)
	}

	return dockerID, nil
}

func GetAssociationTypeByRequest(r *http.Request) (string, error) {
	associationType, ok := utils.GetMuxValueFromRequest(r, associationTypeMuxName)
	if !ok {
		return "", errors.New("unable to get association type from request")
	}

	return associationType, nil
}

func GetAssociationNameByRequest(r *http.Request) (string, error) {
	associationType, ok := utils.GetMuxValueFromRequest(r, associationNameMuxName)
	if !ok {
		return "", errors.New("unable to get association name from request")
	}

	return associationType, nil
}
