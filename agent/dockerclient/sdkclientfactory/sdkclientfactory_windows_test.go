//go:build unit && windows
// +build unit,windows

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

package sdkclientfactory

import (
	"context"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient"
	mock_sdkclient "github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient/mocks"
	docker "github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGetClientMinimumVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedClient := mock_sdkclient.NewMockClient(ctrl)

	newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
		mockClient := mock_sdkclient.NewMockClient(ctrl)
		if version == string(minDockerAPIVersion) {
			mockClient = expectedClient
		}
		mockClient.EXPECT().ServerVersion(gomock.Any()).Return(docker.Version{}, nil).AnyTimes()
		mockClient.EXPECT().Ping(gomock.Any()).AnyTimes()
		return mockClient, nil
	}

	// Ensure a call to GetClient with a version below the minDockerAPIVersion
	// is replaced by the minDockerAPIVersion
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	factory := NewFactory(ctx, expectedEndpoint)
	actualClient, err := factory.GetClient(dockerclient.Version_1_19)

	assert.NoError(t, err)
	assert.Equal(t, expectedClient, actualClient)
}

func TestFindClientAPIVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	ctrl := gomock.NewController(t)
	mockClient := mock_sdkclient.NewMockClient(ctrl)
	factory := NewFactory(ctx, expectedEndpoint)

	for _, version := range getAgentSupportedDockerVersions() {
		if isWindowsReplaceableVersion(version) {
			version = minDockerAPIVersion
		}
		mockClient.EXPECT().ClientVersion().Return(string(version))
		assert.Equal(t, version, factory.FindClientAPIVersion(mockClient))
	}
}

func isWindowsReplaceableVersion(version dockerclient.DockerVersion) bool {
	for _, v := range getWindowsReplaceableVersions() {
		if v == version {
			return true
		}
	}
	return false
}
