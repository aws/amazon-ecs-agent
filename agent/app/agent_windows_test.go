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
	"time"

	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	statemanager_mocks "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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

	// These calls are expected to happen, but cannot be ordered as they are
	// invoked via go routines, which will lead to occasional test failues
	dockerClient.EXPECT().Version().AnyTimes()
	imageManager.EXPECT().StartImageCleanupProcess(gomock.Any()).MaxTimes(1)
	mockCredentialsProvider.EXPECT().IsExpired().Return(false).AnyTimes()
	client.EXPECT().DiscoverPollEndpoint(gomock.Any()).Do(func(x interface{}) {
		// Ensures that the test waits until acs session has bee started
		discoverEndpointsInvoked.Done()
	}).Return("poll-endpoint", nil)
	client.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return("acs-endpoint", nil).AnyTimes()

	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(credentials.Value{}, nil),
		dockerClient.EXPECT().SupportedVersions().Return(nil),
		dockerClient.EXPECT().KnownVersions().Return(nil),
		client.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any()).Return("arn", nil),
		imageManager.EXPECT().SetSaver(gomock.Any()),
		dockerClient.EXPECT().ContainerEvents(gomock.Any()).Return(containerChangeEvents, nil),
		state.EXPECT().AllImageStates().Return(nil),
		state.EXPECT().AllTasks().Return(nil),
	)

	cfg := getTestConfig()
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                &cfg,
		credentialProvider: credentials.NewCredentials(mockCredentialsProvider),
		dockerClient:       dockerClient,
		terminationHandler: func(saver statemanager.Saver, taskEngine engine.TaskEngine) {},
	}

	go agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client)

	// Wait for both DiscoverPollEndpointInput and DiscoverTelemetryEndpoint to be
	// invoked. These are used as proxies to indicate that acs and tcs handlers'
	// NewSession call has been invoked
	discoverEndpointsInvoked.Wait()
}

type mockAgent struct {
	startFunc          func() int
	terminationHandler sighandlers.TerminationHandler
}

func (m *mockAgent) start() int {
	return m.startFunc()
}
func (m *mockAgent) setTerminationHandler(handler sighandlers.TerminationHandler) {
	m.terminationHandler = handler
}
func (m *mockAgent) printVersion() int        { return 0 }
func (m *mockAgent) printECSAttributes() int  { return 0 }
func (m *mockAgent) startWindowsService() int { return 0 }

func TestHandler_RunAgent_StartExitImmediately(t *testing.T) {
	// register some mocks, but nothing should get called on any of them
	ctrl := gomock.NewController(t)
	_ = statemanager_mocks.NewMockStateManager(ctrl)
	_ = engine.NewMockTaskEngine(ctrl)
	defer ctrl.Finish()

	wg := sync.WaitGroup{}
	wg.Add(1)
	startFunc := func() int {
		// startFunc doesn't block, nothing is called
		wg.Done()
		return 0
	}
	agent := &mockAgent{startFunc: startFunc}
	handler := &handler{agent}
	go handler.runAgent(context.TODO())
	wg.Wait()
	assert.NotNil(t, agent.terminationHandler)
}

func TestHandler_RunAgent_NoSaveWithNoTerminationHandler(t *testing.T) {
	// register some mocks, but nothing should get called on any of them
	ctrl := gomock.NewController(t)
	_ = statemanager_mocks.NewMockStateManager(ctrl)
	_ = engine.NewMockTaskEngine(ctrl)
	defer ctrl.Finish()

	done := make(chan struct{})
	startFunc := func() int {
		<-done // block until after the test ends so that we can test that runAgent returns when cancelled
		return 0
	}
	agent := &mockAgent{startFunc: startFunc}
	handler := &handler{agent}
	ctx, cancel := context.WithCancel(context.TODO())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		handler.runAgent(ctx)
		wg.Done()
	}()
	cancel()
	wg.Wait()
	assert.NotNil(t, agent.terminationHandler)
}

func TestHandler_RunAgent_ForceSaveWithTerminationHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	stateManager := statemanager_mocks.NewMockStateManager(ctrl)
	taskEngine := engine.NewMockTaskEngine(ctrl)
	defer ctrl.Finish()

	taskEngine.EXPECT().Disable()
	stateManager.EXPECT().ForceSave()

	agent := &mockAgent{}

	done := make(chan struct{})
	defer func() { done <- struct{}{} }()
	startFunc := func() int {
		go agent.terminationHandler(stateManager, taskEngine)
		<-done // block until after the test ends so that we can test that runAgent returns when cancelled
		return 0
	}
	agent.startFunc = startFunc
	handler := &handler{agent}
	ctx, cancel := context.WithCancel(context.TODO())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		handler.runAgent(ctx)
		wg.Done()
	}()
	time.Sleep(time.Second) // give startFunc enough time to actually call the termination handler
	cancel()
	wg.Wait()
}
