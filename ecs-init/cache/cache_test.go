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
	"net/http"
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

func TestDownloadAgentMkdirFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700).Return(errors.New("test error"))

	d := &Downloader{
		fs: mockFS,
	}

	d.DownloadAgent()
}

func TestDownloadAgentDownloadMD5Failure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(nil, errors.New("test error")),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentReadPublishedMd5Failure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{
		Body: ioutil.NopCloser(&bytes.Buffer{}),
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return(nil, errors.New("test error")),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentDownloadTarballFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball("us-east-1")).Return(nil, errors.New("test error")),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentDownloadTarballNon200(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"
	tarballResponse := &http.Response{
		StatusCode: 418,
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball("us-east-1")).Return(tarballResponse, nil),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentTempFile(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"
	tarballResponse := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&bytes.Buffer{}),
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball("us-east-1")).Return(tarballResponse, nil),
		mockFS.EXPECT().TempFile(config.CacheDirectory(), "ecs-agent.tar").Return(nil, errors.New("test error")),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentCopyFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"
	tarballResponse := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&bytes.Buffer{}),
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball("us-east-1")).Return(tarballResponse, nil),
		mockFS.EXPECT().TempFile(config.CacheDirectory(), "ecs-agent.tar").Return(tempfile, nil),
		mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any()),
		mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), errors.New("test error")),
		mockFS.EXPECT().Remove(tempfile.Name()),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentMD5Mismatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"
	tarballResponse := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(&bytes.Buffer{}),
	}

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball("us-east-1")).Return(tarballResponse, nil),
		mockFS.EXPECT().TempFile(config.CacheDirectory(), "ecs-agent.tar").Return(tempfile, nil),
		mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any()),
		mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().Remove(tempfile.Name()),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	tarballContents := "tarball contents"
	tarballResponse := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBufferString(tarballContents)),
	}
	expectedMd5Sum := fmt.Sprintf("%x\n", md5.Sum([]byte(tarballContents)))
	md5response := &http.Response{}

	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("us-east-1", nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5("us-east-1")).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(expectedMd5Sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball("us-east-1")).Return(tarballResponse, nil),
		mockFS.EXPECT().TempFile(config.CacheDirectory(), "ecs-agent.tar").Return(tempfile, nil),
		mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any()).Do(func(reader io.Reader, writer io.Writer) {
			_, err = io.Copy(writer, reader)
			if err != nil {
				t.Fail()
			}
		}),
		mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().Rename(tempfile.Name(), config.AgentTarball()),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
	}

	d.DownloadAgent()
}

func TestDownloadAgentSuccessWithRegionFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	tarballContents := "tarball contents"
	tarballResponse := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewBufferString(tarballContents)),
	}
	expectedMd5Sum := fmt.Sprintf("%x\n", md5.Sum([]byte(tarballContents)))
	md5response := &http.Response{}

	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockMetadata := NewMockinstanceMetadata(mockCtrl)

	gomock.InOrder(
		mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700),
		mockMetadata.EXPECT().Region().Return("", errors.New("test error")),
		mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5(config.RegionNameNotFound)).Return(md5response, nil),
		mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(expectedMd5Sum), nil),
		mockGetter.EXPECT().Get(config.AgentRemoteTarball(config.RegionNameNotFound)).Return(tarballResponse, nil),
		mockFS.EXPECT().TempFile(config.CacheDirectory(), "ecs-agent.tar").Return(tempfile, nil),
		mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any()).Do(func(reader io.Reader, writer io.Writer) {
			_, err = io.Copy(writer, reader)
			if err != nil {
				t.Fail()
			}
		}),
		mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), nil),
		mockFS.EXPECT().Rename(tempfile.Name(), config.AgentTarball()),
	)

	d := &Downloader{
		getter:   mockGetter,
		fs:       mockFS,
		metadata: mockMetadata,
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
