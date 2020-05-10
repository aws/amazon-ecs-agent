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

package container

import (
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/credentials"

	"github.com/docker/docker/api/types"
)

// RegistryAuthenticationData is the authentication data sent by the ECS backend.  Currently, the only supported
// authentication data is for ECR.
type RegistryAuthenticationData struct {
	Type        string       `json:"type"`
	ECRAuthData *ECRAuthData `json:"ecrAuthData"`
	ASMAuthData *ASMAuthData `json:"asmAuthData"`
}

// ECRAuthData is the authentication details for ECR specifying the region, registryID, and possible endpoint override
type ECRAuthData struct {
	EndpointOverride string `json:"endpointOverride"`
	Region           string `json:"region"`
	RegistryID       string `json:"registryId"`
	UseExecutionRole bool   `json:"useExecutionRole"`
	pullCredentials  credentials.IAMRoleCredentials
	dockerAuthConfig types.AuthConfig
	lock             sync.RWMutex
}

// ASMAuthData is the authentication data required for Docker private registry auth
type ASMAuthData struct {
	// CredentialsParameter is set by ACS and specifies the name of the
	// parameter to retrieve from ASM
	CredentialsParameter string `json:"credentialsParameter"`
	// Region is set by ACS and specifies the region to fetch the
	// secret from
	Region string `json:"region"`
	// dockerAuthConfig gets populated during the ASM resource creation
	// by the task engine
	dockerAuthConfig types.AuthConfig
	lock             sync.RWMutex
}

// GetPullCredentials returns the pull credentials in the auth
func (auth *ECRAuthData) GetPullCredentials() credentials.IAMRoleCredentials {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.pullCredentials
}

// SetPullCredentials sets the credentials to pull from ECR in the auth
func (auth *ECRAuthData) SetPullCredentials(creds credentials.IAMRoleCredentials) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.pullCredentials = creds
}

// GetDockerAuthConfig returns the pull credentials in the auth
func (auth *ECRAuthData) GetDockerAuthConfig() types.AuthConfig {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.dockerAuthConfig
}

// SetDockerAuthConfig sets the credentials to pull from ECR in the
// ecr auth data
func (auth *ECRAuthData) SetDockerAuthConfig(dac types.AuthConfig) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.dockerAuthConfig = dac
}

// GetDockerAuthConfig returns the pull credentials in the auth
func (auth *ASMAuthData) GetDockerAuthConfig() types.AuthConfig {
	auth.lock.RLock()
	defer auth.lock.RUnlock()

	return auth.dockerAuthConfig
}

// SetDockerAuthConfig sets the credentials to pull from ECR in the
// auth
func (auth *ASMAuthData) SetDockerAuthConfig(dac types.AuthConfig) {
	auth.lock.Lock()
	defer auth.lock.Unlock()

	auth.dockerAuthConfig = dac
}
