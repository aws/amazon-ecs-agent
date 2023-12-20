//go:build test
// +build test

// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-init/apparmor"
	"github.com/aws/amazon-ecs-agent/ecs-init/cache"
	"github.com/aws/amazon-ecs-agent/ecs-init/gpu"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	ctrdapparmor "github.com/containerd/containerd/pkg/apparmor"
)

// getDockerClientMock backs up getDockerClient package-level function and replaces it with the mock passed as
// parameter. The backup can be restored by executing the returned function in a deferred manner.
// (e.g. defer getDockerClientMock(mock)() )
func getDockerClientMock(mockDocker dockerClient) func() {
	getDockerClientBkp := getDockerClient
	getDockerClient = func() (dockerClient, error) {
		return mockDocker, nil
	}
	return func() {
		getDockerClient = getDockerClientBkp
	}
}

func TestPreStartImageAlreadyCachedAndLoaded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(nil)
	// Docker reports image is loaded.
	mockDocker.EXPECT().IsAgentImageLoaded().Return(true, nil)
	// Agent tarball and state is present
	mockDownloader.EXPECT().AgentCacheStatus().Return(cache.StatusCached)

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().Enable().Return(nil)
	mockIpv6RouterAdvertisements := NewMockipv6RouterAdvertisements(mockCtrl)
	mockIpv6RouterAdvertisements.EXPECT().Disable().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Create().Return(nil)

	engine := &Engine{
		downloader:               mockDownloader,
		loopbackRouting:          mockLoopbackRouting,
		ipv6RouterAdvertisements: mockIpv6RouterAdvertisements,
		credentialsProxyRoute:    mockRoute,
	}
	err := engine.PreStart()
	if err != nil {
		t.Errorf("engine pre-start error: %v", err)
	}
}

func TestPreStartReloadNeeded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := io.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(nil)
	// Docker reports image is loaded.
	mockDocker.EXPECT().IsAgentImageLoaded().Return(true, nil)
	// Agent tarball and state is present, but requires a reload off of disk
	mockDownloader.EXPECT().AgentCacheStatus().Return(cache.StatusReloadNeeded)
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)
	mockDownloader.EXPECT().RecordCachedAgent()

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().Enable().Return(nil)
	mockIpv6RouterAdvertisements := NewMockipv6RouterAdvertisements(mockCtrl)
	mockIpv6RouterAdvertisements.EXPECT().Disable().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Create().Return(nil)

	engine := &Engine{
		downloader:               mockDownloader,
		loopbackRouting:          mockLoopbackRouting,
		ipv6RouterAdvertisements: mockIpv6RouterAdvertisements,
		credentialsProxyRoute:    mockRoute,
	}
	err := engine.PreStart()
	if err != nil {
		t.Errorf("engine pre-start error: %v", err)
	}
}

func TestPreStartImageNotLoadedCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := io.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	mockDownloader := NewMockdownloader(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(nil)
	mockRoute.EXPECT().Create().Return(nil)
	mockLoopbackRouting.EXPECT().Enable().Return(nil)
	mockIpv6RouterAdvertisements := NewMockipv6RouterAdvertisements(mockCtrl)
	mockIpv6RouterAdvertisements.EXPECT().Disable().Return(nil)
	mockDocker.EXPECT().IsAgentImageLoaded().Return(false, nil)
	mockDownloader.EXPECT().AgentCacheStatus().Return(cache.StatusCached)
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)
	mockDownloader.EXPECT().RecordCachedAgent()

	engine := &Engine{
		downloader:               mockDownloader,
		loopbackRouting:          mockLoopbackRouting,
		ipv6RouterAdvertisements: mockIpv6RouterAdvertisements,
		credentialsProxyRoute:    mockRoute,
	}
	err := engine.PreStart()
	if err != nil {
		t.Errorf("engine pre-start error: %v", err)
	}
}

func TestPreStartImageNotCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := io.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(nil)
	mockDocker.EXPECT().IsAgentImageLoaded().Return(false, nil)
	mockDownloader.EXPECT().AgentCacheStatus().Return(cache.StatusUncached)
	mockDownloader.EXPECT().DownloadAgent()
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)
	mockDownloader.EXPECT().RecordCachedAgent()

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().Enable().Return(nil)
	mockIpv6RouterAdvertisements := NewMockipv6RouterAdvertisements(mockCtrl)
	mockIpv6RouterAdvertisements.EXPECT().Disable().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Create().Return(nil)

	engine := &Engine{
		downloader:               mockDownloader,
		loopbackRouting:          mockLoopbackRouting,
		ipv6RouterAdvertisements: mockIpv6RouterAdvertisements,
		credentialsProxyRoute:    mockRoute,
	}
	err := engine.PreStart()
	if err != nil {
		t.Errorf("engine pre-start error: %v", err)
	}
}

func TestPreStartGPUSetupSuccessful(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)
	mockGPUManager := gpu.NewMockGPUManager(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(map[string]string{
		"ECS_ENABLE_GPU_SUPPORT": "true",
	})
	mockGPUManager.EXPECT().Setup().Return(nil)
	// Docker reports image is loaded.
	mockDocker.EXPECT().IsAgentImageLoaded().Return(true, nil)
	// Agent tarball and state is present
	mockDownloader.EXPECT().AgentCacheStatus().Return(cache.StatusCached)

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().Enable().Return(nil)
	mockIpv6RouterAdvertisements := NewMockipv6RouterAdvertisements(mockCtrl)
	mockIpv6RouterAdvertisements.EXPECT().Disable().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Create().Return(nil)

	engine := &Engine{
		downloader:               mockDownloader,
		loopbackRouting:          mockLoopbackRouting,
		ipv6RouterAdvertisements: mockIpv6RouterAdvertisements,
		credentialsProxyRoute:    mockRoute,
		nvidiaGPUManager:         mockGPUManager,
	}
	err := engine.PreStart()
	if err != nil {
		t.Errorf("engine pre-start error: %v", err)
	}
}

func TestPreStartGPUSetupError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockGPUManager := gpu.NewMockGPUManager(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(map[string]string{
		"ECS_ENABLE_GPU_SUPPORT": "true",
	})
	mockGPUManager.EXPECT().Setup().Return(errors.New("gpu setup failed"))
	engine := &Engine{
		nvidiaGPUManager: mockGPUManager,
	}
	err := engine.PreStart()
	if err == nil {
		t.Error("Expected error to be returned but was nil")
	}
}

func TestStartSupervisedCannotStart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDocker.EXPECT().RemoveExistingAgentContainer()
	mockDocker.EXPECT().StartAgent().Return(0, errors.New("test error"))

	engine := &Engine{}
	err := engine.StartSupervised()
	if err == nil {
		t.Error("Expected error to be returned but was nil")
	}
}

func TestStartSupervisedExitsWhenTerminalFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()

	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(TerminalFailureAgentExitCode, nil),
	)

	engine := &Engine{}
	err := engine.StartSupervised()

	if err == nil {
		t.Error("Expected error to be returned but was nil")
	}

	_, ok := err.(*TerminalError)
	if !ok {
		t.Error("Expected error to be of type TerminalError")
	}
}

func TestLogContainerFailureAgentExitCodeFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDocker.EXPECT().RemoveExistingAgentContainer()
	mockDocker.EXPECT().StartAgent().Return(2, nil)
	mockDocker.EXPECT().GetContainerLogTail(gomock.Any())
	mockDocker.EXPECT().RemoveExistingAgentContainer()
	mockDocker.EXPECT().StartAgent().Return(0, errors.New("test error"))

	engine := &Engine{}
	err := engine.StartSupervised()
	if err == nil {
		t.Error("Expected error to be returned but was nil")
	}
}

func TestStartSupervisedExitsWhenTerminalSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(terminalSuccessAgentExitCode, nil),
	)

	engine := &Engine{}
	err := engine.StartSupervised()
	if err != nil {
		t.Error("Expected error to be nil but was returned")
	}
}

func TestStartSupervisedUpgradeOpenFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(upgradeAgentExitCode, nil),
		mockDownloader.EXPECT().LoadDesiredAgent().Return(nil, errors.New("test error")),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(terminalSuccessAgentExitCode, nil),
	)

	engine := &Engine{
		downloader: mockDownloader,
	}
	err := engine.StartSupervised()
	if err != nil {
		t.Error("Expected error to be nil but was returned")
	}
}

func TestStartSupervisedUpgradeLoadFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(upgradeAgentExitCode, nil),
		mockDownloader.EXPECT().LoadDesiredAgent().Return(&os.File{}, nil),
		mockDocker.EXPECT().LoadImage(gomock.Any()).Return(errors.New("test error")),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(terminalSuccessAgentExitCode, nil),
	)

	engine := &Engine{
		downloader: mockDownloader,
	}
	err := engine.StartSupervised()
	if err != nil {
		t.Error("Expected error to be nil but was returned")
	}
}

func TestStartSupervisedUpgrade(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(upgradeAgentExitCode, nil),
		mockDownloader.EXPECT().LoadDesiredAgent().Return(&os.File{}, nil),
		mockDocker.EXPECT().LoadImage(gomock.Any()),
		mockDownloader.EXPECT().RecordCachedAgent(),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(terminalSuccessAgentExitCode, nil),
	)

	engine := &Engine{
		downloader: mockDownloader,
	}
	err := engine.StartSupervised()
	if err != nil {
		t.Error("Expected error to be nil but was returned")
	}
}

func TestPreStop(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDocker.EXPECT().StopAgent()

	engine := &Engine{}
	err := engine.PreStop()
	if err != nil {
		t.Errorf("engine pre-stop error: %v", err)
	}
}

func TestReloadCacheNotCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := io.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDownloader.EXPECT().IsAgentCached().Return(false)
	mockDownloader.EXPECT().DownloadAgent()
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)
	mockDownloader.EXPECT().RecordCachedAgent()

	engine := &Engine{
		downloader: mockDownloader,
	}
	err := engine.ReloadCache()
	if err != nil {
		t.Errorf("engine reload-cache error: %v", err)
	}
}

func TestReloadCacheCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := io.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDownloader.EXPECT().IsAgentCached().Return(true)
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)
	mockDownloader.EXPECT().RecordCachedAgent()

	engine := &Engine{
		downloader: mockDownloader,
	}
	err := engine.ReloadCache()
	if err != nil {
		t.Errorf("engine reload-cache error: %v", err)
	}
}

func TestPrestartLoopbackRoutingNotEnabled(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)
	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)

	mockDocker.EXPECT().LoadEnvVars().Return(nil)
	mockLoopbackRouting.EXPECT().Enable().Return(fmt.Errorf("sysctl not found"))
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)

	engine := &Engine{
		downloader:            mockDownloader,
		loopbackRouting:       mockLoopbackRouting,
		credentialsProxyRoute: mockRoute,
	}
	err := engine.PreStart()
	if err == nil {
		t.Error("Expected pre-start error when loopback routing not enabled")
	}
}

func TestPrestartCredentialsProxyRouteNotCreated(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	defer getDockerClientMock(mockDocker)()
	mockDownloader := NewMockdownloader(mockCtrl)
	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockIpv6RouterAdvertisements := NewMockipv6RouterAdvertisements(mockCtrl)
	mockIpv6RouterAdvertisements.EXPECT().Disable().Return(nil)

	mockDocker.EXPECT().LoadEnvVars().Return(nil)
	mockLoopbackRouting.EXPECT().Enable().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Create().Return(fmt.Errorf("iptables not found"))

	engine := &Engine{
		downloader:               mockDownloader,
		loopbackRouting:          mockLoopbackRouting,
		ipv6RouterAdvertisements: mockIpv6RouterAdvertisements,
		credentialsProxyRoute:    mockRoute,
	}
	err := engine.PreStart()
	if err == nil {
		t.Error("Expected pre-start error when the credentials proxy route cannot be created")
	}
}

func TestPostStop(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().RestoreDefault().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Remove().Return(nil)

	engine := &Engine{
		loopbackRouting:       mockLoopbackRouting,
		credentialsProxyRoute: mockRoute,
	}
	err := engine.PostStop()
	if err != nil {
		t.Errorf("engine post-stop error: %v", err)
	}
}

func TestPostStopLoopbackRoutingError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().RestoreDefault().Return(fmt.Errorf("cannot restore"))
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Remove().Return(nil)

	engine := &Engine{
		loopbackRouting:       mockLoopbackRouting,
		credentialsProxyRoute: mockRoute,
	}
	err := engine.PostStop()
	if err == nil {
		t.Error("Expected error during engine post-stop")
	}
}

func TestPostStopCredentialsProxyRouteRemoveError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockLoopbackRouting := NewMockloopbackRouting(mockCtrl)
	mockLoopbackRouting.EXPECT().RestoreDefault().Return(nil)
	mockRoute := NewMockcredentialsProxyRoute(mockCtrl)
	mockRoute.EXPECT().Remove().Return(fmt.Errorf("cannot remove"))

	engine := &Engine{
		loopbackRouting:       mockLoopbackRouting,
		credentialsProxyRoute: mockRoute,
	}
	err := engine.PostStop()
	if err != nil {
		t.Errorf("engine post-stop error: %v", err)
	}
}

func TestPreStartAppArmorSetup(t *testing.T) {
	testCases := []struct {
		name             string
		hostSupports     bool
		loadProfileError error
		expectedError    error
	}{
		{
			name:             "HostNotSupported",
			hostSupports:     false,
			loadProfileError: nil,
			expectedError:    nil,
		},
		{
			name:             "HostSupportedNoError",
			hostSupports:     true,
			loadProfileError: nil,
			expectedError:    nil,
		},
		{
			name:             "HostSupportedWithError",
			hostSupports:     true,
			loadProfileError: errors.New("error loading apparmor profile"),
			expectedError:    errors.New("error loading apparmor profile"),
		},
	}
	defer func() {
		hostSupports = ctrdapparmor.HostSupports
		loadDefaultProfile = apparmor.LoadDefaultProfile
	}()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hostSupports = func() bool {
				return tc.hostSupports
			}
			loadDefaultProfile = func(profile string) error {
				return tc.loadProfileError
			}
			engine := &Engine{}
			err := engine.PreStartAppArmor()
			assert.Equal(t, tc.expectedError, err)
		})
	}
}
