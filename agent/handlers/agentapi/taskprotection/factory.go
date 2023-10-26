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
	"github.com/aws/amazon-ecs-agent/agent/api/ecsclient"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
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
) api.ECSTaskProtectionSDK {
	taskCredential := taskRoleCredential.GetIAMRoleCredentials()
	cfg := aws.NewConfig().
		WithCredentials(awscreds.NewStaticCredentials(taskCredential.AccessKeyID,
			taskCredential.SecretAccessKey,
			taskCredential.SessionToken)).
		WithRegion(factory.Region).
		WithHTTPClient(httpclient.New(ecsclient.RoundtripTimeout, factory.AcceptInsecureCert)).
		WithEndpoint(factory.Endpoint)

	ecsClient := ecs.New(session.Must(session.NewSession()), cfg)
	return ecsClient
}
