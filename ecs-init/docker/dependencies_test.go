// Copyright 2015-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package docker

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
)

const immediately = time.Duration(-1)

var netError = &url.Error{Err: &net.OpError{Op: "read", Net: "unix", Err: io.EOF}}
var httpError = &docker.Error{Status: http.StatusInternalServerError, Message: "error"}

func TestIsNetworkErrorReturnsTrue(t *testing.T) {
	if !isNetworkError(netError) {
		t.Errorf("Expected true when checking if network error")
	}
}

func TestIsNetworkErrorReturnsFalse(t *testing.T) {
	if isNetworkError(fmt.Errorf("error")) {
		t.Errorf("Expected false when checking if network error")
	}
}

func TestIsRetryablePingErrorReturnsTrue(t *testing.T) {
	if !isRetryablePingError(httpError) {
		t.Errorf("Expected true when checking if retryable ping error")
	}
}

func TestIsRetryablePingErrorReturnsFalse(t *testing.T) {
	if isRetryablePingError(fmt.Errorf("error")) {
		t.Errorf("Expected false when checking if retryable ping error")
	}
}

func TestNewDockerClientRetriesOnPingNetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := NewMockdockerclient(ctrl)
	mockClientFactory := NewMockdockerClientFactory(ctrl)
	mockBackoff := NewMockBackoff(ctrl)

	gomock.InOrder(
		mockClientFactory.EXPECT().NewVersionedClient(gomock.Any(), gomock.Any()).Return(mockDockerClient, nil),
		mockDockerClient.EXPECT().Ping().Return(netError),
		mockBackoff.EXPECT().ShouldRetry().Return(true),
		mockBackoff.EXPECT().Duration().Return(immediately),
		mockDockerClient.EXPECT().Ping().Return(nil),
	)

	_, err := newDockerClient(mockClientFactory, mockBackoff)
	if err != nil {
		t.Error("Error creating docker client")
	}
}

func TestNewDockerClientRetriesOnHTTPStatusNotOKError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := NewMockdockerclient(ctrl)
	mockClientFactory := NewMockdockerClientFactory(ctrl)
	mockBackoff := NewMockBackoff(ctrl)

	gomock.InOrder(
		mockClientFactory.EXPECT().NewVersionedClient(gomock.Any(), gomock.Any()).Return(mockDockerClient, nil),
		mockDockerClient.EXPECT().Ping().Return(httpError),
		mockBackoff.EXPECT().ShouldRetry().Return(true),
		mockBackoff.EXPECT().Duration().Return(immediately),
		mockDockerClient.EXPECT().Ping().Return(nil),
	)

	_, err := newDockerClient(mockClientFactory, mockBackoff)
	if err != nil {
		t.Error("Error creating docker client")
	}
}

func TestNewDockerClientDoesnotRetryOnPingNonNetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := NewMockdockerclient(ctrl)
	mockClientFactory := NewMockdockerClientFactory(ctrl)
	mockBackoff := NewMockBackoff(ctrl)

	gomock.InOrder(
		mockClientFactory.EXPECT().NewVersionedClient(gomock.Any(), gomock.Any()).Return(mockDockerClient, nil),
		mockDockerClient.EXPECT().Ping().Return(fmt.Errorf("error")),
	)

	_, err := newDockerClient(mockClientFactory, mockBackoff)
	if err == nil {
		t.Error("Expected error creating docker client")
	}
}

func TestNewDockerClientGivesUpRetryingOnPingNetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDockerClient := NewMockdockerclient(ctrl)
	mockClientFactory := NewMockdockerClientFactory(ctrl)
	mockBackoff := NewMockBackoff(ctrl)

	gomock.InOrder(
		mockClientFactory.EXPECT().NewVersionedClient(gomock.Any(), gomock.Any()).Return(mockDockerClient, nil),
		mockDockerClient.EXPECT().Ping().Return(netError),
		mockBackoff.EXPECT().ShouldRetry().Return(true),
		mockBackoff.EXPECT().Duration().Return(immediately),
		mockDockerClient.EXPECT().Ping().Return(netError),
		mockBackoff.EXPECT().ShouldRetry().Return(false),
	)

	_, err := newDockerClient(mockClientFactory, mockBackoff)
	if err == nil {
		t.Error("Expected error creating docker client")
	}
}

func TestNewDockerClientGivesUpRetryingOnUnavailableSocket(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClientFactory := NewMockdockerClientFactory(ctrl)
	mockBackoff := NewMockBackoff(ctrl)

	realDockerClient, dockerClientErr := godockerClientFactory{}.NewVersionedClient("unix:///a/bad/docker.sock", dockerClientAPIVersion)
	if dockerClientErr != nil {
		t.Fatal("error setting up intentionally bad client (nonexistent socket path)")
	}

	gomock.InOrder(
		// We use the real client to ensure we're classifying errors
		// correctly as returned by the upstream's error handling.
		mockClientFactory.EXPECT().NewVersionedClient(gomock.Any(), gomock.Any()).Return(realDockerClient, dockerClientErr),
		// "bad connection", retries
		mockBackoff.EXPECT().ShouldRetry().Return(true),
		mockBackoff.EXPECT().Duration().Return(immediately),
		// Give up
		mockBackoff.EXPECT().ShouldRetry().Return(false),
	)

	_, err := newDockerClient(mockClientFactory, mockBackoff)
	if err == nil {
		t.Fatal("Expected error creating docker client")
	}

	// We expect that the error will be a net.OpError wrapped by a
	// url.Error.
	_, isExpectedError := err.(*url.Error)
	if !isExpectedError {
		t.Fatal("Unexpected error type from docker client given a nonexistent path")
	}
}
