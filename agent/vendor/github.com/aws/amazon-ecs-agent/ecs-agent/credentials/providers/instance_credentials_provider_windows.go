//go:build windows
// +build windows

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

package providers

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
)

// The instance credentials chain is the default credentials chain plus the "rotating shared credentials provider",
// so credentials will be checked in this order:
//
//  1. Env vars (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY).
//
//  2. Shared credentials file (https://docs.aws.amazon.com/ses/latest/DeveloperGuide/create-shared-credentials-file.html) (file at ~/.aws/credentials containing access key id and secret access key).
//
//  3. EC2 role credentials. This is an IAM role that the user specifies when they launch their EC2 container instance (ie ecsInstanceRole (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/instance_IAM_role.html)).
//
//  4. Rotating shared credentials file located at /rotatingcreds/credentials
//
//     The default credential chain provided by the SDK includes:
//     * EnvProvider
//     * SharedCredentialsProvider
//     * RemoteCredProvider (EC2RoleProvider)
//
//     In the case of ECS-A on Windows, the `SharedCredentialsProvider` takes
//     precedence over the `RotatingSharedCredentialsProvider` and this results
//     in the credentials not being refreshed. To mitigate this issue, we will
//     reorder the credential chain and ensure that `RotatingSharedCredentialsProvider`
//     takes precedence over the `SharedCredentialsProvider` for ECS-A.
func NewInstanceCredentialsProvider(
	isExternal bool,
	rotatingSharedCreds aws.CredentialsProvider,
	imdsClient ec2rolecreds.GetMetadataAPIClient,
) *InstanceCredentialsProvider {
	var providers []aws.CredentialsProvider

	// If imdsClient is nil, the SDK will default to the EC2 IMDS client.
	// Pass a non-nil imdsClient to stub it out in tests.
	options := func(o *ec2rolecreds.Options) {
		o.Client = imdsClient
	}

	if isExternal {
		providers = []aws.CredentialsProvider{
			envCreds,
			rotatingSharedCreds,
			sharedCreds,
			ec2rolecreds.New(options),
		}
	} else {
		providers = []aws.CredentialsProvider{
			defaultCreds(options),
			rotatingSharedCreds,
		}
	}

	return &InstanceCredentialsProvider{
		providers: providers,
	}
}

var envCreds = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
	cfg, err := config.NewEnvConfig()
	return cfg.Credentials, err
})

var sharedCreds = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
	// Load the env config to get shared config values from env vars (AWS_PROFILE and AWS_SHARED_CREDENTIALS_FILE).
	envCfg, err := config.NewEnvConfig()
	if err != nil {
		return aws.Credentials{}, err
	}

	// If shared config env vars are unset, use the default values.
	if envCfg.SharedConfigProfile == "" {
		envCfg.SharedConfigProfile = config.DefaultSharedConfigProfile
	}
	if envCfg.SharedCredentialsFile == "" {
		envCfg.SharedCredentialsFile = config.DefaultSharedCredentialsFilename()
	}

	cfg, err := config.LoadSharedConfigProfile(ctx, envCfg.SharedConfigProfile, func(option *config.LoadSharedConfigOptions) {
		option.CredentialsFiles = []string{envCfg.SharedCredentialsFile}
	})
	return cfg.Credentials, err
})
