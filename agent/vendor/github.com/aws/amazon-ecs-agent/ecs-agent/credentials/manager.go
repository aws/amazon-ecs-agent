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

package credentials

import (
	"fmt"
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	// CredentialsIDQueryParameterName is the name of GET query parameter for the task ID.
	CredentialsIDQueryParameterName = "id"

	// CredentialsPath is the path to the credentials handler.
	CredentialsPath = V2CredentialsPath

	V1CredentialsPath = "/v1/credentials"
	V2CredentialsPath = "/v2/credentials"

	// credentialsEndpointRelativeURIFormat defines the relative URI format
	// for the credentials endpoint. The place holders are the API Path and
	// credentials ID
	credentialsEndpointRelativeURIFormat = v2CredentialsEndpointRelativeURIFormat

	v1CredentialsEndpointRelativeURIFormat = "%s?" + CredentialsIDQueryParameterName + "=%s"
	v2CredentialsEndpointRelativeURIFormat = "%s/%s"

	// ApplicationRoleType specifies the credentials that are to be used by the
	// task itself
	ApplicationRoleType = "TaskApplication"

	// ExecutionRoleType specifies the credentials used for non task application
	// uses
	ExecutionRoleType = "TaskExecution"
)

// IAMRoleCredentials is used to save credentials sent by ACS
type IAMRoleCredentials struct {
	CredentialsID   string `json:"-"`
	RoleArn         string `json:"RoleArn"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"Token"`
	// Expiration is a string instead of a timestamp. This is to avoid any loss of context
	// while marshalling/unmarshalling this field in the agent. The agent just echo's
	// whatever is sent by the backend.
	Expiration string `json:"Expiration"`
	// RoleType distinguishes between TaskRole and ExecutionRole for the
	// credentials that are sent from the backend
	RoleType string `json:"-"`
}

// TaskIAMRoleCredentials wraps the task arn and the credentials object for the same
type TaskIAMRoleCredentials struct {
	ARN                string
	IAMRoleCredentials IAMRoleCredentials
}

// GetIAMRoleCredentials returns the IAM role credentials in the task IAM role struct
func (role *TaskIAMRoleCredentials) GetIAMRoleCredentials() IAMRoleCredentials {
	return role.IAMRoleCredentials
}

// GenerateCredentialsEndpointRelativeURI generates the relative URI for the
// credentials endpoint, for a given task id.
func (roleCredentials *IAMRoleCredentials) GenerateCredentialsEndpointRelativeURI() string {
	return fmt.Sprintf(credentialsEndpointRelativeURIFormat, CredentialsPath, roleCredentials.CredentialsID)
}

// credentialsManager implements the Manager interface. It is used to
// save credentials sent from ACS and to retrieve credentials from
// the credentials endpoint
type credentialsManager struct {
	// idToTaskCredentials maps credentials id to its corresponding TaskIAMRoleCredentials object
	// this map contains credentials id for which agent received the actual credentials from ACS already
	idToTaskCredentials map[string]TaskIAMRoleCredentials
	// knownCredentialsIDs tracks all credentials IDs we know about
	knownCredentialsIDs map[string]bool
	taskCredentialsLock sync.RWMutex
}

// IAMRoleCredentialsFromACS translates ecsacs.IAMRoleCredentials object to
// api.IAMRoleCredentials
func IAMRoleCredentialsFromACS(roleCredentials *ecsacs.IAMRoleCredentials, roleType string) IAMRoleCredentials {
	return IAMRoleCredentials{
		CredentialsID:   aws.ToString(roleCredentials.CredentialsId),
		SessionToken:    aws.ToString(roleCredentials.SessionToken),
		RoleArn:         aws.ToString(roleCredentials.RoleArn),
		AccessKeyID:     aws.ToString(roleCredentials.AccessKeyId),
		SecretAccessKey: aws.ToString(roleCredentials.SecretAccessKey),
		Expiration:      aws.ToString(roleCredentials.Expiration),
		RoleType:        roleType,
	}
}

// NewManager creates a new credentials manager object
func NewManager() Manager {
	return &credentialsManager{
		idToTaskCredentials: make(map[string]TaskIAMRoleCredentials),
		knownCredentialsIDs: make(map[string]bool),
	}
}

// SetTaskCredentials adds or updates credentials in the credentials manager
func (manager *credentialsManager) SetTaskCredentials(taskCredentials *TaskIAMRoleCredentials) error {
	manager.taskCredentialsLock.Lock()
	defer manager.taskCredentialsLock.Unlock()

	credentials := taskCredentials.IAMRoleCredentials
	// Validate that credentials id is not empty
	if credentials.CredentialsID == "" {
		return fmt.Errorf("CredentialsId is empty")
	}

	// Validate that task arn is not empty
	if taskCredentials.ARN == "" {
		return fmt.Errorf("task ARN is empty")
	}

	manager.idToTaskCredentials[credentials.CredentialsID] = TaskIAMRoleCredentials{
		ARN:                taskCredentials.ARN,
		IAMRoleCredentials: taskCredentials.GetIAMRoleCredentials(),
	}

	manager.knownCredentialsIDs[credentials.CredentialsID] = true

	return nil
}

// GetTaskCredentials retrieves credentials for a given credentials id
func (manager *credentialsManager) GetTaskCredentials(id string) (TaskIAMRoleCredentials, bool) {
	manager.taskCredentialsLock.RLock()
	defer manager.taskCredentialsLock.RUnlock()

	taskCredentials, ok := manager.idToTaskCredentials[id]

	if !ok {
		return TaskIAMRoleCredentials{}, ok
	}
	return TaskIAMRoleCredentials{
		ARN:                taskCredentials.ARN,
		IAMRoleCredentials: taskCredentials.GetIAMRoleCredentials(),
	}, ok
}

// RemoveCredentials removes credentials from the credentials manager
func (manager *credentialsManager) RemoveCredentials(id string) {
	manager.taskCredentialsLock.Lock()
	defer manager.taskCredentialsLock.Unlock()

	delete(manager.idToTaskCredentials, id)
	delete(manager.knownCredentialsIDs, id)
}

// IsCredentialsPending returns true if credentials ID is known but has not yet arrived from ACS.
func (manager *credentialsManager) IsCredentialsPending(id string) bool {
	manager.taskCredentialsLock.RLock()
	defer manager.taskCredentialsLock.RUnlock()

	_, isKnown := manager.knownCredentialsIDs[id]
	_, hasCredentials := manager.idToTaskCredentials[id]

	return isKnown && !hasCredentials
}

// AddKnownCredentialsID adds a credentials ID to the known set.
// This is useful when agent needs to track known credentials IDs
// for which the credentials themselves have not arrived from ACS.
func (manager *credentialsManager) AddKnownCredentialsID(id string) {
	manager.taskCredentialsLock.Lock()
	defer manager.taskCredentialsLock.Unlock()

	manager.knownCredentialsIDs[id] = true
}
