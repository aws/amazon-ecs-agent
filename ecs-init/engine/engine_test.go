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
	"code.google.com/p/gomock/gomock"
	"errors"
	"io/ioutil"
	"os"
	"testing"
)

func TestPreStartImageAlreadyLoaded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
	mockDocker.EXPECT().IsAgentImageLoaded().Return(true, nil)

	engine := &Engine{
		docker: mockDocker,
	}
	engine.PreStart()
}

func TestPreStartImageNotLoadedCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := ioutil.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDocker.EXPECT().IsAgentImageLoaded().Return(false, nil)
	mockDownloader.EXPECT().IsAgentCached().Return(true)
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)

	engine := &Engine{
		docker:     mockDocker,
		downloader: mockDownloader,
	}
	engine.PreStart()
}

func TestPreStartImageNotLoadedNotCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := ioutil.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDocker.EXPECT().IsAgentImageLoaded().Return(false, nil)
	mockDownloader.EXPECT().IsAgentCached().Return(false)
	mockDownloader.EXPECT().DownloadAgent()
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)

	engine := &Engine{
		docker:     mockDocker,
		downloader: mockDownloader,
	}
	engine.PreStart()
}

func TestStartSupervisedCannotStart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)

	mockDocker.EXPECT().RemoveExistingAgentContainer()
	mockDocker.EXPECT().StartAgent().Return(0, errors.New("test error"))

	engine := &Engine{
		docker: mockDocker,
	}
	err := engine.StartSupervised()
	if err == nil {
		t.Error("Expected error to be returned but was nil")
	}
}

func TestStartSupervisedExitsWhenTerminalFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)

	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(1, nil),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(terminalFailureAgentExitCode, nil),
	)

	engine := &Engine{
		docker: mockDocker,
	}
	err := engine.StartSupervised()
	if err == nil {
		t.Error("Expected error to be returned but was nil")
	}
}

func TestStartSupervisedExitsWhenTerminalSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)

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

	engine := &Engine{
		docker: mockDocker,
	}
	err := engine.StartSupervised()
	if err != nil {
		t.Error("Expected error to be nil but was returned")
	}
}

func TestStartSupervisedUpgradeOpenFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)
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
		docker:     mockDocker,
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
		docker:     mockDocker,
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
	mockDownloader := NewMockdownloader(mockCtrl)

	gomock.InOrder(
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(upgradeAgentExitCode, nil),
		mockDownloader.EXPECT().LoadDesiredAgent().Return(&os.File{}, nil),
		mockDocker.EXPECT().LoadImage(gomock.Any()),
		mockDocker.EXPECT().RemoveExistingAgentContainer(),
		mockDocker.EXPECT().StartAgent().Return(terminalSuccessAgentExitCode, nil),
	)

	engine := &Engine{
		downloader: mockDownloader,
		docker:     mockDocker,
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

	mockDocker.EXPECT().StopAgent()

	engine := &Engine{
		docker: mockDocker,
	}
	engine.PreStop()
}

func TestReloadCacheNotCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := ioutil.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDownloader.EXPECT().IsAgentCached().Return(false)
	mockDownloader.EXPECT().DownloadAgent()
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)

	engine := &Engine{
		docker:     mockDocker,
		downloader: mockDownloader,
	}
	engine.ReloadCache()
}

func TestReloadCacheCached(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	cachedAgentBuffer := ioutil.NopCloser(&bytes.Buffer{})

	mockDocker := NewMockdockerClient(mockCtrl)
	mockDownloader := NewMockdownloader(mockCtrl)

	mockDownloader.EXPECT().IsAgentCached().Return(true)
	mockDownloader.EXPECT().LoadCachedAgent().Return(cachedAgentBuffer, nil)
	mockDocker.EXPECT().LoadImage(cachedAgentBuffer)

	engine := &Engine{
		docker:     mockDocker,
		downloader: mockDownloader,
	}
	engine.ReloadCache()
}
