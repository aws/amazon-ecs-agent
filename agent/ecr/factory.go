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

// Package ecr helps generate clients to talk to the ECR API
package ecr

import (
	"fmt"
	"net/http"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/credentials/instancecreds"
	ecrapi "github.com/aws/amazon-ecs-agent/agent/ecr/model/ecr"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

// ECRFactory defines the interface to produce an ECR SDK client
type ECRFactory interface {
	GetClient(*apicontainer.ECRAuthData) (ECRClient, error)
}

type ecrFactory struct {
	httpClient *http.Client
}

const (
	roundtripTimeout = 5 * time.Second
)

// NewECRFactory returns an ECRFactory capable of producing ECRSDK clients
func NewECRFactory(acceptInsecureCert bool) ECRFactory {
	return &ecrFactory{
		httpClient: httpclient.New(roundtripTimeout, acceptInsecureCert),
	}
}

// GetClient creates the ECR SDK client based on the authdata
func (factory *ecrFactory) GetClient(authData *apicontainer.ECRAuthData) (ECRClient, error) {
	clientConfig, err := getClientConfig(factory.httpClient, authData)
	if err != nil {
		return &ecrClient{}, err
	}

	return factory.newClient(clientConfig), nil
}

// getClientConfig returns the config for the ecr client based on authData
func getClientConfig(httpClient *http.Client, authData *apicontainer.ECRAuthData) (*aws.Config, error) {
	cfg := aws.NewConfig().WithRegion(authData.Region).WithHTTPClient(httpClient)
	if authData.EndpointOverride != "" {
		cfg.Endpoint = aws.String(authData.EndpointOverride)
	}

	if authData.UseExecutionRole {
		if authData.GetPullCredentials() == (credentials.IAMRoleCredentials{}) {
			return nil, fmt.Errorf("container uses execution credentials, but the credentials are empty")
		}
		creds := awscreds.NewStaticCredentials(authData.GetPullCredentials().AccessKeyID,
			authData.GetPullCredentials().SecretAccessKey,
			authData.GetPullCredentials().SessionToken)
		cfg = cfg.WithCredentials(creds)
	} else {
		cfg = cfg.WithCredentials(instancecreds.GetCredentials())
	}

	return cfg, nil
}

func (factory *ecrFactory) newClient(cfg *aws.Config) ECRClient {
	sdkClient := ecrapi.New(session.New(cfg))
	return NewECRClient(sdkClient)
}
