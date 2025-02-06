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

package v1

// AgentState is the interface for interacting with agent state relevant to the Introspection Server.
type AgentState interface {
	// Returns the agent's license text
	GetLicenseText() (string, error)
	// Returns agent metadata in v1 format.
	GetAgentMetadata() (*AgentMetadataResponse, error)
	// Returns task metadata in v1 format for all tasks on the host.
	GetTasksMetadata() (*TasksResponse, error)
	// Returns task metadata in v1 format for the task with a matching task Arn.
	GetTaskMetadataByArn(taskArn string) (*TaskResponse, error)
	// Returns task metadata in v1 format for the task with a matching docker ID.
	GetTaskMetadataByID(dockerID string) (*TaskResponse, error)
	// Returns task metadata in v1 format for the task with a matching short docker ID.
	GetTaskMetadataByShortID(shortDockerID string) (*TaskResponse, error)
}
