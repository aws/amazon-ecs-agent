// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package dockerauth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/aws-sdk-go/aws"
	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

type EcrAuthProvider struct {
	clientFactory          ecr.ECRFactory
	authorizationDatasLock sync.Mutex
	authorizationDatas     map[string]*ecrapi.AuthorizationData
}

const (
	proxyEndpointScheme = "https://"
	authDataTimeout     = 5 * time.Minute
)

// NewECRAuthProvider returns a DockerAuthProvider that can handle retrieve
// credentials for pulling from Amazon EC2 Container Registry
func NewECRAuthProvider(clientFactory ecr.ECRFactory) EcrAuthProvider {
	return EcrAuthProvider{
		authorizationDatas: make(map[string]*ecrapi.AuthorizationData),
		clientFactory:      clientFactory,
	}
}

// GetAuthconfig retrieves the correct auth configuration for the given repository
func (authProvider *EcrAuthProvider) GetAuthconfig(image string, authData *api.ECRAuthData) (docker.AuthConfiguration, error) {
	if authData == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("EcrAuthProvider cannot be used without AuthData")
	}

	log.Debugf("Calling ECR.GetAuthorizationToken for %s", image)
	registryId := authData.RegistryId

	cachedAuthData := authProvider.authorizationDatas[registryId]
	if cachedAuthData != nil {

		allowTime := time.Now().Add(authDataTimeout)

		if cachedAuthData.ExpiresAt != nil && cachedAuthData.ExpiresAt.After(allowTime) {
			log.Infof("Use cached token ", authData.Region, authData.EndpointOverride)
			return extractToken(cachedAuthData)
		} else {
			authProvider.authorizationDatasLock.Lock()
			delete(authProvider.authorizationDatas, registryId)
			authProvider.authorizationDatasLock.Unlock()
		}
	}

	input := &ecrapi.GetAuthorizationTokenInput{
		RegistryIds: []*string{aws.String(registryId)},
	}

	log.Infof("Getting client in %s with endpoint %s", authData.Region, authData.EndpointOverride)
	client := authProvider.clientFactory.GetClient(authData.Region, authData.EndpointOverride)
	output, err := client.GetAuthorizationToken(input)

	if err != nil {
		return docker.AuthConfiguration{}, err
	}

	if output == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("Missing AuthorizationData in ECR response for %s", image)
	}

	for _, outputAuthData := range output.AuthorizationData {
		if outputAuthData.ProxyEndpoint != nil &&
			strings.HasPrefix(proxyEndpointScheme+image, aws.StringValue(outputAuthData.ProxyEndpoint)) &&
			outputAuthData.AuthorizationToken != nil {

			authProvider.authorizationDatasLock.Lock()
			authProvider.authorizationDatas[registryId] = outputAuthData
			authProvider.authorizationDatasLock.Unlock()
			return extractToken(outputAuthData)
		}
	}
	return docker.AuthConfiguration{}, fmt.Errorf("No AuthorizationToken found for %s", image)
}

func extractToken(authData *ecrapi.AuthorizationData) (docker.AuthConfiguration, error) {
	decodedToken, err := base64.StdEncoding.DecodeString(aws.StringValue(authData.AuthorizationToken))
	if err != nil {
		return docker.AuthConfiguration{}, err
	}
	parts := strings.SplitN(string(decodedToken), ":", 2)
	return docker.AuthConfiguration{
		Username:      parts[0],
		Password:      parts[1],
		ServerAddress: aws.StringValue(authData.ProxyEndpoint),
	}, nil

}
