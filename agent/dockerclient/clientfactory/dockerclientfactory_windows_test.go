// +build unit,windows

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package clientfactory

import (
	"context"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockeriface"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockeriface/mocks"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGetClientMinimumVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedClient := mock_dockeriface.NewMockClient(ctrl)

	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		mockClient := mock_dockeriface.NewMockClient(ctrl)
		if version == string(minDockerAPIVersion) {
			mockClient = expectedClient
		}
		mockClient.EXPECT().VersionWithContext(gomock.Any()).Return(&docker.Env{}, nil).AnyTimes()
		mockClient.EXPECT().Ping().AnyTimes()
		return mockClient, nil
	}

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
	factory := NewFactory(ctx, expectedEndpoint)

	for _, version := range getAgentVersions() {
		client, err := factory.GetClient(version)
		assert.NoError(t, err)
		if isWindowsReplaceableVersion(version) {
			version = minDockerAPIVersion
		}
		assert.Equal(t, version, factory.FindClientAPIVersion(client))
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
