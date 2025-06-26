// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package factory

import (
	"time"

	"context"

	"github.com/aws/amazon-ecs-agent/agent/config"
	fsxclient "github.com/aws/amazon-ecs-agent/agent/fsx"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
)

const (
	roundtripTimeout = 5 * time.Second
)

type FSxClientCreator interface {
	NewFSxClient(region string, creds credentials.IAMRoleCredentials) (fsxclient.FSxClient, error)
}

func NewFSxClientCreator() FSxClientCreator {
	return &fsxClientCreator{}
}

type fsxClientCreator struct{}

func (*fsxClientCreator) NewFSxClient(region string,
	creds credentials.IAMRoleCredentials) (fsxclient.FSxClient, error) {
	// Create credentials provider for SDK v2
	staticCreds := awscreds.NewStaticCredentialsProvider(
		creds.AccessKeyID,
		creds.SecretAccessKey,
		creds.SessionToken,
	)

	cfg, err := awsconfig.LoadDefaultConfig(
		context.TODO(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(staticCreds),
		awsconfig.WithHTTPClient(httpclient.New(roundtripTimeout, false, agentversion.String(), config.OSType, config.GetOSFamily())),
	)
	if err != nil {
		return nil, err
	}

	fsxClient := fsx.NewFromConfig(cfg)

	return fsxClient, nil
}
