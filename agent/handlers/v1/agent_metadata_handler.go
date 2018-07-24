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

package v1

import (
	"encoding/json"
	"net/http"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/handlers/utils"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
)

// AgentMetadataPath is the Agent metadata path for v1 handler.
const AgentMetadataPath = "/v1/metadata"

// AgentMetadataHandler creates response for 'v1/metadata' API.
func AgentMetadataHandler(containerInstanceArn *string, cfg *config.Config) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := &MetadataResponse{
			Cluster:              cfg.Cluster,
			ContainerInstanceArn: containerInstanceArn,
			Version:              agentversion.String(),
		}
		responseJSON, _ := json.Marshal(resp)
		utils.WriteJSONToResponse(w, http.StatusOK, responseJSON, utils.RequestTypeAgentMetadata)
	}
}
