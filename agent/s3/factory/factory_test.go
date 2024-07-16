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

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/stretchr/testify/assert"
)

func TestCreateAWSConfig(t *testing.T) {
	creds := credentials.IAMRoleCredentials{
		AccessKeyID:     "dummyAccessKeyID",
		SecretAccessKey: "dummySecretAccessKey",
		SessionToken:    "dummySessionToken",
	}
	region := "us-west-2"
	// Test without FIPS enabled
	config.SetFIPSEnabled(false)
	cfg := createAWSConfig(region, creds, false)
	assert.Equal(t, roundtripTimeout, cfg.HTTPClient.Timeout, "HTTPClient timeout should be set")
	assert.Equal(t, region, aws.StringValue(cfg.Region), "Region should be set")
	credsValue, err := cfg.Credentials.Get()
	assert.NoError(t, err)
	assert.Equal(t, "dummyAccessKeyID", credsValue.AccessKeyID, "AccessKeyID should be set")
	assert.Equal(t, "dummySecretAccessKey", credsValue.SecretAccessKey, "SecretAccessKey should be set")
	assert.Equal(t, "dummySessionToken", credsValue.SessionToken, "SessionToken should be set")
	assert.Equal(t, endpoints.FIPSEndpointStateUnset, cfg.UseFIPSEndpoint, "UseFIPSEndpoint should not be set")
	assert.Nil(t, cfg.S3ForcePathStyle, "S3ForcePathStyle should not be set")
	// Test with FIPS enabled in a non-FIPS compliant region
	config.SetFIPSEnabled(true)
	cfg = createAWSConfig(region, creds, false)
	assert.Equal(t, roundtripTimeout, cfg.HTTPClient.Timeout, "HTTPClient timeout should be set")
	assert.Equal(t, region, aws.StringValue(cfg.Region), "Region should be set")
	credsValue, err = cfg.Credentials.Get()
	assert.NoError(t, err)
	assert.Equal(t, "dummyAccessKeyID", credsValue.AccessKeyID, "AccessKeyID should be set")
	assert.Equal(t, "dummySecretAccessKey", credsValue.SecretAccessKey, "SecretAccessKey should be set")
	assert.Equal(t, "dummySessionToken", credsValue.SessionToken, "SessionToken should be set")
	assert.Equal(t, endpoints.FIPSEndpointStateUnset, cfg.UseFIPSEndpoint, "UseFIPSEndpoint should not be set")
	assert.Nil(t, cfg.S3ForcePathStyle, "S3ForcePathStyle should not be set")
	// Test with FIPS enabled in a FIPS compliant region
	fipsRegion := "us-gov-west-1"
	cfg = createAWSConfig(fipsRegion, creds, true)
	assert.Equal(t, roundtripTimeout, cfg.HTTPClient.Timeout, "HTTPClient timeout should be set")
	assert.Equal(t, fipsRegion, aws.StringValue(cfg.Region), "Region should be set")
	credsValue, err = cfg.Credentials.Get()
	assert.NoError(t, err)
	assert.Equal(t, "dummyAccessKeyID", credsValue.AccessKeyID, "AccessKeyID should be set")
	assert.Equal(t, "dummySecretAccessKey", credsValue.SecretAccessKey, "SecretAccessKey should be set")
	assert.Equal(t, "dummySessionToken", credsValue.SessionToken, "SessionToken should be set")
	assert.Equal(t, endpoints.FIPSEndpointStateEnabled, cfg.UseFIPSEndpoint, "UseFIPSEndpoint should be set to FIPSEndpointStateEnabled")
	assert.False(t, aws.BoolValue(cfg.S3ForcePathStyle), "S3ForcePathStyle should be set to false")
}
