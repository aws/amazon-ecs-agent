// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package cache

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/aws/amazon-ecs-init/ecs-init/config"
	"github.com/golang/mock/gomock"
)

func TestIsAgentCachedFalseMissingState(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().Stat(config.CacheState()).Return(nil, errors.New("test error"))

	d := &Downloader{
		fs: mockFS,
	}

	if d.IsAgentCached() {
		t.Error("Expected d.IsAgentCached() to be false")
	}
}

func TestIsAgentCachedFalseEmptyState(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockFSInfo := NewMockfileSizeInfo(mockCtrl)
	mockFS.EXPECT().Stat(config.CacheState()).Return(mockFSInfo, nil)
	mockFSInfo.EXPECT().Size().Return(int64(0))

	d := &Downloader{
		fs: mockFS,
	}

	if d.IsAgentCached() {
		t.Error("Expected d.IsAgentCached() to be false")
	}
}

func TestIsAgentCachedFalseMissingTarball(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockFSInfo := NewMockfileSizeInfo(mockCtrl)
	mockFS.EXPECT().Stat(config.CacheState()).Return(mockFSInfo, nil)
	mockFSInfo.EXPECT().Size().Return(int64(1))
	mockFS.EXPECT().Stat(config.AgentTarball()).Return(nil, errors.New("test error"))

	d := &Downloader{
		fs: mockFS,
	}

	if d.IsAgentCached() {
		t.Error("Expected d.IsAgentCached() to be false")
	}
}

func TestIsAgentCachedTrue(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockFSInfo := NewMockfileSizeInfo(mockCtrl)
	mockFS.EXPECT().Stat(config.CacheState()).Return(mockFSInfo, nil)
	mockFS.EXPECT().Stat(config.AgentTarball()).Return(mockFSInfo, nil)
	mockFSInfo.EXPECT().Size().Return(int64(1)).Times(2)

	d := &Downloader{
		fs: mockFS,
	}

	if !d.IsAgentCached() {
		t.Error("Expected d.IsAgentCached() to be true")
	}
}

func TestGetBucketRegion(t *testing.T) {
	d := &Downloader{}

	var cases = []struct {
		region         string
		expectedResult string
	}{
		{config.DefaultRegionName, config.DefaultRegionName},
		{"cn-north-1", "cn-north-1"},
		{"missing-region", config.DefaultRegionName},
	}

	for _, c := range cases {
		t.Run(c.region, func(t *testing.T) {
			d.region = c.region
			result := d.getBucketRegion()
			if result != c.expectedResult {
				t.Errorf("getBucketRegion did not get correct result. Result returned: %t", result)
			}
		})
	}
}

func TestDownloadAgentMkdirFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700).Return(errors.New("test error"))

	d := &Downloader{
		fs:       mockFS,
		metadata: mockMetadata,
		region:   config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentDownloadMD5Failure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), errors.New("test error")),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentReadPublishedMd5Failure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().ReadAll(gomock.Any()).Return(nil, errors.New("test error")),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentDownloadTarballFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	tempAgentFile, err := ioutil.TempFile("", "agent-test")
	if err != nil {
		t.Fail()
	}
	defer tempAgentFile.Close()

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().ReadAll(gomock.Any()).Return([]byte(md5sum), nil),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar").Return(tempAgentFile, nil),
		mocks3Downloader.EXPECT().Download(tempAgentFile, gomock.Any()).Return(int64(0), errors.New("test error")),
		mockFS.EXPECT().Remove(tempAgentFile.Name()),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentMD5TempFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(nil, errors.New("test error")),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentTempFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().ReadAll(tempMD5File).Return([]byte(md5sum), nil),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar").Return(nil, errors.New("test error")),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentCopyFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	tempAgentFile, err := ioutil.TempFile("", "agent-test")
	if err != nil {
		t.Fail()
	}
	defer tempAgentFile.Close()

	tempReader := ioutil.NopCloser(&bytes.Buffer{})

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().ReadAll(tempMD5File).Return([]byte(md5sum), nil),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar").Return(tempAgentFile, nil),
		mocks3Downloader.EXPECT().Download(tempAgentFile, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().Open(tempAgentFile.Name()).Return(tempReader, nil),
		mockFS.EXPECT().Copy(gomock.Any(), tempReader).Return(int64(0), errors.New("test error")),
		mockFS.EXPECT().Stat(tempAgentFile.Name()).Return(nil, nil),
		mockFS.EXPECT().Remove(tempAgentFile.Name()),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentMD5Mismatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	tempAgentFile, err := ioutil.TempFile("", "agent-test")
	if err != nil {
		t.Fail()
	}
	defer tempAgentFile.Close()

	tempReader := ioutil.NopCloser(&bytes.Buffer{})

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().ReadAll(tempMD5File).Return([]byte(md5sum), nil),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar").Return(tempAgentFile, nil),
		mocks3Downloader.EXPECT().Download(tempAgentFile, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().Open(tempAgentFile.Name()).Return(tempReader, nil),
		mockFS.EXPECT().Copy(gomock.Any(), tempReader).Return(int64(0), nil),
		mockFS.EXPECT().Stat(tempAgentFile.Name()).Return(nil, nil),
		mockFS.EXPECT().Remove(tempAgentFile.Name()),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestDownloadAgentSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	tarballContents := "tarball contents"
	tarballReader := ioutil.NopCloser(bytes.NewBufferString(tarballContents))
	expectedMd5Sum := fmt.Sprintf("%x\n", md5.Sum([]byte(tarballContents)))

	tempMD5File, err := ioutil.TempFile("", "md5-test")
	if err != nil {
		t.Fail()
	}
	defer tempMD5File.Close()

	tempAgentFile, err := ioutil.TempFile("", "agent-test")
	if err != nil {
		t.Fail()
	}
	defer tempAgentFile.Close()

	mockFS := NewMockfileSystem(mockCtrl)
	mocks3Downloader := NewMocks3Downloader(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar.md5").Return(tempMD5File, nil),
		mocks3Downloader.EXPECT().Download(tempMD5File, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().ReadAll(tempMD5File).Return([]byte(expectedMd5Sum), nil),
		mockFS.EXPECT().Remove(tempMD5File.Name()),
		mockFS.EXPECT().TempFile(gomock.Any(), "ecs-agent.tar").Return(tempAgentFile, nil),
		mocks3Downloader.EXPECT().Download(tempAgentFile, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().Open(tempAgentFile.Name()).Return(tarballReader, nil),
		mockFS.EXPECT().Copy(gomock.Any(), tarballReader).Do(func(writer io.Writer, reader io.Reader) {
			_, err = io.Copy(writer, reader)
			if err != nil {
				t.Fail()
			}
		}),
		mockFS.EXPECT().Rename(tempAgentFile.Name(), config.AgentTarball()),
		mockFS.EXPECT().Stat(tempAgentFile.Name()).Return(nil, errors.New("temp file has been renamed")),
	)

	d := &Downloader{
		s3downloader: mocks3Downloader,
		fs:           mockFS,
		metadata:     mockMetadata,
		region:       config.DefaultRegionName,
	}

	d.DownloadAgent()
}

func TestLoadDesiredAgentFailOpenDesired(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().Open(config.DesiredImageLocatorFile()).Return(nil, errors.New("test error"))

	d := &Downloader{
		fs: mockFS,
	}

	_, err := d.LoadDesiredAgent()
	if err == nil {
		t.Error("Expected error to be returned but got nil")
	}
}

func TestLoadDesiredAgentFailReadDesired(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().Open(config.DesiredImageLocatorFile()).Return(ioutil.NopCloser(&bytes.Buffer{}), nil)

	d := &Downloader{
		fs: mockFS,
	}

	_, err := d.LoadDesiredAgent()
	if err == nil {
		t.Error("Expected error to be returned but got nil")
	}
}

func TestRecordCachedAgent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().WriteFile(config.CacheState(), []byte("1"), os.FileMode(orwPerm))

	d := &Downloader{
		fs: mockFS,
	}
	d.RecordCachedAgent()
}

func TestLoadDesiredAgent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	desiredImage := "my-new-agent-image"

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().Open(config.DesiredImageLocatorFile()).Return(ioutil.NopCloser(bytes.NewBufferString(desiredImage+"\n")), nil)
	mockFS.EXPECT().Base(gomock.Any()).Return(desiredImage + "\n")
	mockFS.EXPECT().Open(config.CacheDirectory() + "/" + desiredImage)

	d := &Downloader{
		fs: mockFS,
	}

	d.LoadDesiredAgent()
}
