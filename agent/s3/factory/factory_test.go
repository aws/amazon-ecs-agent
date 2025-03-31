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
	"context"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	accessKeyId     = "dummyAccessKeyID"
	secretAccessKey = "dummySecretAccessKey"
	sessionToken    = "dummySessionToken"
)

var (
	creds = credentials.IAMRoleCredentials{
		AccessKeyID:     accessKeyId,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}
)

func TestCreateAWSConfig(t *testing.T) {
	tests := []struct {
		name                    string
		region                  string
		useFIPSEndpoint         bool
		useDualStackEndpoint    bool
		dualStackEndpointState  aws.DualStackEndpointState
		expectedUseFIPSEndpoint aws.FIPSEndpointState
		expectedUsePathStyle    bool
	}{
		{
			name:                    "Default endpoint configuration",
			region:                  "us-west-2",
			useFIPSEndpoint:         false,
			useDualStackEndpoint:    false,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateUnset,
			dualStackEndpointState:  aws.DualStackEndpointStateUnset,
			expectedUsePathStyle:    false,
		},
		{
			name:                    "FIPS enabled only",
			region:                  "us-gov-west-1",
			useFIPSEndpoint:         true,
			useDualStackEndpoint:    false,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateEnabled,
			dualStackEndpointState:  aws.DualStackEndpointStateUnset,
			expectedUsePathStyle:    false,
		},
		{
			name:                    "Dualstack enabled only",
			region:                  "us-west-2",
			useFIPSEndpoint:         false,
			useDualStackEndpoint:    true,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateUnset,
			dualStackEndpointState:  aws.DualStackEndpointStateEnabled,
			expectedUsePathStyle:    false,
		},
		{
			name:                    "DualStack and FIPS enabled",
			region:                  "us-gov-west-1",
			useFIPSEndpoint:         true,
			useDualStackEndpoint:    true,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateEnabled,
			dualStackEndpointState:  aws.DualStackEndpointStateEnabled,
			expectedUsePathStyle:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := createAWSConfig(tt.region, creds, tt.useFIPSEndpoint, tt.useDualStackEndpoint)
			require.NoError(t, err)

			// Creating a new S3 client off of the AWS config object
			client := s3.NewFromConfig(cfg)
			// Obtain a copy of the options of the S3 client
			clientOpts := client.Options()

			assert.Equal(t, tt.region, clientOpts.Region)
			verifyCredentials(t, clientOpts, creds)

			// Verify endpoint configurations
			assert.Equal(t, tt.expectedUseFIPSEndpoint, clientOpts.EndpointOptions.GetUseFIPSEndpoint())
			assert.Equal(t, tt.expectedUsePathStyle, clientOpts.UsePathStyle)
			assert.Equal(t, tt.dualStackEndpointState, clientOpts.EndpointOptions.GetUseDualStackEndpoint())
		})
	}
}

func verifyCredentials(t *testing.T, clientOpts s3.Options, expected credentials.IAMRoleCredentials) {
	credsValue, err := clientOpts.Credentials.Retrieve(context.TODO())
	assert.NoError(t, err)
	assert.Equal(t, expected.AccessKeyID, credsValue.AccessKeyID)
	assert.Equal(t, expected.SecretAccessKey, credsValue.SecretAccessKey)
	assert.Equal(t, expected.SessionToken, credsValue.SessionToken)
}
