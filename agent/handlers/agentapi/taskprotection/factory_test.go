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

	"github.com/aws/amazon-ecs-agent/ecs-agent/api"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/golang/mock/gomock"
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

// TestGetECSClientHappyCase tests newTaskProtectionClient uses credential in credentials manager and
// returns an ECS client with correct status code and error
func TestGetECSClientHappyCase(t *testing.T) {
	testIAMRoleCredentials := credentials.TaskIAMRoleCredentials{
		IAMRoleCredentials: credentials.IAMRoleCredentials{
			AccessKeyID:     testAccessKey,
			SecretAccessKey: testSecretKey,
			SessionToken:    testSessionToken,
		},
	}

	factory := TaskProtectionClientFactory{
		Region: testRegion, Endpoint: testECSEndpoint, AcceptInsecureCert: testAcceptInsecureCert,
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ret := factory.NewTaskProtectionClient(testIAMRoleCredentials)
	_, ok := ret.(api.ECSTaskProtectionSDK)

	// Assert response
	assert.True(t, ok)
}
