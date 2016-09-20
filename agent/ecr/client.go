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

package ecr

import (
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/async"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	log "github.com/cihub/seelog"
)

const (
	MinimumJitterDuration = 30 * time.Minute
	MaximumJitterDuration = 1 * time.Hour
)

// ECRClient wrapper interface for mocking
type ECRClient interface {
	GetAuthorizationToken(registryId string) (*ecrapi.AuthorizationData, error)
	IsTokenValid(*ecrapi.AuthorizationData) bool
}

// ECRSDK is an interface that specifies the subset of the AWS Go SDK's ECR
// client that the Agent uses.  This interface is meant to allow injecting a
// mock for testing.
type ECRSDK interface {
	GetAuthorizationToken(*ecrapi.GetAuthorizationTokenInput) (*ecrapi.GetAuthorizationTokenOutput, error)
}

type ecrClient struct {
	sdkClient  ECRSDK
	tokenCache async.Cache
}

func NewECRClient(sdkClient ECRSDK, tokenCache async.Cache) ECRClient {
	return &ecrClient{
		sdkClient:  sdkClient,
		tokenCache: tokenCache,
	}
}

func (client *ecrClient) GetAuthorizationToken(registryId string) (*ecrapi.AuthorizationData, error) {
	cachedToken, found := client.tokenCache.Get(registryId)
	if found {
		cachedAuthData := cachedToken.(*ecrapi.AuthorizationData)

		if client.IsTokenValid(cachedAuthData) {
			return cachedAuthData, nil
		} else {
			log.Debugf("Token found, but expires at %s", aws.TimeValue(cachedAuthData.ExpiresAt))
		}
	}

	log.Debugf("Calling GetAuthorizationToken for %q", registryId)

	output, err := client.sdkClient.GetAuthorizationToken(&ecrapi.GetAuthorizationTokenInput{
		RegistryIds: []*string{aws.String(registryId)},
	})

	if err != nil {
		return nil, err
	}

	if len(output.AuthorizationData) != 1 {
		return nil, fmt.Errorf("Unexpected number of results in AuthorizationData (%d)", len(output.AuthorizationData))
	}
	authData := output.AuthorizationData[0]
	client.tokenCache.Set(registryId, authData)

	return authData, nil
}

// Ensure token is still within it's expiration window. We early expire to allow for timing in calls and add jitter to avoid
// refreshing all of the tokens at once.
func (client *ecrClient) IsTokenValid(authData *ecrapi.AuthorizationData) bool {
	if authData == nil || authData.ExpiresAt == nil {
		return false
	}

	refreshTime := aws.TimeValue(authData.ExpiresAt).Add(-1 * client.expirationJitter())
	return time.Now().Before(refreshTime)
}

func (client *ecrClient) expirationJitter() time.Duration {
	return utils.AddJitter(MinimumJitterDuration, MaximumJitterDuration)
}
