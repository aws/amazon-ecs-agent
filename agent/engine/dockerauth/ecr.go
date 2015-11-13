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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/aws-sdk-go/aws"
	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

type ecrAuthProvider struct {
	authData *api.ECRAuthData
	client   ecr.ECRSDK
}

const proxyEndpointScheme = "https://"

// NewECRAuthProvider returns a DockerAuthProvider that can handle retrieve
// credentials for pulling from Amazon EC2 Container Registry
func NewECRAuthProvider(authData *api.ECRAuthData, clientFactory ecr.ECRFactory) DockerAuthProvider {
	if authData == nil {
		return &ecrAuthProvider{}
	}
	log.Tracef("Getting client in %s with endpoint %s", authData.Region, authData.EndpointOverride)
	return &ecrAuthProvider{
		authData: authData,
		client:   clientFactory.GetClient(authData.Region, authData.EndpointOverride),
	}
}

// GetAuthconfig retrieves the correct auth configuration for the given repository
func (authProvider *ecrAuthProvider) GetAuthconfig(image string) (docker.AuthConfiguration, error) {
	if authProvider.authData == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("ecrAuthProvider cannot be used without AuthData")
	}
	log.Debugf("Calling ECR.GetAuthorizationToken for %s", image)
	input := &ecrapi.GetAuthorizationTokenInput{
		RegistryIds: []*string{aws.String(authProvider.authData.RegistryId)},
	}
	output, err := authProvider.client.GetAuthorizationToken(input)
	if err != nil {
		return docker.AuthConfiguration{}, err
	}
	if output == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("Missing AuthorizationData in ECR response for %s", image)
	}
	for _, authData := range output.AuthorizationData {
		if authData.ProxyEndpoint != nil &&
			strings.HasPrefix(proxyEndpointScheme+image, aws.StringValue(authData.ProxyEndpoint)) &&
			authData.AuthorizationToken != nil {
			return extractToken(authData)
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
