// +build !integration,!windows
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package dockerclient

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGetClientCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		mockClient := mock_dockeriface.NewMockClient(ctrl)
		mockClient.EXPECT().Ping()
		return mockClient, nil
	}

	factory := NewFactory(expectedEndpoint)
	client, err := factory.GetClient(Version_1_18)
	assert.Nil(t, err)

	clientAgain, errAgain := factory.GetClient(Version_1_18)
	assert.Nil(t, errAgain)

	assert.Equal(t, client, clientAgain)
}
