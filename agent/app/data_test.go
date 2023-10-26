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

package app

import (
	"context"
	"strconv"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/app/factory"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testContainerName = "test-name"
	testImageId       = "test-imageId"
	testMac           = "test-mac"
	testAttachmentArn = "arn:aws:ecs:us-west-2:1234567890:attachment/abc"
	testDockerID      = "test-docker-id"
	testTaskARN       = "arn:aws:ecs:region:account-id:task/task-id"

	testContainerInstanceARN        = "test-ci-arn"
	testCluster                     = "test-cluster"
	testEC2InstanceID               = "test-instance-id"
	testAvailabilityZone            = "test-az"
	testAgentVersion                = "1.40.0"
	testLatestSeqNumberTaskManifest = int64(1)
)

var (
	testContainer = &apicontainer.Container{
		Name:          testContainerName,
		TaskARNUnsafe: testTaskARN,
	}
	testDockerContainer = &apicontainer.DockerContainer{
		DockerID:  testDockerID,
		Container: testContainer,
	}
	testTask = &apitask.Task{
		Arn:        testTaskARN,
		Containers: []*apicontainer.Container{testContainer},
	}

	testImageState = &image.ImageState{
		Image:         testImage,
		PullSucceeded: false,
	}
	testImage = &image.Image{
		ImageID: testImageId,
	}

	testENIAttachment = &ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			AttachmentARN:    testAttachmentArn,
			AttachStatusSent: false,
		},
		MACAddress: testMac,
	}
)

func TestLoadDataNoPreviousState(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		_, stateManagerFactory, _, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	stateManager, dataClient := newTestClient(t)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(cfg *config.Config, options ...statemanager.Option) {
			for _, option := range options {
				option(stateManager)
			}
		}).Return(stateManager, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: factory.NewSaveableOption(),
	}
	state := dockerstate.NewTaskEngineState()
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, err := agent.loadData(eventstream.NewEventStream("events", ctx), credentialsManager,
		state, imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.NoError(t, err)
}

func TestLoadDataLoadFromBoltDB(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		_, stateManagerFactory, _, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	_, dataClient := newTestClient(t)

	// Populate boltdb with test data.
	populateBoltDB(dataClient, t)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: factory.NewSaveableOption(),
	}

	state := dockerstate.NewTaskEngineState()
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()
	s, err := agent.loadData(eventstream.NewEventStream("events", ctx), credentialsManager,
		state, imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.NoError(t, err)
	checkLoadedData(state, s, t)
}

func TestLoadDataLoadFromStateFile(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		_, stateManagerFactory, _, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	stateManager, dataClient := newTestClient(t)

	// Generate a state file with test data.
	generateStateFile(stateManager, t)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(cfg *config.Config, options ...statemanager.Option) {
			for _, option := range options {
				option(stateManager)
			}
		}).Return(stateManager, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: factory.NewSaveableOption(),
	}

	state := dockerstate.NewTaskEngineState()
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()
	s, err := agent.loadData(eventstream.NewEventStream("events", ctx), credentialsManager,
		state, imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.NoError(t, err)
	checkLoadedData(state, s, t)

	// Also verify that the data in the state file has been migrated to boltdb.
	tasks, err := dataClient.GetTasks()
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	containers, err := dataClient.GetContainers()
	require.NoError(t, err)
	assert.Len(t, containers, 1)
	images, err := dataClient.GetImageStates()
	require.NoError(t, err)
	assert.Len(t, images, 1)
	eniAttachments, err := dataClient.GetENIAttachments()
	require.NoError(t, err)
	assert.Len(t, eniAttachments, 1)
}

func checkLoadedData(state dockerstate.TaskEngineState, s *savedData, t *testing.T) {
	_, ok := state.TaskByArn(testTaskARN)
	assert.True(t, ok)
	_, ok = state.ContainerByID(testDockerID)
	assert.True(t, ok)
	assert.Len(t, state.AllImageStates(), 1)
	assert.Len(t, state.AllENIAttachments(), 1)
	assert.Equal(t, testAvailabilityZone, s.availabilityZone)
	assert.Equal(t, testCluster, s.cluster)
	assert.Equal(t, testContainerInstanceARN, s.containerInstanceARN)
	assert.Equal(t, testEC2InstanceID, s.ec2InstanceID)
	assert.Equal(t, testLatestSeqNumberTaskManifest, s.latestTaskManifestSeqNum)
}

func newTestClient(t *testing.T) (statemanager.StateManager, data.Client) {
	testDir := t.TempDir()

	stateManager, err := statemanager.NewStateManager(&config.Config{DataDir: testDir})
	require.NoError(t, err)

	dataClient, err := data.NewWithSetup(testDir)

	t.Cleanup(func() {
		require.NoError(t, dataClient.Close())
	})
	return stateManager, dataClient
}

func generateStateFile(stateManager statemanager.StateManager, t *testing.T) {
	engineState := getTestEngineState()
	statemanager.AddSaveable("TaskEngine", engineState)(stateManager)
	containerInstanceARN := testContainerInstanceARN
	statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceARN)(stateManager)
	cluster := testCluster
	statemanager.AddSaveable("Cluster", &cluster)(stateManager)
	instanceID := testEC2InstanceID
	statemanager.AddSaveable("EC2InstanceID", &instanceID)(stateManager)
	availabilityZone := testAvailabilityZone
	statemanager.AddSaveable("availabilityZone", &availabilityZone)(stateManager)
	latestSeqNumberTaskManifest := testLatestSeqNumberTaskManifest
	statemanager.AddSaveable("latestSeqNumberTaskManifest", &latestSeqNumberTaskManifest)(stateManager)
	require.NoError(t, stateManager.ForceSave())
}

func populateBoltDB(dataClient data.Client, t *testing.T) {
	require.NoError(t, dataClient.SaveTask(testTask))
	require.NoError(t, dataClient.SaveDockerContainer(testDockerContainer))
	require.NoError(t, dataClient.SaveImageState(testImageState))
	require.NoError(t, dataClient.SaveENIAttachment(testENIAttachment))
	require.NoError(t, dataClient.SaveMetadata(data.AgentVersionKey, testAgentVersion))
	require.NoError(t, dataClient.SaveMetadata(data.AvailabilityZoneKey, testAvailabilityZone))
	require.NoError(t, dataClient.SaveMetadata(data.ClusterNameKey, testCluster))
	require.NoError(t, dataClient.SaveMetadata(data.ContainerInstanceARNKey, testContainerInstanceARN))
	require.NoError(t, dataClient.SaveMetadata(data.EC2InstanceIDKey, testEC2InstanceID))
	require.NoError(t, dataClient.SaveMetadata(data.TaskManifestSeqNumKey, strconv.FormatInt(testLatestSeqNumberTaskManifest, 10)))
}

func getTestEngineState() dockerstate.TaskEngineState {
	state := dockerstate.NewTaskEngineState()
	state.AddTask(testTask)
	state.AddContainer(testDockerContainer, testTask)
	state.AddImageState(testImageState)
	state.AddENIAttachment(testENIAttachment)
	return state
}
