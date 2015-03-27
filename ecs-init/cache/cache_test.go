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

	"code.google.com/p/gomock/gomock"
	"github.com/aws/amazon-ecs-init/ecs-init/config"
)

func TestIsAgentCachedFalse(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().Open(config.AgentTarball()).Return(nil, errors.New("test error"))

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

	mockFS.EXPECT().Open(config.AgentTarball()).Return(&os.File{}, nil)

	d := &Downloader{
		fs: mockFS,
	}

	if !d.IsAgentCached() {
		t.Error("Expected d.IsAgentCached() to be true")
	}
}

func TestIsAgentLatestCannotOpen(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)

	mockFS.EXPECT().Open(config.AgentTarball()).Return(nil, errors.New("test error"))

	d := &Downloader{
		fs: mockFS,
	}

	if d.IsAgentLatest() {
		t.Error("Expected d.IsAgentLatest() to be false")
	}
}

func TestIsAgentLatestDownloadMD5Failure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)

	mockFS.EXPECT().Open(config.AgentTarball()).Return(&os.File{}, nil)
	mockFS.EXPECT().Copy(gomock.Any(), gomock.Any())
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(nil, errors.New("test error"))

	d := &Downloader{
		fs:     mockFS,
		getter: mockGetter,
	}

	if d.IsAgentLatest() {
		t.Error("Expected d.IsAgentLatest() to be false")
	}
}

func TestIsAgentLatestMD5Mismatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)

	mockFS.EXPECT().Open(config.AgentTarball()).Return(&os.File{}, nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockFS.EXPECT().Copy(gomock.Any(), gomock.Any())

	d := &Downloader{
		fs:     mockFS,
		getter: mockGetter,
	}

	if d.IsAgentLatest() {
		t.Error("Expected d.IsAgentLatest() to be false")
	}
}

func TestIsAgentLatestTrue(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	agentContent := "agent content"
	md5sum := fmt.Sprintf("%x\n", md5.Sum([]byte(agentContent)))
	md5response := &http.Response{}

	mockFS := NewMockfileSystem(mockCtrl)
	mockGetter := NewMockhttpGetter(mockCtrl)

	mockFS.EXPECT().Open(config.AgentTarball()).Return(&os.File{}, nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockFS.EXPECT().Copy(gomock.Any(), gomock.Any()).Do(func(writer io.Writer, reader io.Reader) {
		_, err := io.Copy(writer, bytes.NewBufferString(agentContent))
		if err != nil {
			t.Fail()
		}
	})

	d := &Downloader{
		fs:     mockFS,
		getter: mockGetter,
	}

	if !d.IsAgentLatest() {
		t.Error("Expected d.IsAgentLatest() to be true")
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

	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(nil, errors.New("test error"))

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
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
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return(nil, errors.New("test error"))

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
	}

	d.DownloadAgent()
}

func TestDownloadAgentDownloadTarballFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	md5response := &http.Response{}
	md5sum := "md5sum"

	mockFS := NewMockfileSystem(mockCtrl)
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarball).Return(nil, errors.New("test error"))

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
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
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarball).Return(tarballResponse, nil)

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
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
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarball).Return(tarballResponse, nil)
	mockFS.EXPECT().TempFile("", "ecs-agent.tar").Return(nil, errors.New("test error"))

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
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
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarball).Return(tarballResponse, nil)
	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()
	mockFS.EXPECT().TempFile("", "ecs-agent.tar").Return(tempfile, nil)
	mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any())
	mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), errors.New("test error"))
	mockFS.EXPECT().Remove(tempfile.Name())

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
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
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(md5sum), nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarball).Return(tarballResponse, nil)
	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()
	mockFS.EXPECT().TempFile("", "ecs-agent.tar").Return(tempfile, nil)
	mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any())
	mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), nil)
	mockFS.EXPECT().Remove(tempfile.Name())

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
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

	mockFS := NewMockfileSystem(mockCtrl)
	mockFS.EXPECT().MkdirAll(config.CacheDirectory(), os.ModeDir|0700)
	mockGetter := NewMockhttpGetter(mockCtrl)
	mockGetter.EXPECT().Get(config.AgentRemoteTarballMD5).Return(md5response, nil)
	mockFS.EXPECT().ReadAll(md5response.Body).Return([]byte(expectedMd5Sum), nil)
	mockGetter.EXPECT().Get(config.AgentRemoteTarball).Return(tarballResponse, nil)
	tempfile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fail()
	}
	defer tempfile.Close()
	mockFS.EXPECT().TempFile("", "ecs-agent.tar").Return(tempfile, nil)
	mockFS.EXPECT().TeeReader(tarballResponse.Body, gomock.Any()).Do(func(reader io.Reader, writer io.Writer) {
		_, err = io.Copy(writer, reader)
		if err != nil {
			t.Fail()
		}
	})
	mockFS.EXPECT().Copy(tempfile, gomock.Any()).Return(int64(0), nil)
	mockFS.EXPECT().Rename(tempfile.Name(), config.AgentTarball())

	d := &Downloader{
		getter: mockGetter,
		fs:     mockFS,
	}

	d.DownloadAgent()
}
