//go:build unit
// +build unit

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
package factory

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/stretchr/testify/assert"
)

func TestCreateAWSConfig(t *testing.T) {
	testCreds := credentials.IAMRoleCredentials{
		AccessKeyID:     "dummyAccessKeyID",
		SecretAccessKey: "dummySecretAccessKey",
		SessionToken:    "dummySessionToken",
	}

	testRegion := "us-west-2"

	tests := []struct {
		name                   string
		useFIPSEndpoint        bool
		useDualStackEndpoint   bool
		fipsEndpointState      endpoints.FIPSEndpointState
		dualStackEndpointState endpoints.DualStackEndpointState
	}{
		{
			name:                   "Default endpoint configuration",
			useFIPSEndpoint:        false,
			useDualStackEndpoint:   false,
			fipsEndpointState:      endpoints.FIPSEndpointStateUnset,
			dualStackEndpointState: endpoints.DualStackEndpointStateUnset,
		},
		{
			name:                   "FIPS enabled",
			useFIPSEndpoint:        true,
			useDualStackEndpoint:   false,
			fipsEndpointState:      endpoints.FIPSEndpointStateEnabled,
			dualStackEndpointState: endpoints.DualStackEndpointStateUnset,
		},
		{
			name:                   "Dualstack enabled",
			useFIPSEndpoint:        false,
			useDualStackEndpoint:   true,
			fipsEndpointState:      endpoints.FIPSEndpointStateUnset,
			dualStackEndpointState: endpoints.DualStackEndpointStateEnabled,
		},
		{
			name:                   "DualStack and FIPS enabled",
			useFIPSEndpoint:        true,
			useDualStackEndpoint:   true,
			fipsEndpointState:      endpoints.FIPSEndpointStateEnabled,
			dualStackEndpointState: endpoints.DualStackEndpointStateEnabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createAWSConfig(testRegion, testCreds, tt.useFIPSEndpoint, tt.useDualStackEndpoint)

			// Verify common configurations
			assert.Equal(t, roundtripTimeout, cfg.HTTPClient.Timeout)
			assert.Equal(t, testRegion, aws.StringValue(cfg.Region))
			verifyCredentials(t, cfg, testCreds)

			// Verify endpoint configurations
			assert.Equal(t, tt.fipsEndpointState, cfg.UseFIPSEndpoint)
			assert.Equal(t, tt.dualStackEndpointState, cfg.UseDualStackEndpoint)
		})
	}
}

func verifyCredentials(t *testing.T, cfg *aws.Config, expected credentials.IAMRoleCredentials) {
	credsValue, err := cfg.Credentials.Get()
	assert.NoError(t, err)
	assert.Equal(t, expected.AccessKeyID, credsValue.AccessKeyID)
	assert.Equal(t, expected.SecretAccessKey, credsValue.SecretAccessKey)
	assert.Equal(t, expected.SessionToken, credsValue.SessionToken)
}
