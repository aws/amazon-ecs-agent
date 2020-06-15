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

package ecr

import (
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/stretchr/testify/assert"
)

func TestGetClientConfigEndpointOverride(t *testing.T) {
	testAuthData := &apicontainer.ECRAuthData{
		EndpointOverride: "api.ecr.us-west-2.amazonaws.com",
		Region:           "us-west-2",
		UseExecutionRole: false,
	}

	cfg, err := getClientConfig(nil, testAuthData)

	assert.Nil(t, err)
	assert.Equal(t, testAuthData.EndpointOverride, *cfg.Endpoint)
}
