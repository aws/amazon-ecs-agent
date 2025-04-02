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

	"github.com/aws/amazon-ecs-agent/agent/config"
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

func TestCreateAWSConfigForS3Client(t *testing.T) {
	tcs := []struct {
		name                    string
		region                  string
		useFIPSEndpoint         bool
		expectedUseFIPSEndpoint aws.FIPSEndpointState
		expectedUsePathStyle    bool
	}{
		{
			name:                    "config without FIPS enabled",
			region:                  "us-west-2",
			useFIPSEndpoint:         false,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateUnset,
			expectedUsePathStyle:    false,
		},
		{
			name:                    "config with FIPS enabled in a non-FIPS compliant region",
			region:                  "us-west-2",
			useFIPSEndpoint:         true,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateEnabled,
			expectedUsePathStyle:    false,
		},
		{
			name:                    "config with FIPS enabled",
			region:                  "us-gov-west-1",
			useFIPSEndpoint:         true,
			expectedUseFIPSEndpoint: aws.FIPSEndpointStateEnabled,
			expectedUsePathStyle:    false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			config.SetFIPSEnabled(tc.useFIPSEndpoint)

			// Creating the AWS Config object
			cfg, err := createAWSConfig(tc.region, creds, tc.useFIPSEndpoint)
			require.NoError(t, err)
			assert.Equal(t, tc.region, cfg.Region, "Region should be set")

			// Creating a new S3 client off of the AWS config object
			client := s3.NewFromConfig(cfg)
			// Obtain a copy of the options of the S3 client
			clientOpts := client.Options()

			credsValue, err := clientOpts.Credentials.Retrieve(context.TODO())
			require.NoError(t, err)
			assert.Equal(t, accessKeyId, credsValue.AccessKeyID, "AccessKeyID should be set")
			assert.Equal(t, secretAccessKey, credsValue.SecretAccessKey, "SecretAccessKey should be set")
			assert.Equal(t, sessionToken, credsValue.SessionToken, "SessionToken should be set")
			assert.Equal(t, tc.expectedUseFIPSEndpoint, clientOpts.EndpointOptions.GetUseFIPSEndpoint())
			assert.Equal(t, tc.expectedUsePathStyle, clientOpts.UsePathStyle)
		})
	}
}
