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
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

const (
	ALTERNATE_CREDENTIAL_PROFILE_ENV_VAR = "ECS_ALTERNATE_CREDENTIAL_PROFILE"
	DEFAULT_CREDENTIAL_PROFILE           = "default"
	// defaultRotationInterval is how frequently to expire and re-retrieve the credentials from file.
	defaultRotationInterval = time.Minute
	// RotatingSharedCredentialsProviderName is the name of this provider
	RotatingSharedCredentialsProviderName = "RotatingSharedCredentialsProvider"
)

// RotatingSharedCredentialsProviderV2 is a provider that retrieves credentials from the
// shared credentials file and adds the functionality of expiring and re-retrieving
// those credentials from the file.
// TODO (@tiffwang): Remove V2 suffix after the credentials package is
// fully migrated to aws-sdk-go-v2.
type RotatingSharedCredentialsProviderV2 struct {
	RotationInterval time.Duration
	profile          string
	file             string
}

// NewRotatingSharedCredentials returns a rotating shared credentials provider
// with default values set.
func NewRotatingSharedCredentialsProviderV2() *RotatingSharedCredentialsProviderV2 {
	var credentialProfile = DEFAULT_CREDENTIAL_PROFILE
	if alternateCredentialProfile := os.Getenv(ALTERNATE_CREDENTIAL_PROFILE_ENV_VAR); alternateCredentialProfile != "" {
		logger.Info(fmt.Sprintf("Overriding %s credential profile; using: %s.", DEFAULT_CREDENTIAL_PROFILE, alternateCredentialProfile))
		credentialProfile = alternateCredentialProfile
	}

	return &RotatingSharedCredentialsProviderV2{
		RotationInterval: defaultRotationInterval,
		profile:          credentialProfile,
		file:             defaultRotatingCredentialsFilename,
	}
}

// Retrieve will use the given filename and profile and retrieve AWS credentials.
func (p *RotatingSharedCredentialsProviderV2) Retrieve(ctx context.Context) (aws.Credentials, error) {
	sharedConfig, err := config.LoadSharedConfigProfile(ctx, p.profile, func(option *config.LoadSharedConfigOptions) {
		option.CredentialsFiles = []string{p.file}
	})
	credentials := sharedConfig.Credentials
	credentials.Source = RotatingSharedCredentialsProviderName
	if err != nil {
		return credentials, err
	}

	credentials.CanExpire = true
	credentials.Expires = time.Now().Add(p.RotationInterval)
	logger.Info(fmt.Sprintf("Successfully got instance credentials from file %s. %s",
		p.file, credentialsToString(credentials)))
	return credentials, nil
}

func credentialsToString(credentials aws.Credentials) string {
	akid := ""
	// only print last 4 chars if it's less than half the full AKID
	if len(credentials.AccessKeyID) > 8 {
		akid = credentials.AccessKeyID[len(credentials.AccessKeyID)-4:]
	}
	return fmt.Sprintf("Provider: %s. Access Key ID XXXX%s", credentials.Source, akid)
}
