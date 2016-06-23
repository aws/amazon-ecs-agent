// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"net/url"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	// CredentialsIdQueryParameterName is the name of GET query parameter for the task ID.
	CredentialsIdQueryParameterName = "id"

	// CredentialsPath is the path to the credentials handler.
	CredentialsPath = "/v1/credentials"

	// credentialsEndpointRelativeURIFormat defines the relative URI format
	// for the credentials endpoint. The place holders are the API Path and
	// Query Parameters.
	credentialsEndpointRelativeURIFormat = "%s?%s"
)

// IAMRoleCredentials is used to save credentials sent by ACS
type IAMRoleCredentials struct {
	CredentialsId   string `json:"-"`
	RoleArn         string `json:"RoleArn"`
	AccessKeyId     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"Token"`
	// Expiration is a string instead of a timestamp. This is to avoid any loss of context
	// while marshalling/unmarshalling this field in the agent. The agent just echo's
	// whatever is sent by the backend.
	Expiration string `json:"Expiration"`
}

// GenerateCredentialsEndpointRelativeURI generates the relative URI for the
// credentials endpoint, for a given task id.
func (roleCredentials *IAMRoleCredentials) GenerateCredentialsEndpointRelativeURI() string {
	params := make(url.Values)
	params[CredentialsIdQueryParameterName] = []string{roleCredentials.CredentialsId}
	return fmt.Sprintf(credentialsEndpointRelativeURIFormat, CredentialsPath, params.Encode())
}

// credentialsManager implements the Manager interface. It is used to
// save credentials sent from ACS and to retrieve credentials from
// the credentials endpoint
type credentialsManager struct {
	idToCredentials map[string]*IAMRoleCredentials
	credentialsLock sync.RWMutex
}

// IAMRoleCredentialsFromACS translates ecsacs.IAMRoleCredentials object to
// api.IAMRoleCredentials
func IAMRoleCredentialsFromACS(roleCredentials *ecsacs.IAMRoleCredentials) IAMRoleCredentials {
	return IAMRoleCredentials{
		CredentialsId:   aws.StringValue(roleCredentials.CredentialsId),
		SessionToken:    aws.StringValue(roleCredentials.SessionToken),
		RoleArn:         aws.StringValue(roleCredentials.RoleArn),
		AccessKeyId:     aws.StringValue(roleCredentials.AccessKeyId),
		SecretAccessKey: aws.StringValue(roleCredentials.SecretAccessKey),
		Expiration:      aws.StringValue(roleCredentials.Expiration),
	}
}

// NewManager creates a new credentials manager object
func NewManager() Manager {
	return &credentialsManager{
		idToCredentials: make(map[string]*IAMRoleCredentials),
	}
}

// SetCredentials adds or updates credentials in the credentials manager
func (manager *credentialsManager) SetCredentials(credentials IAMRoleCredentials) error {
	manager.credentialsLock.Lock()
	defer manager.credentialsLock.Unlock()

	if credentials.CredentialsId == "" {
		return fmt.Errorf("CredentialsId is empty")
	}

	// Check if credentials exists for the given credentials id
	credentialsInMap, ok := manager.idToCredentials[credentials.CredentialsId]
	if !ok {
		// No existing credentials, create a new one
		credentialsInMap = &IAMRoleCredentials{}
	}
	*credentialsInMap = credentials
	manager.idToCredentials[credentials.CredentialsId] = credentialsInMap

	return nil
}

// GetCredentials retrieves credentials for a given credentials id
func (manager *credentialsManager) GetCredentials(id string) (*IAMRoleCredentials, bool) {
	manager.credentialsLock.RLock()
	defer manager.credentialsLock.RUnlock()

	credentials, ok := manager.idToCredentials[id]
	return credentials, ok
}

// RemoveCredentials removes credentials from the credentials manager
func (manager *credentialsManager) RemoveCredentials(id string) {
	manager.credentialsLock.Lock()
	defer manager.credentialsLock.Unlock()

	delete(manager.idToCredentials, id)
}
