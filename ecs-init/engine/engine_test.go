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

func TestStartCannotRemoveExisting(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)

	mockDocker.EXPECT().RemoveExistingAgentContainer().Return(errors.New("test error"))

	engine := &Engine{
		docker: mockDocker,
	}
	engine.Start()
}

func TestStartCannotStart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)

	mockDocker.EXPECT().RemoveExistingAgentContainer()
	mockDocker.EXPECT().StartAgent().Return(0, errors.New("test error"))

	engine := &Engine{
		docker: mockDocker,
	}
	engine.Start()
}

func TestStart(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDocker := NewMockdockerClient(mockCtrl)

	mockDocker.EXPECT().RemoveExistingAgentContainer()
	mockDocker.EXPECT().StartAgent()

	engine := &Engine{
		docker: mockDocker,
	}
	engine.Start()
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

func TestUpdateCacheIsLatest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDownloader := NewMockdownloader(mockCtrl)

	mockDownloader.EXPECT().IsAgentLatest().Return(true)

	engine := &Engine{
		downloader: mockDownloader,
	}
	engine.UpdateCache()
}

func TestUpdateCacheIsNotLatest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockDownloader := NewMockdownloader(mockCtrl)

	mockDownloader.EXPECT().IsAgentLatest().Return(false)
	mockDownloader.EXPECT().DownloadAgent()

	engine := &Engine{
		downloader: mockDownloader,
	}
	engine.UpdateCache()
}
