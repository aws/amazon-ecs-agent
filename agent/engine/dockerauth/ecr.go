// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/async"
	"github.com/aws/amazon-ecs-agent/agent/ecr"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

type cacheKey struct {
	region           string
	rolearn          string
	registryID       string
	endpointOverride string
}

type ecrAuthProvider struct {
	tokenCache async.Cache
	factory    ecr.ECRFactory
	cacheLock  sync.RWMutex
}

const (
	roundtripTimeout      = 5 * time.Second
	tokenCacheSize        = 100
	MinimumJitterDuration = 30 * time.Minute
	tokenCacheTTL         = 12 * time.Hour
	proxyEndpointScheme   = "https://"
)

func (key *cacheKey) String() string {
	return fmt.Sprintf("%s-%s-%s-%s", key.rolearn, key.region, key.registryID, key.endpointOverride)
}

// NewECRAuthProvider returns a DockerAuthProvider that can handle retrieve
// credentials for pulling from Amazon EC2 Container Registry
func NewECRAuthProvider(ecrFactory ecr.ECRFactory) DockerAuthProvider {
	return &ecrAuthProvider{
		tokenCache: async.NewLRUCache(tokenCacheSize, tokenCacheTTL),
		factory:    ecrFactory,
	}
}

// GetAuthconfig retrieves the correct auth configuration for the given repository
func (authProvider *ecrAuthProvider) GetAuthconfig(image string,
	authData *api.ECRAuthData) (docker.AuthConfiguration, error) {
	if authData == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("Missing AuthConfiguration data")
	}

	// First try to get the token from cache, if the token does not exist,
	// then call ecr api to get the new token
	key := cacheKey{
		region:           authData.Region,
		endpointOverride: authData.EndpointOverride,
		registryID:       authData.RegistryID,
	}
	if !utils.ZeroOrNil(authData.PullCredentials) {
		key.rolearn = authData.PullCredentials.RoleArn
	}

	authProvider.cacheLock.RLock()
	defer authProvider.cacheLock.RUnlock()

	token, ok := authProvider.tokenCache.Get(key.String())
	if ok {
		cachedToken := token.(*ecrapi.AuthorizationData)
		if authProvider.ISTokenValid(cachedToken) {
			return extractToken(cachedToken)
		}
		authProvider.tokenCache.Delete(key.String())
	}

	// Create ECR client to get the token
	client, err := authProvider.factory.GetClient(authData)
	log.Debugf("Calling ECR.GetAuthorizationToken for %s", image)
	ecrAuthData, err := client.GetAuthorizationToken(authData.RegistryID)
	if err != nil {
		return docker.AuthConfiguration{}, err
	}
	if ecrAuthData == nil {
		return docker.AuthConfiguration{}, fmt.Errorf("Missing AuthorizationData in ECR response for %s", image)
	}

	if ecrAuthData.ProxyEndpoint != nil &&
		strings.HasPrefix(proxyEndpointScheme+image, aws.StringValue(ecrAuthData.ProxyEndpoint)) &&
		ecrAuthData.AuthorizationToken != nil {

		// Cache the new token
		authProvider.tokenCache.Set(key.String(), ecrAuthData)
		return extractToken(ecrAuthData)
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

// Ensure token is still within it's expiration window. We early expire to allow
// for timing in calls and add jitter to avoid refreshing all of the tokens at once.
func (authProvider *ecrAuthProvider) ISTokenValid(authData *ecrapi.AuthorizationData) bool {
	if authData == nil || authData.ExpiresAt == nil {
		return false
	}

	refreshTime := aws.TimeValue(authData.ExpiresAt).
		Add(-1 * utils.AddJitter(MinimumJitterDuration, MinimumJitterDuration))

	return time.Now().Before(refreshTime)
}
