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
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	ssmclient "github.com/aws/amazon-ecs-agent/agent/ssm"
	agentversion "github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	roundtripTimeout = 5 * time.Second
)

type SSMClientCreator interface {
	NewSSMClient(region string, creds credentials.IAMRoleCredentials, ipCompatibility ipcompatibility.IPCompatibility) (ssmclient.SSMClient, error)
}

func NewSSMClientCreator() SSMClientCreator {
	return &ssmClientCreator{}
}

type ssmClientCreator struct{}

// SSM Client will automatically retry 3 times when has throttling error
func (*ssmClientCreator) NewSSMClient(region string,
	creds credentials.IAMRoleCredentials,
	ipCompatibility ipcompatibility.IPCompatibility) (ssmclient.SSMClient, error) {

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithHTTPClient(httpclient.New(roundtripTimeout, false, agentversion.String(), config.OSType, config.GetOSFamily())),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			awscreds.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey,
				creds.SessionToken),
		),
	}

	if ipCompatibility.IsIPv6Only() {
		logger.Debug("Configuring SSM Client DualStack endpoint")
		opts = append(opts, awsconfig.WithUseDualStackEndpoint(aws.DualStackEndpointStateEnabled))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(), opts...)

	if err != nil {
		return nil, err
	}

	return ssm.NewFromConfig(cfg), nil
}
