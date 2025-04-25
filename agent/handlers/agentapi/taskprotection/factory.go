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
package taskprotection

import (
	"context"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/version"
	ecsapi "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	ecsclient "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/client"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/httpclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// TaskProtectionClientFactory implements TaskProtectionClientFactoryInterface
type TaskProtectionClientFactory struct {
	Region             string
	Endpoint           string
	AcceptInsecureCert bool
}

// Helper function for retrieving credential from credentials manager and create ecs client
func (factory TaskProtectionClientFactory) NewTaskProtectionClient(
	taskRoleCredential credentials.TaskIAMRoleCredentials,
) (ecsapi.ECSTaskProtectionSDK, error) {
	taskCredential := taskRoleCredential.GetIAMRoleCredentials()
	cfg, err := awsconfig.LoadDefaultConfig(
		context.TODO(),
		awsconfig.WithCredentialsProvider(
			awscreds.NewStaticCredentialsProvider(
				taskCredential.AccessKeyID,
				taskCredential.SecretAccessKey,
				taskCredential.SessionToken,
			),
		),
		awsconfig.WithRegion(factory.Region),
		awsconfig.WithHTTPClient(
			httpclient.New(
				ecsclient.RoundtripTimeout,
				factory.AcceptInsecureCert,
				version.String(),
				config.OSType,
			),
		),
		awsconfig.WithBaseEndpoint(utils.AddScheme(factory.Endpoint)),
	)

	if err != nil {
		return nil, err
	}

	ecsClient := ecs.NewFromConfig(cfg)
	return ecsClient, nil
}
