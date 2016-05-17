// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface/mocks"
	"github.com/golang/mock/gomock"
)

func TestGetDefaultClientSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_dockeriface.NewMockClient(ctrl)
	mockClient.EXPECT().Ping()

	expectedEndpoint := "expectedEndpoint"

	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		if endpoint != expectedEndpoint {
			t.Errorf("Expected endpoint %s but was %s", expectedEndpoint, endpoint)
		}
		if version != string(defaultVersion) {
			t.Errorf("Expected version %s but was %s", defaultVersion, version)
		}
		return mockClient, nil
	}

	factory := NewFactory(expectedEndpoint)
	client, err := factory.GetDefaultClient()
	if err != nil {
		t.Fatal("err should be nil")
	}
	if client != mockClient {
		t.Error("Client returned by GetDefaultClient differs from mockClient")
	}
}

func TestGetClientCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_dockeriface.NewMockClient(ctrl)
	mockClient.EXPECT().Ping()

	expectedEndpoint := "expectedEndpoint"

	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		if endpoint != expectedEndpoint {
			t.Errorf("Expected endpoint %s but was %s", expectedEndpoint, endpoint)
		}
		if version != string(Version_1_18) {
			t.Errorf("Expected version %s but was %s", Version_1_18, version)
		}
		return mockClient, nil
	}

	factory := NewFactory(expectedEndpoint)
	client, err := factory.GetClient(Version_1_18)
	if err != nil {
		t.Fatal("err should be nil")
	}
	if client != mockClient {
		t.Error("Client returned by GetDefaultClient differs from mockClient")
	}

	client, err = factory.GetClient(Version_1_18)
	if err != nil {
		t.Fatal("err should be nil")
	}
	if client != mockClient {
		t.Error("Client returned by GetDefaultClient differs from mockClient")
	}
}

func TestGetClientFailCreateNotCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_dockeriface.NewMockClient(ctrl)

	calledOnce := false
	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		if calledOnce {
			return mockClient, nil
		}
		calledOnce = true
		return mockClient, fmt.Errorf("Test error!")
	}

	factory := NewFactory("")
	client, err := factory.GetClient(Version_1_19)
	if err == nil {
		t.Fatal("err should not be nil")
	}
	if client != nil {
		t.Error("client should be nil")
	}

	mockClient.EXPECT().Ping()

	client, err = factory.GetClient(Version_1_19)
	if err != nil {
		t.Fatal("err should be nil")
	}
	if client != mockClient {
		t.Error("Client returned by GetDefaultClient differs from mockClient")
	}
}

func TestGetClientFailPingNotCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mock_dockeriface.NewMockClient(ctrl)

	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		return mockClient, nil
	}

	mockClient.EXPECT().Ping().Return(fmt.Errorf("Test error!"))

	factory := NewFactory("")
	client, err := factory.GetClient(Version_1_20)
	if err == nil {
		t.Fatal("err should not be nil")
	}
	if client != nil {
		t.Error("client should be nil")
	}

	mockClient.EXPECT().Ping()

	client, err = factory.GetClient(Version_1_20)
	if err != nil {
		t.Fatal("err should be nil")
	}
	if client != mockClient {
		t.Error("Client returned by GetDefaultClient differs from mockClient")
	}
}

func TestFindAvailableVersiosn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient117 := mock_dockeriface.NewMockClient(ctrl)
	mockClient118 := mock_dockeriface.NewMockClient(ctrl)
	mockClient119 := mock_dockeriface.NewMockClient(ctrl)
	mockClient120 := mock_dockeriface.NewMockClient(ctrl)
	mockClient121 := mock_dockeriface.NewMockClient(ctrl)
	mockClient122 := mock_dockeriface.NewMockClient(ctrl)

	expectedEndpoint := "expectedEndpoint"

	newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
		if endpoint != expectedEndpoint {
			t.Errorf("Expected endpoint %s but was %s", expectedEndpoint, endpoint)
		}

		switch DockerVersion(version) {
		case Version_1_17:
			return mockClient117, nil
		case Version_1_18:
			return mockClient118, nil
		case Version_1_19:
			return mockClient119, nil
		case Version_1_20:
			return mockClient120, nil
		case Version_1_21:
			return mockClient121, nil
		case Version_1_22:
			return mockClient122, nil
		default:
			t.Fatal("Unrecognized version")
		}
		return nil, fmt.Errorf("This should not happen, update the test")
	}

	mockClient117.EXPECT().Ping()
	mockClient118.EXPECT().Ping().Return(fmt.Errorf("Test error!"))
	mockClient119.EXPECT().Ping()
	mockClient120.EXPECT().Ping()
	mockClient121.EXPECT().Ping()
	mockClient122.EXPECT().Ping()

	expectedVersions := []DockerVersion{Version_1_17, Version_1_19, Version_1_20, Version_1_21, Version_1_22}

	factory := NewFactory(expectedEndpoint)
	versions := factory.FindAvailableVersions()
	if len(versions) != len(expectedVersions) {
		t.Errorf("Expected %d versions but got %d", len(expectedVersions), len(versions))
	}
	for i := 0; i < len(versions); i++ {
		if versions[i] != expectedVersions[i] {
			t.Errorf("Expected version %s but got version %s", expectedVersions[i], versions[i])
		}
	}
}
