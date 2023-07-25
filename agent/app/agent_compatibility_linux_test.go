//go:build linux && unit
// +build linux,unit

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

package app

import (
	"context"
	"testing"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	mockPathExists(false)
}

func TestCompatibilityEnabledSuccess(t *testing.T) {
	ctrl, creds, _, images, _, _, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()
	stateManager := mock_statemanager.NewMockStateManager(ctrl)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.TaskCPUMemLimit = config.BooleanDefaultTrue{Value: config.NotSet}

	agent := &ecsAgent{
		cfg:                   &cfg,
		dataClient:            data.NewNoopClient(),
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: saveableOptionFactory,
		ec2MetadataClient:     ec2.NewBlackholeEC2MetadataClient(),
	}

	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable(gomock.Any(), gomock.Any()).AnyTimes(),
		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(stateManager, nil),
		stateManager.EXPECT().Load().AnyTimes(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	containerChangeEventStream := eventstream.NewEventStream("events", ctx)
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()
	_, _, err := agent.newTaskEngine(containerChangeEventStream, creds, dockerstate.NewTaskEngineState(), images, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)

	assert.NoError(t, err)
	assert.True(t, cfg.TaskCPUMemLimit.Enabled())
}

func TestCompatibilityNotSetFail(t *testing.T) {
	ctrl, creds, _, images, _, _, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()
	stateManager := mock_statemanager.NewMockStateManager(ctrl)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.TaskCPUMemLimit = config.BooleanDefaultTrue{Value: config.NotSet}

	dataClient := newTestDataClient(t)
	populateBoltDB(dataClient, t)

	// Put a bad task in previously saved state.
	for _, task := range getTaskListWithOneBadTask() {
		require.NoError(t, dataClient.SaveTask(task))
	}

	cfg.Cluster = "test-cluster"
	agent := &ecsAgent{
		cfg:                   &cfg,
		dataClient:            dataClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: saveableOptionFactory,
		ec2MetadataClient:     ec2.NewBlackholeEC2MetadataClient(),
	}
	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable(gomock.Any(), gomock.Any()).AnyTimes(),
		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(stateManager, nil),
		stateManager.EXPECT().Load().AnyTimes(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	containerChangeEventStream := eventstream.NewEventStream("events", ctx)
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()
	_, _, err := agent.newTaskEngine(containerChangeEventStream, creds, dockerstate.NewTaskEngineState(), images, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)

	assert.NoError(t, err)
	assert.False(t, cfg.TaskCPUMemLimit.Enabled())
}

func TestCompatibilityExplicitlyEnabledFail(t *testing.T) {
	ctrl, creds, _, images, _, _, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()
	stateManager := mock_statemanager.NewMockStateManager(ctrl)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.TaskCPUMemLimit = config.BooleanDefaultTrue{Value: config.ExplicitlyEnabled}

	dataClient := newTestDataClient(t)
	populateBoltDB(dataClient, t)

	// Put a bad task in previously saved state.
	for _, task := range getTaskListWithOneBadTask() {
		require.NoError(t, dataClient.SaveTask(task))
	}

	agent := &ecsAgent{
		cfg:                   &cfg,
		dataClient:            dataClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: saveableOptionFactory,
		ec2MetadataClient:     ec2.NewBlackholeEC2MetadataClient(),
	}
	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable(gomock.Any(), gomock.Any()).AnyTimes(),
		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(stateManager, nil),
		stateManager.EXPECT().Load().AnyTimes(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	containerChangeEventStream := eventstream.NewEventStream("events", ctx)
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()
	_, _, err := agent.newTaskEngine(containerChangeEventStream, creds, dockerstate.NewTaskEngineState(), images, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)

	assert.Error(t, err)
}

func getTaskListWithOneBadTask() []*apitask.Task {
	badTask := &apitask.Task{
		Arn: testTaskARN,
	}
	return []*apitask.Task{badTask}
}
