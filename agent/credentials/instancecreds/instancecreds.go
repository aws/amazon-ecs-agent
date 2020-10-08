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

package instancecreds

import (
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/credentials/providers"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/cihub/seelog"
)

var (
	credentialChain *credentials.Credentials
	mu              sync.Mutex
)

// GetCredentials returns the instance credentials chain. This is the default chain
// credentials plus the "rotating shared credentials provider", so credentials will
// be checked in this order:
//    1. Env vars (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY).
//    2. Shared credentials file (https://docs.aws.amazon.com/ses/latest/DeveloperGuide/create-shared-credentials-file.html) (file at ~/.aws/credentials containing access key id and secret access key).
//    3. EC2 role credentials. This is an IAM role that the user specifies when they launch their EC2 container instance (ie ecsInstanceRole (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/instance_IAM_role.html)).
//    4. Rotating shared credentials file located at /rotatingcreds/credentials
func GetCredentials() *credentials.Credentials {
	mu.Lock()
	if credentialChain == nil {
		credProviders := defaults.CredProviders(defaults.Config(), defaults.Handlers())
		credProviders = append(credProviders, providers.NewRotatingSharedCredentialsProvider())
		credentialChain = credentials.NewCredentials(&credentials.ChainProvider{
			VerboseErrors: false,
			Providers:     credProviders,
		})
	}
	mu.Unlock()

	// credentials.Credentials is concurrency-safe, so lock not needed here
	v, err := credentialChain.Get()
	if err != nil {
		seelog.Errorf("Error getting ECS instance credentials from default chain: %s", err)
	} else {
		seelog.Infof("Successfully got ECS instance credentials from provider: %s", v.ProviderName)
	}
	return credentialChain
}
