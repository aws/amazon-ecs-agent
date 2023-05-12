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
	"encoding/json"
	"fmt"
	"net/http"

	v3 "github.com/aws/amazon-ecs-agent/agent/handlers/v3"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/utils"
	"github.com/cihub/seelog"
)

const (
	// associationTypeMuxName is the key that's used in gorilla/mux to get the association type.
	associationTypeMuxName = "associationTypeMuxName"
	// associationNameMuxName is the key that's used in gorilla/mux to get the association name.
	associationNameMuxName = "associationNameMuxName"
)

var (
	// Container associations endpoint: /v4/<v3 endpoint id>/<association type>
	ContainerAssociationsPath = fmt.Sprintf("/v4/%s/associations/%s",
		utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx),
		utils.ConstructMuxVar(associationTypeMuxName, utils.AnythingButSlashRegEx))
	// Container association endpoint: /v4/<v3 endpoint id>/<association type>/<association name>
	ContainerAssociationPath = fmt.Sprintf("/v4/%s/associations/%s/%s",
		utils.ConstructMuxVar(v3.V3EndpointIDMuxName, utils.AnythingButSlashRegEx),
		utils.ConstructMuxVar(associationTypeMuxName, utils.AnythingButSlashRegEx),
		utils.ConstructMuxVar(associationNameMuxName, utils.AnythingButEmptyRegEx))
	// Treat "/v4/<v3 endpoint id>/<association type>/" as a container association endpoint with empty association name (therefore invalid), to be consistent with similar situation in credentials endpoint and v3 metadata endpoint
	ContainerAssociationPathWithSlash = ContainerAssociationsPath + "/"
)

// ContainerAssociationHandler returns the handler method for handling container associations requests.
func ContainerAssociationsHandler(state dockerstate.TaskEngineState) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		containerID, err := v3.GetContainerIDByRequest(r, state)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V4 container associations handler: unable to get container id from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerAssociations)
			return
		}

		taskARN, err := v3.GetTaskARNByRequest(r, state)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V4 container associations handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerAssociations)
			return
		}

		associationType, err := v3.GetAssociationTypeByRequest(r)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V4 container associations handler: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerAssociations)
			return
		}

		seelog.Infof("V4 container associations handler: writing response for container '%s' for association type %s", containerID, associationType)

		writeContainerAssociationsResponse(w, containerID, taskARN, associationType, state)
	}
}

// ContainerAssociationHandler returns the handler method for handling container association requests.
func ContainerAssociationHandler(state dockerstate.TaskEngineState) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		taskARN, err := v3.GetTaskARNByRequest(r, state)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V4 container associations handler: unable to get task arn from request: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerAssociation)
			return
		}

		associationType, err := v3.GetAssociationTypeByRequest(r)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V4 container associations handler: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerAssociation)
			return
		}

		associationName, err := v3.GetAssociationNameByRequest(r)
		if err != nil {
			responseJSON, err := json.Marshal(
				fmt.Sprintf("V4 container associations handler: %s", err.Error()))
			if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
				return
			}
			utils.WriteJSONToResponse(w, http.StatusBadRequest, responseJSON, utils.RequestTypeContainerAssociation)
			return
		}

		seelog.Infof("V4 container association handler: writing response for association '%s' of type %s", associationName, associationType)

		writeContainerAssociationResponse(w, taskARN, associationType, associationName, state)
	}
}

func writeContainerAssociationsResponse(w http.ResponseWriter, containerID, taskARN, associationType string, state dockerstate.TaskEngineState) {
	associationsResponse, err := v3.NewAssociationsResponse(containerID, taskARN, associationType, state)
	if err != nil {
		errResponseJSON, err := json.Marshal(fmt.Sprintf("Unable to write container associations response: %s", err.Error()))
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerAssociations)
		return
	}

	responseJSON, err := json.Marshal(associationsResponse)
	if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
		return
	}
	utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeContainerAssociations)
}

func writeContainerAssociationResponse(w http.ResponseWriter, taskARN, associationType, associationName string, state dockerstate.TaskEngineState) {
	associationResponse, err := v3.NewAssociationResponse(taskARN, associationType, associationName, state)
	if err != nil {
		errResponseJSON, err := json.Marshal(fmt.Sprintf("Unable to write container association response: %s", err.Error()))
		if e := utils.WriteResponseIfMarshalError(w, err); e != nil {
			return
		}
		utils.WriteJSONToResponse(w, http.StatusBadRequest, errResponseJSON, utils.RequestTypeContainerAssociation)
		return
	}

	// associationResponse is assumed to be a valid json string; see comments on newAssociationResponse method for details
	utils.WriteJSONToResponse(w, http.StatusOK, []byte(associationResponse), utils.RequestTypeContainerAssociation)
}
