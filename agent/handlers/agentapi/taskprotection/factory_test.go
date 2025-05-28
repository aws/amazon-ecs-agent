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
package taskprotection

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecsservice "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/stretchr/testify/assert"
)

const (
	testAccessKey          = "accessKey"
	testSecretKey          = "secretKey"
	testSessionToken       = "sessionToken"
	testRegion             = "region"
	testECSEndpoint        = "endpoint"
	testAcceptInsecureCert = false
)

var testIAMRoleCredentials = credentials.TaskIAMRoleCredentials{
	IAMRoleCredentials: credentials.IAMRoleCredentials{
		AccessKeyID:     testAccessKey,
		SecretAccessKey: testSecretKey,
		SessionToken:    testSessionToken,
	},
}

type endpointConfig struct {
	baseEndpoint         string
	useDualStackEndpoint aws.DualStackEndpointState
}

func TestTaskProtectionClientFactoryHappyCase(t *testing.T) {
	testCases := []struct {
		name                   string
		factory                TaskProtectionClientFactory
		expectedEndpointConfig endpointConfig
	}{
		{
			name:                   "IPV4 Only",
			factory:                TaskProtectionClientFactory{testRegion, "", testAcceptInsecureCert, ipcompatibility.NewIPv4OnlyCompatibility()},
			expectedEndpointConfig: endpointConfig{"", aws.DualStackEndpointStateUnset},
		},
		{
			name:                   "IPV6 Only",
			factory:                TaskProtectionClientFactory{testRegion, "", testAcceptInsecureCert, ipcompatibility.NewIPv6OnlyCompatibility()},
			expectedEndpointConfig: endpointConfig{"", aws.DualStackEndpointStateEnabled},
		},
		{
			name:                   "Dualstack",
			factory:                TaskProtectionClientFactory{testRegion, "", testAcceptInsecureCert, ipcompatibility.NewIPCompatibility(true, true)},
			expectedEndpointConfig: endpointConfig{"", aws.DualStackEndpointStateUnset},
		},
		{
			name:                   "Custom Endpoint Only",
			factory:                TaskProtectionClientFactory{testRegion, testECSEndpoint, testAcceptInsecureCert, ipcompatibility.NewIPv4OnlyCompatibility()},
			expectedEndpointConfig: endpointConfig{"https://" + testECSEndpoint, aws.DualStackEndpointStateUnset},
		},
		{
			name:                   "Custom Endpoint + IPV6 Only",
			factory:                TaskProtectionClientFactory{testRegion, testECSEndpoint, testAcceptInsecureCert, ipcompatibility.NewIPv6OnlyCompatibility()},
			expectedEndpointConfig: endpointConfig{"https://" + testECSEndpoint, aws.DualStackEndpointStateUnset},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := tc.factory.NewTaskProtectionClient(testIAMRoleCredentials)
			assert.NoError(t, err)

			// Type check
			_, ok := client.(ecs.ECSTaskProtectionSDK)
			assert.True(t, ok)

			ecsClientOptions := client.(*ecsservice.Client).Options()
			if tc.expectedEndpointConfig.baseEndpoint != "" {
				clientEndpoint := ecsClientOptions.BaseEndpoint
				assert.NotNil(t, clientEndpoint)

				assert.Equal(t, "https://"+testECSEndpoint, *clientEndpoint)
			}

			assert.Equal(t, tc.expectedEndpointConfig.useDualStackEndpoint, ecsClientOptions.EndpointOptions.UseDualStackEndpoint)
		})
	}
}
