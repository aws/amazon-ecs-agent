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
	"context"
	"fmt"
	"net/http"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials/providers"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	ecrservice "github.com/aws/aws-sdk-go-v2/service/ecr"
)

// ECRFactory defines the interface to produce an ECR SDK client
type ECRFactory interface {
	GetClient(*apicontainer.ECRAuthData) (ECRClient, error)
}

type ecrFactory struct {
	httpClient           *http.Client
	useDualStackEndpoint bool
}

const (
	roundtripTimeout = 5 * time.Second
)

// NewECRFactory returns an ECRFactory capable of producing ECRSDK clients
func NewECRFactory(acceptInsecureCert bool, useDualStackEndpoint bool) ECRFactory {
	return &ecrFactory{
		httpClient:           httpclient.New(roundtripTimeout, acceptInsecureCert, agentversion.String(), config.OSType, config.GetOSFamily()),
		useDualStackEndpoint: useDualStackEndpoint,
	}
}

// GetClient creates the ECR SDK client based on the authdata
func (factory *ecrFactory) GetClient(authData *apicontainer.ECRAuthData) (ECRClient, error) {
	clientConfig, err := getClientConfig(factory.httpClient, authData, factory.useDualStackEndpoint)
	if err != nil {
		return &ecrClient{}, err
	}

	return factory.newClient(clientConfig), nil
}

// getClientConfig returns the config for the ecr client based on authData
func getClientConfig(httpClient *http.Client, authData *apicontainer.ECRAuthData, useDualStackEndpoint bool) (*aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(authData.Region),
		awsconfig.WithHTTPClient(httpClient),
	}

	if authData.EndpointOverride != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(utils.AddScheme(authData.EndpointOverride)))
	} else if useDualStackEndpoint {
		logger.Debug("Configuring ECR Client DualStack endpoint")
		opts = append(opts, awsconfig.WithUseDualStackEndpoint(aws.DualStackEndpointStateEnabled))
	}

	var credentialsOpt awsconfig.LoadOptionsFunc
	if authData.UseExecutionRole {
		if authData.GetPullCredentials() == (credentials.IAMRoleCredentials{}) {
			return nil, fmt.Errorf("container uses execution credentials, but the credentials are empty")
		}
		credentialsOpt = awsconfig.WithCredentialsProvider(
			awscreds.NewStaticCredentialsProvider(
				authData.GetPullCredentials().AccessKeyID,
				authData.GetPullCredentials().SecretAccessKey,
				authData.GetPullCredentials().SessionToken,
			),
		)
	} else {
		credentialsOpt = awsconfig.WithCredentialsProvider(
			providers.NewInstanceCredentialsCache(
				false,
				providers.NewRotatingSharedCredentialsProviderV2(),
				nil,
			),
		)
	}
	opts = append(opts, credentialsOpt)

	// Load the config with the options
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(), opts...)

	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (factory *ecrFactory) newClient(cfg *aws.Config) ECRClient {
	ecrClient := ecrservice.NewFromConfig(*cfg)
	return NewECRClient(ecrClient)
}
