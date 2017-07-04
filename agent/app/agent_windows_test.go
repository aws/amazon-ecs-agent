// +build windows

// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package app

import (
	"context"
	"sync"
	"testing"

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
)

// TestDoStartHappyPath tests the doStart method for windows. This method should
// go away when we support metrics for windows containers
func TestDoStartHappyPath(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, _, _ := setup(t)
	defer ctrl.Finish()

	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)

	var discoverEndpointsInvoked sync.WaitGroup
	discoverEndpointsInvoked.Add(1)
	containerChangeEvents := make(chan engine.DockerContainerChangeEvent)

	dockerClient.EXPECT().Version().AnyTimes()
	imageManager.EXPECT().StartImageCleanupProcess(gomock.Any()).MaxTimes(1)
	mockCredentialsProvider.EXPECT().IsExpired().Return(false).AnyTimes()

	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(credentials.Value{}, nil),
		dockerClient.EXPECT().SupportedVersions().Return(nil),
		dockerClient.EXPECT().KnownVersions().Return(nil),
		client.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).Return("arn", nil),
		imageManager.EXPECT().SetSaver(gomock.Any()),
		dockerClient.EXPECT().ContainerEvents(gomock.Any()).Return(containerChangeEvents, nil),
		state.EXPECT().AllImageStates().Return(nil),
		state.EXPECT().AllTasks().Return(nil),
		client.EXPECT().DiscoverPollEndpoint(gomock.Any()).Do(func(x interface{}) {
			// Ensures that the test waits until acs session has bee started
			discoverEndpointsInvoked.Done()
		}).Return("poll-endpoint", nil),
		client.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return("acs-endpoint", nil).AnyTimes(),
	)

	cfg := config.DefaultConfig()
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                &cfg,
		credentialProvider: credentials.NewCredentials(mockCredentialsProvider),
		dockerClient:       dockerClient,
	}

	go agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client)

	// Wait for both DiscoverPollEndpointInput and DiscoverTelemetryEndpoint to be
	// invoked. These are used as proxies to indicate that acs and tcs handlers'
	// NewSession call has been invoked
	discoverEndpointsInvoked.Wait()
}
