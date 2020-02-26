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
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/pkg/errors"
)

// AssociationsResponse defines the schema for the associations response JSON object
type AssociationsResponse struct {
	Associations []string `json:"Associations"`
}

func NewAssociationsResponse(containerID, taskARN, associationType string, state dockerstate.TaskEngineState) (*AssociationsResponse, error) {
	dockerContainer, ok := state.ContainerByID(containerID)
	if !ok {
		return nil, errors.Errorf("unable to get container name from docker id: %s", containerID)
	}
	containerName := dockerContainer.Container.Name

	task, ok := state.TaskByArn(taskARN)
	if !ok {
		return nil, errors.Errorf("unable to get task from task arn: %s", taskARN)
	}

	associationNames := task.AssociationsByTypeAndContainer(associationType, containerName)

	return &AssociationsResponse{
		Associations: associationNames,
	}, nil
}

// Association response is a string that's assumed to be in valid JSON format, which will be exactly the same as
// the value of Association.Content.Value (cp is responsible to validate it and deal with it if it's not valid). We
// don't do any decoding base on the encoding, because the only encoding that cp currently sends us is 'identity';
// we don't explicitly model the value field as a struct because we don't want to let agent's implementation depends
// on the payload format of the association (i.e. eia device for now)
func NewAssociationResponse(taskARN, associationType, associationName string, state dockerstate.TaskEngineState) (string, error) {
	task, ok := state.TaskByArn(taskARN)
	if !ok {
		return "", errors.Errorf("unable to get task from task arn: %s", taskARN)
	}

	association, ok := task.AssociationByTypeAndName(associationType, associationName)

	if !ok {
		return "", errors.Errorf("unable to get association from association type %s and association name %s", associationType, associationName)
	}

	return association.Content.Value, nil
}
