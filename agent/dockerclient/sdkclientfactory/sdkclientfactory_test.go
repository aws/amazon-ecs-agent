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

const expectedEndpoint = "expectedEndpoint"

func TestGetDefaultClientSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedClient := mock_sdkclient.NewMockClient(ctrl)
	newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
		mockClient := mock_sdkclient.NewMockClient(ctrl)
		if version == string(GetDefaultVersion()) {
			mockClient = expectedClient
		}
		mockClient.EXPECT().ServerVersion(gomock.Any()).Return(docker.Version{}, nil).AnyTimes()
		mockClient.EXPECT().Ping(gomock.Any()).AnyTimes()

		return mockClient, nil
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	factory := NewFactory(ctx, expectedEndpoint)
	actualClient, err := factory.GetDefaultClient()
	assert.Nil(t, err)
	assert.Equal(t, expectedClient, actualClient)
}

func TestFindSupportedAPIVersions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dockerVersions := getAgentSupportedDockerVersions()
	allVersions := dockerclient.GetKnownAPIVersions()

	// Set up the mocks and expectations
	mockClients := make(map[string]*mock_sdkclient.MockClient)

	// Ensure that agent pings all known versions of Docker API
	for i := 0; i < len(allVersions); i++ {
		mockClients[string(allVersions[i])] = mock_sdkclient.NewMockClient(ctrl)
		mockClients[string(allVersions[i])].EXPECT().ServerVersion(gomock.Any()).Return(docker.Version{}, nil).AnyTimes()
		mockClients[string(allVersions[i])].EXPECT().Ping(gomock.Any()).AnyTimes()
	}

	// Define the function for the mock client
	// For simplicity, we will pretend all versions of docker are available
	newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
		return mockClients[version], nil
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	factory := NewFactory(ctx, expectedEndpoint)
	actualVersions := factory.FindSupportedAPIVersions()

	assert.Equal(t, len(dockerVersions), len(actualVersions))
	for i := 0; i < len(actualVersions); i++ {
		assert.Equal(t, dockerVersions[i], actualVersions[i])
	}
}

func TestVerifyAgentVersions(t *testing.T) {
	var isKnown = func(v1 dockerclient.DockerVersion) bool {
		for _, v2 := range getAgentSupportedDockerVersions() {
			if v1 == v2 {
				return true
			}
		}
		return false
	}

	// Validate that agentVersions is a subset of allVersions
	for _, agentVersion := range getAgentSupportedDockerVersions() {
		assert.True(t, isKnown(agentVersion))
	}
}

func TestFindSupportedAPIVersionsFromMinAPIVersions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dockerVersions := getAgentSupportedDockerVersions()
	allVersions := dockerclient.GetKnownAPIVersions()

	// Set up the mocks and expectations
	mockClients := make(map[string]*mock_sdkclient.MockClient)

	// Ensure that agent pings all known versions of Docker API
	for i := 0; i < len(allVersions); i++ {
		mockClients[string(allVersions[i])] = mock_sdkclient.NewMockClient(ctrl)
		mockClients[string(allVersions[i])].EXPECT().ServerVersion(gomock.Any()).Return(docker.Version{}, nil).AnyTimes()
		mockClients[string(allVersions[i])].EXPECT().Ping(gomock.Any()).AnyTimes()
	}

	// Define the function for the mock client
	// For simplicity, we will pretend all versions of docker are available
	newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
		return mockClients[version], nil
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	factory := NewFactory(ctx, expectedEndpoint)
	actualVersions := factory.FindSupportedAPIVersions()

	assert.Equal(t, len(dockerVersions), len(actualVersions))
	for i := 0; i < len(actualVersions); i++ {
		assert.Equal(t, dockerVersions[i], actualVersions[i])
	}
}

func TestCompareDockerVersionsWithMinAPIVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	minAPIVersion := "1.12"
	apiVersion := "1.32"
	versions := []string{"1.11", "1.33"}
	rightVersion := "1.25"
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	for _, version := range versions {
		_, err := getDockerClientForVersion("endpoint", version, minAPIVersion, apiVersion, ctx)
		assert.EqualError(t, err, "version detection using MinAPIVersion: unsupported version: "+version)
	}

	mockClients := make(map[string]*mock_sdkclient.MockClient)
	newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
		mockClients[version] = mock_sdkclient.NewMockClient(ctrl)
		mockClients[version].EXPECT().Ping(gomock.Any())
		return mockClients[version], nil
	}
	client, _ := getDockerClientForVersion("endpoint", rightVersion, minAPIVersion, apiVersion, ctx)
	assert.Equal(t, mockClients[rightVersion], client)
}

func TestGetClientCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
		mockClient := mock_sdkclient.NewMockClient(ctrl)
		mockClient.EXPECT().ServerVersion(gomock.Any()).Return(docker.Version{}, nil).AnyTimes()
		mockClient.EXPECT().Ping(gomock.Any()).AnyTimes()
		return mockClient, nil
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	factory := NewFactory(ctx, expectedEndpoint)
	client, err := factory.GetClient(dockerclient.Version_1_17)
	assert.Nil(t, err)

	clientAgain, errAgain := factory.GetClient(dockerclient.Version_1_17)
	assert.Nil(t, errAgain)

	assert.Equal(t, client, clientAgain)
}
