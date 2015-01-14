// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package auth

import "errors"
import . "github.com/aws/amazon-ecs-agent/agent/ecs_client/authv4/credentials"

type BasicAWSCredentialProvider struct {
	providers           []AWSCredentialProvider
	lastUsedProviderNdx int
}

func newBasicAWSCredentialProvider() *BasicAWSCredentialProvider {
	provider := new(BasicAWSCredentialProvider)
	provider.lastUsedProviderNdx = -1
	return provider
}

// NewBasicAWSCredentialProvider creates a credential provider chain with
// commonly desired defaults. It pulls credentials from the environment and then
// the instance metadata
// TODO: Add the aws-cli's saved credentials format which reads from a file
func NewBasicAWSCredentialProvider() *BasicAWSCredentialProvider {
	provider := newBasicAWSCredentialProvider()

	provider.AddProvider(NewInstanceMetadataCredentialProvider())
	provider.AddProvider(NewEnvironmentCredentialProvider())

	return provider
}

// AddProvider allows a custom provider to be added. Calling this will add the
// provider as the first in order of preference for usage
func (bcp *BasicAWSCredentialProvider) AddProvider(acp AWSCredentialProvider) {
	// Prepend credentials to prefer later added ones
	bcp.providers = append([]AWSCredentialProvider{acp}, bcp.providers...)
}

// Credentials returns valid AWSCredentials from a provider. It "sticks" to the
// last known good provider for as long as it works and will automatically
// refresh credentials if needed
func (bcp BasicAWSCredentialProvider) Credentials() (*AWSCredentials, error) {
	if bcp.lastUsedProviderNdx >= 0 {
		return bcp.providers[bcp.lastUsedProviderNdx].Credentials()
	}

	for ndx, provider := range bcp.providers {
		credentials, err := provider.Credentials()
		if err == nil {
			bcp.lastUsedProviderNdx = ndx
			return credentials, err
		}
	}

	return nil, errors.New("Unable to find any working credential provider")
}
