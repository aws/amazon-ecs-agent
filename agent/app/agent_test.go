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
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	mock_factory "github.com/aws/amazon-ecs-agent/agent/app/factory/mocks"
	app_mocks "github.com/aws/amazon-ecs-agent/agent/app/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_containermetadata "github.com/aws/amazon-ecs-agent/agent/containermetadata/mocks"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_ec2 "github.com/aws/amazon-ecs-agent/agent/ec2/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/daemonmanager"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_execcmdagent "github.com/aws/amazon-ecs-agent/agent/engine/execcmd/mocks"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	mock_serviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect/mock"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	mock_statemanager "github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	mock_loader "github.com/aws/amazon-ecs-agent/agent/utils/loader/mocks"
	mock_mobypkgwrapper "github.com/aws/amazon-ecs-agent/agent/utils/mobypkgwrapper/mocks"
	"github.com/aws/amazon-ecs-agent/agent/version"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	clusterName                      = "some-cluster"
	containerInstanceARN             = "container-instance1"
	availabilityZone                 = "us-west-2b"
	hostPrivateIPv4Address           = "127.0.0.1"
	hostPublicIPv4Address            = "127.0.0.1"
	instanceID                       = "i-123"
	warmedState                      = "Warmed:Running"
	testTargetLifecycleMaxRetryCount = 1
)

var notFoundErr = awserr.NewRequestFailure(awserr.Error(awserr.New("NotFound", "", errors.New(""))), 404, "")
var badReqErr = awserr.NewRequestFailure(awserr.Error(awserr.New("BadRequest", "", errors.New(""))), 400, "")
var serverErr = awserr.NewRequestFailure(awserr.Error(awserr.New("InternalServerError", "", errors.New(""))), 500, "")
var apiVersions = []dockerclient.DockerVersion{
	dockerclient.Version_1_21,
	dockerclient.Version_1_22,
	dockerclient.Version_1_23}
var capabilities []*ecs.Attribute
var testHostCPU = int64(1024)
var testHostMEMORY = int64(1024)
var testHostResource = map[string]*ecs.Resource{
	"CPU": &ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &testHostCPU,
	},
	"MEMORY": &ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &testHostMEMORY,
	},
}

func setup(t *testing.T) (*gomock.Controller,
	*mock_credentials.MockManager,
	*mock_dockerstate.MockTaskEngineState,
	*mock_engine.MockImageManager,
	*mock_api.MockECSClient,
	*mock_dockerapi.MockDockerClient,
	*mock_factory.MockStateManager,
	*mock_factory.MockSaveableOption,
	*mock_execcmdagent.MockManager,
	*mock_serviceconnect.MockManager) {

	ctrl := gomock.NewController(t)

	return ctrl,
		mock_credentials.NewMockManager(ctrl),
		mock_dockerstate.NewMockTaskEngineState(ctrl),
		mock_engine.NewMockImageManager(ctrl),
		mock_api.NewMockECSClient(ctrl),
		mock_dockerapi.NewMockDockerClient(ctrl),
		mock_factory.NewMockStateManager(ctrl),
		mock_factory.NewMockSaveableOption(ctrl),
		mock_execcmdagent.NewMockManager(ctrl),
		mock_serviceconnect.NewMockManager(ctrl)
}

func TestDoStartMinimumSupportedDockerVersionTerminal(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()

	oldAPIVersions := []dockerclient.DockerVersion{
		dockerclient.Version_1_18,
		dockerclient.Version_1_19,
		dockerclient.Version_1_20}
	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(oldAPIVersions),
	)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          dockerClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: saveableOptionFactory,
	}
	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client, execCmdMgr)
	assert.Equal(t, exitcodes.ExitTerminal, exitCode)
}

func TestDoStartMinimumSupportedDockerVersionError(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()

	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return([]dockerclient.DockerVersion{}),
	)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          dockerClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: saveableOptionFactory,
	}

	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client, execCmdMgr)
	assert.Equal(t, exitcodes.ExitError, exitCode)
}

func TestDoStartNewTaskEngineError(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, client,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(apiVersions),
		client.EXPECT().GetHostResources().Return(testHostResource, nil),
		saveableOptionFactory.EXPECT().AddSaveable("TaskEngine", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("ContainerInstanceArn", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("Cluster", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("EC2InstanceID", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("availabilityZone", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("latestSeqNumberTaskManifest", gomock.Any()).Return(nil),

		// An error in creating the state manager should result in an
		// error from newTaskEngine as well
		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil, errors.New("error")),
	)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            data.NewNoopClient(),
		dockerClient:          dockerClient,
		ec2MetadataClient:     ec2MetadataClient,
		stateManagerFactory:   stateManagerFactory,
		saveableOptionFactory: saveableOptionFactory,
	}

	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, dockerstate.NewTaskEngineState(), imageManager, client, execCmdMgr)
	assert.Equal(t, exitcodes.ExitTerminal, exitCode)
}

func TestDoStartRegisterContainerInstanceErrorTerminal(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, _, _, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()

	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockEC2Metadata.EXPECT().PrimaryENIMAC().Return("mac", nil)
	mockEC2Metadata.EXPECT().VPCID(gomock.Eq("mac")).Return("vpc-id", nil)
	mockEC2Metadata.EXPECT().SubnetID(gomock.Eq("mac")).Return("subnet-id", nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(apiVersions),
		client.EXPECT().GetHostResources().Return(testHostResource, nil),
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		dockerClient.EXPECT().SupportedVersions().Return(nil),
		dockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{""}, nil),
		dockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return("", "", apierrors.NewAttributeError("error")),
	)

	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                &cfg,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		dockerClient:       dockerClient,
		mobyPlugins:        mockMobyPlugins,
		ec2MetadataClient:  mockEC2Metadata,
		terminationHandler: func(taskEngineState dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
		},
		serviceconnectManager: mockServiceConnectManager,
	}

	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client, execCmdMgr)
	assert.Equal(t, exitcodes.ExitTerminal, exitCode)
}

func TestDoStartRegisterContainerInstanceErrorNonTerminal(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, _, _, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockEC2Metadata.EXPECT().PrimaryENIMAC().Return("mac", nil)
	mockEC2Metadata.EXPECT().VPCID(gomock.Eq("mac")).Return("vpc-id", nil)
	mockEC2Metadata.EXPECT().SubnetID(gomock.Eq("mac")).Return("subnet-id", nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(apiVersions),
		client.EXPECT().GetHostResources().Return(testHostResource, nil),
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		dockerClient.EXPECT().SupportedVersions().Return(nil),
		dockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{""}, nil),
		dockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return("", "", errors.New("error")),
	)

	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                &cfg,
		dockerClient:       dockerClient,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
		ec2MetadataClient:  mockEC2Metadata,
		terminationHandler: func(taskEngineState dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
		},
		serviceconnectManager: mockServiceConnectManager,
	}

	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client, execCmdMgr)
	assert.Equal(t, exitcodes.ExitError, exitCode)
}

func TestDoStartWarmPoolsError(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, _, _, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(apiVersions),
		client.EXPECT().GetHostResources().Return(testHostResource, nil),
	)

	cfg := getTestConfig()
	cfg.WarmPoolsSupport = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	terminationHandlerChan := make(chan bool)
	terminationHandlerInvoked := false
	agent := &ecsAgent{
		ctx:               ctx,
		cfg:               &cfg,
		dockerClient:      dockerClient,
		ec2MetadataClient: mockEC2Metadata,
		terminationHandler: func(taskEngineState dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
			terminationHandlerChan <- true
		},
	}

	err := errors.New("error")
	mockEC2Metadata.EXPECT().TargetLifecycleState().Return("", err).Times(targetLifecycleMaxRetryCount)

	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client, execCmdMgr)

	select {
	case terminationHandlerInvoked = <-terminationHandlerChan:
	case <-time.After(10 * time.Second):
	}
	assert.Equal(t, exitcodes.ExitTerminal, exitCode)
	// verify that termination handler had been started before pollling
	assert.True(t, terminationHandlerInvoked)
}

func TestDoStartHappyPath(t *testing.T) {
	testDoStartHappyPathWithConditions(t, false, false, false)
}

func TestDoStartWarmPoolsEnabled(t *testing.T) {
	testDoStartHappyPathWithConditions(t, false, true, false)
}

func TestDoStartWarmPoolsBlackholed(t *testing.T) {
	testDoStartHappyPathWithConditions(t, true, true, false)
}

func TestDoStartHappyPathExternal(t *testing.T) {
	testDoStartHappyPathWithConditions(t, false, false, true)
}

func testDoStartHappyPathWithConditions(t *testing.T, blackholed bool, warmPoolsEnv bool, isExternalLaunchType bool) {
	ctrl, credentialsManager, _, imageManager, client,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()

	saveableOptionFactory.EXPECT().AddSaveable(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any()).
		Return(statemanager.NewNoopStateManager(), nil).AnyTimes()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2MetadataClient.EXPECT().PrivateIPv4Address().Return(hostPrivateIPv4Address, nil)
	ec2MetadataClient.EXPECT().PublicIPv4Address().Return(hostPublicIPv4Address, nil)
	ec2MetadataClient.EXPECT().OutpostARN().Return("", nil)

	if !isExternalLaunchType {
		// VPC and Subnet should not be initizalied for external launch type
		ec2MetadataClient.EXPECT().PrimaryENIMAC().Return("mac", nil)
		ec2MetadataClient.EXPECT().VPCID(gomock.Eq("mac")).Return("vpc-id", nil)
		ec2MetadataClient.EXPECT().SubnetID(gomock.Eq("mac")).Return("subnet-id", nil)
	}

	if blackholed {
		if warmPoolsEnv {
			ec2MetadataClient.EXPECT().TargetLifecycleState().Return("", errors.New("blackholed")).Times(targetLifecycleMaxRetryCount)
		}
		ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("blackholed"))
	} else {
		if warmPoolsEnv {
			ec2MetadataClient.EXPECT().TargetLifecycleState().Return("", errors.New("error"))
			ec2MetadataClient.EXPECT().TargetLifecycleState().Return(inServiceState, nil)
		}
		ec2MetadataClient.EXPECT().InstanceID().Return(instanceID, nil)
	}

	var discoverEndpointsInvoked sync.WaitGroup
	discoverEndpointsInvoked.Add(2)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	dockerClient.EXPECT().Version(gomock.Any(), gomock.Any()).AnyTimes()
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	containermetadata := mock_containermetadata.NewMockManager(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	mockServiceConnectManager.EXPECT().GetAppnetContainerTarballDir().AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedImageName().Return("service_connect_agent:v1").AnyTimes()
	imageManager.EXPECT().AddImageToCleanUpExclusionList(gomock.Eq("service_connect_agent:v1")).Times(1)
	imageManager.EXPECT().StartImageCleanupProcess(gomock.Any()).MaxTimes(1)
	dockerClient.EXPECT().ListContainers(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		dockerapi.ListContainersResponse{}).AnyTimes()
	client.EXPECT().DiscoverPollEndpoint(gomock.Any()).Do(func(x interface{}) {
		// Ensures that the test waits until acs session has bee started
		discoverEndpointsInvoked.Done()
	}).Return("poll-endpoint", nil)
	client.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return("acs-endpoint", nil).AnyTimes()
	client.EXPECT().DiscoverTelemetryEndpoint(gomock.Any()).Do(func(x interface{}) {
		// Ensures that the test waits until telemetry session has bee started
		discoverEndpointsInvoked.Done()
	}).Return("telemetry-endpoint", nil)
	client.EXPECT().DiscoverTelemetryEndpoint(gomock.Any()).Return(
		"tele-endpoint", nil).AnyTimes()

	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(apiVersions),
		client.EXPECT().GetHostResources().Return(testHostResource, nil),
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		dockerClient.EXPECT().SupportedVersions().Return(nil),
		dockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{""}, nil),
		dockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).Return(containerInstanceARN, availabilityZone, nil),
		containermetadata.EXPECT().SetContainerInstanceARN(containerInstanceARN),
		containermetadata.EXPECT().SetAvailabilityZone(availabilityZone),
		containermetadata.EXPECT().SetHostPrivateIPv4Address(hostPrivateIPv4Address),
		containermetadata.EXPECT().SetHostPublicIPv4Address(hostPublicIPv4Address),
		imageManager.EXPECT().SetDataClient(gomock.Any()),
		dockerClient.EXPECT().ContainerEvents(gomock.Any()),
	)

	cfg := getTestConfig()
	cfg.ContainerMetadataEnabled = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	if warmPoolsEnv {
		cfg.WarmPoolsSupport = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	}
	if isExternalLaunchType {
		cfg.External = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	}
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())

	dataClient := newTestDataClient(t)

	// Cancel the context to cancel async routines
	agent := &ecsAgent{
		ctx:                ctx,
		cfg:                &cfg,
		dockerClient:       dockerClient,
		dataClient:         dataClient,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:        mockMobyPlugins,
		metadataManager:    containermetadata,
		terminationHandler: func(taskEngineState dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
		},
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		saveableOptionFactory: saveableOptionFactory,
		serviceconnectManager: mockServiceConnectManager,
	}

	var agentW sync.WaitGroup
	agentW.Add(1)
	go func() {
		agent.doStart(eventstream.NewEventStream("events", ctx),
			credentialsManager, dockerstate.NewTaskEngineState(), imageManager, client, execCmdMgr)
		agentW.Done()
	}()

	discoverEndpointsInvoked.Wait()
	cancel()
	agentW.Wait()

	assertMetadata(t, data.AgentVersionKey, version.Version, dataClient)
	assertMetadata(t, data.AvailabilityZoneKey, availabilityZone, dataClient)
	assertMetadata(t, data.ClusterNameKey, clusterName, dataClient)
	assertMetadata(t, data.ContainerInstanceARNKey, containerInstanceARN, dataClient)
	if !blackholed {
		assertMetadata(t, data.EC2InstanceIDKey, instanceID, dataClient)
	}
}

func assertMetadata(t *testing.T, key, expectedVal string, dataClient data.Client) {
	val, err := dataClient.GetMetadata(key)
	require.NoError(t, err)
	assert.Equal(t, expectedVal, val)
}

func TestNewTaskEngineRestoreFromCheckpointNoEC2InstanceIDToLoadHappyPath(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	expectedInstanceID := "inst-1"
	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable("TaskEngine", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("ContainerInstanceArn", gomock.Any()).Do(
			func(name string, saveable statemanager.Saveable) {
				previousContainerInstanceARN, ok := saveable.(*string)
				assert.True(t, ok)
				*previousContainerInstanceARN = "prev-container-inst"
			}).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("Cluster", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("EC2InstanceID", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("availabilityZone", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("latestSeqNumberTaskManifest", gomock.Any()).Return(nil),

		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			statemanager.NewNoopStateManager(), nil),
		ec2MetadataClient.EXPECT().InstanceID().Return(expectedInstanceID, nil),
	)

	dataClient := newTestDataClient(t)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		dockerClient:          dockerClient,
		pauseLoader:           mockPauseLoader,
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		saveableOptionFactory: saveableOptionFactory,
	}

	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, instanceID, err := agent.newTaskEngine(eventstream.NewEventStream("events", ctx),
		credentialsManager, dockerstate.NewTaskEngineState(), imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstanceID, instanceID)
	assert.Equal(t, "prev-container-inst", agent.containerInstanceARN)
}

func TestNewTaskEngineRestoreFromCheckpointPreviousEC2InstanceIDLoadedHappyPath(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	expectedInstanceID := "inst-1"

	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable("TaskEngine", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("ContainerInstanceArn", gomock.Any()).Do(
			func(name string, saveable statemanager.Saveable) {
				previousContainerInstanceARN, ok := saveable.(*string)
				assert.True(t, ok)
				*previousContainerInstanceARN = "prev-container-inst"
			}).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("Cluster", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("EC2InstanceID", gomock.Any()).Do(
			func(name string, saveable statemanager.Saveable) {
				previousEC2InstanceID, ok := saveable.(*string)
				assert.True(t, ok)
				*previousEC2InstanceID = "inst-2"
			}).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("availabilityZone", gomock.Any()).Do(
			func(name string, saveable statemanager.Saveable) {
				previousAZ, ok := saveable.(*string)
				assert.True(t, ok)
				*previousAZ = "us-west-2b"
			}).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("latestSeqNumberTaskManifest", gomock.Any()).Return(nil),
		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			statemanager.NewNoopStateManager(), nil),
		ec2MetadataClient.EXPECT().InstanceID().Return(expectedInstanceID, nil),
	)

	dataClient := newTestDataClient(t)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		dockerClient:          dockerClient,
		pauseLoader:           mockPauseLoader,
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		saveableOptionFactory: saveableOptionFactory,
	}
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, instanceID, err := agent.newTaskEngine(eventstream.NewEventStream("events", ctx),
		credentialsManager, dockerstate.NewTaskEngineState(), imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.NoError(t, err)
	assert.Equal(t, expectedInstanceID, instanceID)
	assert.NotEqual(t, "prev-container-inst", agent.containerInstanceARN)
	assert.NotEqual(t, "us-west-2b", agent.availabilityZone)
}

func TestNewTaskEngineRestoreFromCheckpointClusterIDMismatch(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.Cluster = "default"
	ec2InstanceID := "inst-1"

	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable("TaskEngine", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("ContainerInstanceArn", gomock.Any()).Do(
			func(name string, saveable statemanager.Saveable) {
				previousContainerInstanceARN, ok := saveable.(*string)
				assert.True(t, ok)
				*previousContainerInstanceARN = ec2InstanceID
			}).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("Cluster", gomock.Any()).Do(
			func(name string, saveable statemanager.Saveable) {
				previousCluster, ok := saveable.(*string)
				assert.True(t, ok)
				*previousCluster = clusterName
			}).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("EC2InstanceID", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("availabilityZone", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("latestSeqNumberTaskManifest", gomock.Any()).Return(nil),

		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			statemanager.NewNoopStateManager(), nil),
		ec2MetadataClient.EXPECT().InstanceID().Return(ec2InstanceID, nil),
	)

	dataClient := newTestDataClient(t)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		dockerClient:          dockerClient,
		pauseLoader:           mockPauseLoader,
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		saveableOptionFactory: saveableOptionFactory,
	}

	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, _, err := agent.newTaskEngine(eventstream.NewEventStream("events", ctx),
		credentialsManager, dockerstate.NewTaskEngineState(), imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.Error(t, err)
	assert.IsType(t, clusterMismatchError{}, err)
}

func TestNewTaskEngineRestoreFromCheckpointNewStateManagerError(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)
	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable("TaskEngine", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("ContainerInstanceArn", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("Cluster", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("EC2InstanceID", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("availabilityZone", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("latestSeqNumberTaskManifest", gomock.Any()).Return(nil),

		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil, errors.New("error")),
	)

	dataClient := newTestDataClient(t)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		dockerClient:          dockerClient,
		pauseLoader:           mockPauseLoader,
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		saveableOptionFactory: saveableOptionFactory,
	}

	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, _, err := agent.newTaskEngine(eventstream.NewEventStream("events", ctx),
		credentialsManager, dockerstate.NewTaskEngineState(), imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.Error(t, err)
	assert.False(t, isTransient(err))
}

func TestNewTaskEngineRestoreFromCheckpointStateLoadError(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	stateManager := mock_statemanager.NewMockStateManager(ctrl)
	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	gomock.InOrder(
		saveableOptionFactory.EXPECT().AddSaveable("TaskEngine", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("ContainerInstanceArn", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("Cluster", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("EC2InstanceID", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("availabilityZone", gomock.Any()).Return(nil),
		saveableOptionFactory.EXPECT().AddSaveable("latestSeqNumberTaskManifest", gomock.Any()).Return(nil),
		stateManagerFactory.EXPECT().NewStateManager(gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(stateManager, nil),
		stateManager.EXPECT().Load().Return(errors.New("error")),
	)

	dataClient := newTestDataClient(t)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		dockerClient:          dockerClient,
		pauseLoader:           mockPauseLoader,
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		saveableOptionFactory: saveableOptionFactory,
	}

	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, _, err := agent.newTaskEngine(eventstream.NewEventStream("events", ctx),
		credentialsManager, dockerstate.NewTaskEngineState(), imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.Error(t, err)
	assert.False(t, isTransient(err))
}

func TestNewTaskEngineRestoreFromCheckpoint(t *testing.T) {
	ctrl, credentialsManager, _, imageManager, _,
		dockerClient, stateManagerFactory, saveableOptionFactory, execCmdMgr, serviceConnectManager := setup(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	cfg := getTestConfig()
	cfg.Checkpoint = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.Cluster = testCluster
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	ec2MetadataClient.EXPECT().InstanceID().Return(testEC2InstanceID, nil)

	dataClient := newTestDataClient(t)

	// Populate boldtb with test data.
	populateBoltDB(dataClient, t)

	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dataClient:            dataClient,
		dockerClient:          dockerClient,
		stateManagerFactory:   stateManagerFactory,
		ec2MetadataClient:     ec2MetadataClient,
		pauseLoader:           mockPauseLoader,
		saveableOptionFactory: saveableOptionFactory,
	}

	state := dockerstate.NewTaskEngineState()
	hostResources := getTestHostResources()
	daemonManagers := getTestDaemonManagers()

	_, instanceID, err := agent.newTaskEngine(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, hostResources, execCmdMgr, serviceConnectManager, daemonManagers)
	assert.NoError(t, err)
	assert.Equal(t, testEC2InstanceID, instanceID)

	require.NotNil(t, agent.latestSeqNumberTaskManifest)
	s := &savedData{
		availabilityZone:         agent.availabilityZone,
		cluster:                  cfg.Cluster,
		containerInstanceARN:     agent.containerInstanceARN,
		ec2InstanceID:            instanceID,
		latestTaskManifestSeqNum: *agent.latestSeqNumberTaskManifest,
	}
	checkLoadedData(state, s, t)
}

func TestSetClusterInConfigMismatch(t *testing.T) {
	clusterNamesInConfig := []string{"", "foo"}
	for _, clusterNameInConfig := range clusterNamesInConfig {
		t.Run(fmt.Sprintf("cluster in config is '%s'", clusterNameInConfig), func(t *testing.T) {
			cfg := getTestConfig()
			cfg.Cluster = ""
			agent := &ecsAgent{cfg: &cfg}
			err := agent.setClusterInConfig("bar")
			assert.Error(t, err)
		})
	}
}

func TestSetClusterInConfig(t *testing.T) {
	cfg := getTestConfig()
	cfg.Cluster = clusterName
	agent := &ecsAgent{cfg: &cfg}
	err := agent.setClusterInConfig(clusterName)
	assert.NoError(t, err)
}

func TestGetEC2InstanceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	agent := &ecsAgent{ec2MetadataClient: ec2MetadataClient}

	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("error"))
	ec2MetadataClient.EXPECT().InstanceID().Return(instanceID, nil)
	assert.Equal(t, "i-123", agent.getEC2InstanceID())
}

func TestGetEC2InstanceIDBlackholedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	agent := &ecsAgent{ec2MetadataClient: ec2MetadataClient}

	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("blackholed"))
	assert.Equal(t, "", agent.getEC2InstanceID())
}

func TestGetEC2InstanceIDIIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	agent := &ecsAgent{ec2MetadataClient: ec2MetadataClient}

	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("error"))
	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("error"))
	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("error"))
	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("error"))
	ec2MetadataClient.EXPECT().InstanceID().Return("", errors.New("error"))
	assert.Equal(t, "", agent.getEC2InstanceID())
}

func TestGetOupostIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	agent := &ecsAgent{ec2MetadataClient: ec2MetadataClient}

	ec2MetadataClient.EXPECT().OutpostARN().Return("", errors.New("error"))
	assert.Equal(t, "", agent.getoutpostARN())
}

func TestReregisterContainerInstanceHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{""}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(containerInstanceARN, gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).Return(containerInstanceARN, availabilityZone, nil),
	)
	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()

	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          mockDockerClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		ec2MetadataClient:     mockEC2Metadata,
		serviceconnectManager: mockServiceConnectManager,
	}
	agent.containerInstanceARN = containerInstanceARN
	agent.availabilityZone = availabilityZone

	err := agent.registerContainerInstance(client, nil)
	assert.NoError(t, err)
}

func TestReregisterContainerInstanceInstanceTypeChanged(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{""}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(containerInstanceARN, gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).Return("", "", awserr.New("",
			apierrors.InstanceTypeChangedErrorMessage, errors.New(""))),
	)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          mockDockerClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		ec2MetadataClient:     mockEC2Metadata,
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}
	agent.containerInstanceARN = containerInstanceARN
	agent.availabilityZone = availabilityZone

	err := agent.registerContainerInstance(client, nil)
	assert.Error(t, err)
	assert.False(t, isTransient(err))
}

func TestReregisterContainerInstanceAttributeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()

	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(containerInstanceARN, gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).Return("", "", apierrors.NewAttributeError("error")),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		ec2MetadataClient:     mockEC2Metadata,
		dockerClient:          mockDockerClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}
	agent.containerInstanceARN = containerInstanceARN
	agent.availabilityZone = availabilityZone

	err := agent.registerContainerInstance(client, nil)
	assert.Error(t, err)
	assert.False(t, isTransient(err))
}

func TestReregisterContainerInstanceNonTerminalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(containerInstanceARN, gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).Return("", "", errors.New("error")),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          mockDockerClient,
		ec2MetadataClient:     mockEC2Metadata,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}
	agent.containerInstanceARN = containerInstanceARN
	agent.availabilityZone = availabilityZone

	err := agent.registerContainerInstance(client, nil)
	assert.Error(t, err)
	assert.True(t, isTransient(err))
}

func TestRegisterContainerInstanceWhenContainerInstanceARNIsNotSetHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)

	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance("", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return(containerInstanceARN, availabilityZone, nil),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          mockDockerClient,
		ec2MetadataClient:     mockEC2Metadata,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}
	err := agent.registerContainerInstance(client, nil)
	assert.NoError(t, err)
	assert.Equal(t, containerInstanceARN, agent.containerInstanceARN)
	assert.Equal(t, availabilityZone, agent.availabilityZone)
}

func TestRegisterContainerInstanceWhenContainerInstanceARNIsNotSetCanRetryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	retriableError := apierrors.NewRetriableError(apierrors.NewRetriable(true), errors.New("error"))
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance("", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return("", "", retriableError),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		dockerClient:          mockDockerClient,
		ec2MetadataClient:     mockEC2Metadata,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}

	err := agent.registerContainerInstance(client, nil)
	assert.Error(t, err)
	assert.True(t, isTransient(err))
}

func TestRegisterContainerInstanceWhenContainerInstanceARNIsNotSetCannotRetryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	cannotRetryError := apierrors.NewRetriableError(apierrors.NewRetriable(false), errors.New("error"))
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance("", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return("", "", cannotRetryError),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		ec2MetadataClient:     mockEC2Metadata,
		dockerClient:          mockDockerClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}

	err := agent.registerContainerInstance(client, nil)
	assert.Error(t, err)
	assert.False(t, isTransient(err))
}

func TestRegisterContainerInstanceWhenContainerInstanceARNIsNotSetAttributeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	client := mock_api.NewMockECSClient(ctrl)
	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		mockDockerClient.EXPECT().SupportedVersions().Return(nil),
		mockDockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		mockDockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance("", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return("", "", apierrors.NewAttributeError("error")),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	cfg.Cluster = clusterName
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                   ctx,
		cfg:                   &cfg,
		ec2MetadataClient:     mockEC2Metadata,
		dockerClient:          mockDockerClient,
		pauseLoader:           mockPauseLoader,
		credentialProvider:    aws_credentials.NewCredentials(mockCredentialsProvider),
		mobyPlugins:           mockMobyPlugins,
		serviceconnectManager: mockServiceConnectManager,
	}

	err := agent.registerContainerInstance(client, nil)
	assert.Error(t, err)
	assert.False(t, isTransient(err))
}

func TestRegisterContainerInstanceInvalidParameterTerminalError(t *testing.T) {
	ctrl, credentialsManager, state, imageManager, client,
		dockerClient, _, _, execCmdMgr, _ := setup(t)
	defer ctrl.Finish()

	mockCredentialsProvider := app_mocks.NewMockProvider(ctrl)
	mockMobyPlugins := mock_mobypkgwrapper.NewMockPlugins(ctrl)
	mockEC2Metadata := mock_ec2.NewMockEC2MetadataClient(ctrl)
	mockPauseLoader := mock_loader.NewMockLoader(ctrl)

	mockPauseLoader.EXPECT().IsLoaded(gomock.Any()).Return(false, nil).AnyTimes()
	mockPauseLoader.EXPECT().LoadImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockEC2Metadata.EXPECT().PrimaryENIMAC().Return("mac", nil)
	mockEC2Metadata.EXPECT().VPCID(gomock.Eq("mac")).Return("vpc-id", nil)
	mockEC2Metadata.EXPECT().SubnetID(gomock.Eq("mac")).Return("subnet-id", nil)
	mockServiceConnectManager := mock_serviceconnect.NewMockManager(ctrl)
	mockServiceConnectManager.EXPECT().IsLoaded(gomock.Any()).Return(true, nil).AnyTimes()
	mockServiceConnectManager.EXPECT().GetLoadedAppnetVersion().AnyTimes()
	mockServiceConnectManager.EXPECT().GetCapabilitiesForAppnetInterfaceVersion("").AnyTimes()
	mockServiceConnectManager.EXPECT().SetECSClient(gomock.Any(), gomock.Any()).AnyTimes()
	gomock.InOrder(
		dockerClient.EXPECT().SupportedVersions().Return(apiVersions),
		client.EXPECT().GetHostResources().Return(testHostResource, nil),
		mockCredentialsProvider.EXPECT().Retrieve().Return(aws_credentials.Value{}, nil),
		dockerClient.EXPECT().SupportedVersions().Return(nil),
		dockerClient.EXPECT().KnownVersions().Return(nil),
		mockMobyPlugins.EXPECT().Scan().AnyTimes().Return([]string{}, nil),
		dockerClient.EXPECT().ListPluginsWithFilters(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).AnyTimes().Return([]string{}, nil),
		client.EXPECT().RegisterContainerInstance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any()).Return("", "", awserr.New("InvalidParameterException", "", nil)),
	)
	mockEC2Metadata.EXPECT().OutpostARN().Return("", nil)

	cfg := getTestConfig()
	ctx, cancel := context.WithCancel(context.TODO())
	// Cancel the context to cancel async routines
	defer cancel()
	agent := &ecsAgent{
		ctx:                ctx,
		ec2MetadataClient:  mockEC2Metadata,
		cfg:                &cfg,
		pauseLoader:        mockPauseLoader,
		credentialProvider: aws_credentials.NewCredentials(mockCredentialsProvider),
		dockerClient:       dockerClient,
		mobyPlugins:        mockMobyPlugins,
		terminationHandler: func(taskEngineState dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
		},
		serviceconnectManager: mockServiceConnectManager,
	}

	exitCode := agent.doStart(eventstream.NewEventStream("events", ctx),
		credentialsManager, state, imageManager, client, execCmdMgr)
	assert.Equal(t, exitcodes.ExitTerminal, exitCode)
}
func TestMergeTags(t *testing.T) {
	ec2Key := "ec2Key"
	ec2Value := "ec2Value"
	localKey := "localKey"
	localValue := "localValue"
	commonKey := "commonKey"
	commonKeyEC2Value := "commonEC2Value"
	commonKeyLocalValue := "commonKeyLocalValue"

	localTags := []*ecs.Tag{
		{
			Key:   aws.String(localKey),
			Value: aws.String(localValue),
		},
		{
			Key:   aws.String(commonKey),
			Value: aws.String(commonKeyLocalValue),
		},
	}

	ec2Tags := []*ecs.Tag{
		{
			Key:   aws.String(ec2Key),
			Value: aws.String(ec2Value),
		},
		{
			Key:   aws.String(commonKey),
			Value: aws.String(commonKeyEC2Value),
		},
	}

	mergedTags := mergeTags(localTags, ec2Tags)

	assert.Equal(t, len(mergedTags), 3)
	sort.Slice(mergedTags, func(i, j int) bool {
		return aws.StringValue(mergedTags[i].Key) < aws.StringValue(mergedTags[j].Key)
	})

	assert.Equal(t, commonKey, aws.StringValue(mergedTags[0].Key))
	assert.Equal(t, commonKeyLocalValue, aws.StringValue(mergedTags[0].Value))
	assert.Equal(t, ec2Key, aws.StringValue(mergedTags[1].Key))
	assert.Equal(t, ec2Value, aws.StringValue(mergedTags[1].Value))
	assert.Equal(t, localKey, aws.StringValue(mergedTags[2].Key))
	assert.Equal(t, localValue, aws.StringValue(mergedTags[2].Value))
}

func TestGetContainerInstanceTagsFromEC2API(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)

	agent := &ecsAgent{
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
	}
	instanceID := "iid"
	tags := []*ecs.Tag{
		{
			Key:   aws.String("key"),
			Value: aws.String("value"),
		},
	}
	gomock.InOrder(
		ec2MetadataClient.EXPECT().InstanceID().Return(instanceID, nil),
		ec2Client.EXPECT().DescribeECSTagsForInstance(instanceID).Return(tags, nil),
	)

	resTags, err := agent.getContainerInstanceTagsFromEC2API()
	assert.NoError(t, err)
	assert.Equal(t, tags, resTags)
}

func TestGetContainerInstanceTagsFromEC2APIFailToDescribeECSTagsForInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)

	agent := &ecsAgent{
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
	}
	instanceID := "iid"
	gomock.InOrder(
		ec2MetadataClient.EXPECT().InstanceID().Return(instanceID, nil),
		ec2Client.EXPECT().DescribeECSTagsForInstance(instanceID).Return(nil, errors.New("error")),
	)

	resTags, err := agent.getContainerInstanceTagsFromEC2API()
	assert.Error(t, err)
	assert.Nil(t, resTags)
}

func TestGetHostPrivateIPv4AddressFromEC2Metadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)

	agent := &ecsAgent{
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
	}
	ec2MetadataClient.EXPECT().PrivateIPv4Address().Return(hostPrivateIPv4Address, nil)

	assert.Equal(t, hostPrivateIPv4Address, agent.getHostPrivateIPv4AddressFromEC2Metadata())
}

func TestGetHostPrivateIPv4AddressFromEC2MetadataFailWithError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)

	agent := &ecsAgent{
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
	}
	ec2MetadataClient.EXPECT().PrivateIPv4Address().Return("", errors.New("Unable to get IP Address"))

	assert.Empty(t, agent.getHostPrivateIPv4AddressFromEC2Metadata())
}

func TestGetHostPublicIPv4AddressFromEC2Metadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)

	agent := &ecsAgent{
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
	}
	ec2MetadataClient.EXPECT().PublicIPv4Address().Return(hostPublicIPv4Address, nil)

	assert.Equal(t, hostPublicIPv4Address, agent.getHostPublicIPv4AddressFromEC2Metadata())
}

func TestGetHostPublicIPv4AddressFromEC2MetadataFailWithError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)

	agent := &ecsAgent{
		ec2MetadataClient: ec2MetadataClient,
		ec2Client:         ec2Client,
	}
	ec2MetadataClient.EXPECT().PublicIPv4Address().Return("", errors.New("Unable to get IP Address"))

	assert.Empty(t, agent.getHostPublicIPv4AddressFromEC2Metadata())
}

func TestSpotInstanceActionCheck_Sunny(t *testing.T) {
	tests := []struct {
		jsonresp string
	}{
		{jsonresp: `{"action": "terminate", "time": "2017-09-18T08:22:00Z"}`},
		{jsonresp: `{"action": "hibernate", "time": "2017-09-18T08:22:00Z"}`},
		{jsonresp: `{"action": "stop", "time": "2017-09-18T08:22:00Z"}`},
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	for _, test := range tests {
		myARN := "myARN"
		agent := &ecsAgent{
			ec2MetadataClient:    ec2MetadataClient,
			ec2Client:            ec2Client,
			containerInstanceARN: myARN,
		}
		ec2MetadataClient.EXPECT().SpotInstanceAction().Return(test.jsonresp, nil)
		ecsClient.EXPECT().UpdateContainerInstancesState(myARN, "DRAINING").Return(nil)

		assert.True(t, agent.spotInstanceDrainingPoller(ecsClient))
	}
}

func TestSpotInstanceActionCheck_Fail(t *testing.T) {
	tests := []struct {
		jsonresp string
	}{
		{jsonresp: `{"action": "terminate" "time": "2017-09-18T08:22:00Z"}`}, // invalid json
		{jsonresp: ``}, // empty json
		{jsonresp: `{"action": "flip!", "time": "2017-09-18T08:22:00Z"}`}, // invalid action
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	for _, test := range tests {
		myARN := "myARN"
		agent := &ecsAgent{
			ec2MetadataClient:    ec2MetadataClient,
			ec2Client:            ec2Client,
			containerInstanceARN: myARN,
		}
		ec2MetadataClient.EXPECT().SpotInstanceAction().Return(test.jsonresp, nil)
		// Container state should NOT be updated because the termination time field is empty.
		ecsClient.EXPECT().UpdateContainerInstancesState(gomock.Any(), gomock.Any()).Times(0)

		assert.False(t, agent.spotInstanceDrainingPoller(ecsClient))
	}
}

func TestSpotInstanceActionCheck_NoInstanceActionYet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
	ec2Client := mock_ec2.NewMockClient(ctrl)
	ecsClient := mock_api.NewMockECSClient(ctrl)

	myARN := "myARN"
	agent := &ecsAgent{
		ec2MetadataClient:    ec2MetadataClient,
		ec2Client:            ec2Client,
		containerInstanceARN: myARN,
	}
	ec2MetadataClient.EXPECT().SpotInstanceAction().Return("", fmt.Errorf("404"))

	// Container state should NOT be updated because there is no termination time.
	ecsClient.EXPECT().UpdateContainerInstancesState(gomock.Any(), gomock.Any()).Times(0)

	assert.False(t, agent.spotInstanceDrainingPoller(ecsClient))
}

func TestSaveMetadata(t *testing.T) {
	dataClient := newTestDataClient(t)

	agent := &ecsAgent{
		dataClient: dataClient,
	}
	agent.saveMetadata(data.AvailabilityZoneKey, availabilityZone)
	az, err := dataClient.GetMetadata(data.AvailabilityZoneKey)
	require.NoError(t, err)
	assert.Equal(t, availabilityZone, az)
}

func getTestConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	return cfg
}

func getTestHostResources() map[string]*ecs.Resource {
	hostResources := make(map[string]*ecs.Resource)
	CPUs := int64(1024)
	hostResources["CPU"] = &ecs.Resource{
		Name:         utils.Strptr("CPU"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &CPUs,
	}
	//MEMORY
	memory := int64(1024)
	hostResources["MEMORY"] = &ecs.Resource{
		Name:         utils.Strptr("MEMORY"),
		Type:         utils.Strptr("INTEGER"),
		IntegerValue: &memory,
	}
	//PORTS
	ports_tcp := []*string{}
	hostResources["PORTS_TCP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_TCP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: ports_tcp,
	}
	//PORTS_UDP
	ports_udp := []*string{}
	hostResources["PORTS_UDP"] = &ecs.Resource{
		Name:           utils.Strptr("PORTS_UDP"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: ports_udp,
	}
	//GPUs
	gpuIDs := []string{"gpu1", "gpu2", "gpu3", "gpu4"}
	hostResources["GPU"] = &ecs.Resource{
		Name:           utils.Strptr("GPU"),
		Type:           utils.Strptr("STRINGSET"),
		StringSetValue: aws.StringSlice(gpuIDs),
	}
	return hostResources
}

func getTestDaemonManagers() map[string]daemonmanager.DaemonManager {
	daemonManagers := make(map[string]daemonmanager.DaemonManager)
	return daemonManagers
}

func newTestDataClient(t *testing.T) data.Client {
	testDir := t.TempDir()

	testClient, err := data.NewWithSetup(testDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, testClient.Close())
	})
	return testClient
}

type targetLifecycleFuncDetail struct {
	val         string
	err         error
	returnTimes int
}

func TestWaitUntilInstanceInServicePolling(t *testing.T) {
	warmedResult := targetLifecycleFuncDetail{warmedState, nil, 1}
	inServiceResult := targetLifecycleFuncDetail{inServiceState, nil, 1}
	notFoundErrResult := targetLifecycleFuncDetail{"", notFoundErr, testTargetLifecycleMaxRetryCount}
	unexpectedErrResult := targetLifecycleFuncDetail{"", badReqErr, testTargetLifecycleMaxRetryCount}
	serverErrResult := targetLifecycleFuncDetail{"", serverErr, testTargetLifecycleMaxRetryCount}
	testCases := []struct {
		name            string
		funcTestDetails []targetLifecycleFuncDetail
		result          error
		maxPolls        int
	}{
		{"TestWaitUntilInServicePollWarmed", []targetLifecycleFuncDetail{warmedResult, warmedResult, inServiceResult}, nil, asgLifecyclePollMax},
		{"TestWaitUntilInServicePollMissing", []targetLifecycleFuncDetail{notFoundErrResult, inServiceResult}, nil, asgLifecyclePollMax},
		{"TestWaitUntilInServiceErrPollMaxReached", []targetLifecycleFuncDetail{notFoundErrResult}, notFoundErr, 1},
		{"TestWaitUntilInServiceNoStateUnexpectedErr", []targetLifecycleFuncDetail{unexpectedErrResult}, badReqErr, asgLifecyclePollMax},
		{"TestWaitUntilInServiceUnexpectedErr", []targetLifecycleFuncDetail{warmedResult, unexpectedErrResult}, badReqErr, asgLifecyclePollMax},
		{"TestWaitUntilInServiceServerErrContinue", []targetLifecycleFuncDetail{warmedResult, serverErrResult, inServiceResult}, nil, asgLifecyclePollMax},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cfg := getTestConfig()
			cfg.WarmPoolsSupport = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
			ec2MetadataClient := mock_ec2.NewMockEC2MetadataClient(ctrl)
			agent := &ecsAgent{ec2MetadataClient: ec2MetadataClient, cfg: &cfg}
			for _, detail := range tc.funcTestDetails {
				ec2MetadataClient.EXPECT().TargetLifecycleState().Return(detail.val, detail.err).Times(detail.returnTimes)
			}
			assert.Equal(t, tc.result, agent.waitUntilInstanceInService(1*time.Millisecond, tc.maxPolls, testTargetLifecycleMaxRetryCount))
		})
	}
}
