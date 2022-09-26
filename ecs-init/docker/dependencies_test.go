//go:build test
// +build test

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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const immediately = time.Duration(-1)

var netError = &url.Error{Err: &net.OpError{Op: "read", Net: "unix", Err: io.EOF}}
var httpError = &docker.Error{Status: http.StatusInternalServerError, Message: "error"}

func TestIsNetworkErrorReturnsTrue(t *testing.T) {
	assert.True(t, isNetworkError(netError), "Expect isNetworkError to return true if network error passed in")
}

func TestIsNetworkErrorReturnsFalse(t *testing.T) {
	assert.False(t, isNetworkError(fmt.Errorf("error")), "Expect isNetworkError to be false when non-network error passed in")
}

func TestIsRetryablePingErrorReturnsTrue(t *testing.T) {
	assert.True(t, isRetryablePingError(httpError), "Expect RetryablePingError to be true if httpError passed in")
}

func TestIsRetryablePingErrorReturnsFalse(t *testing.T) {
	assert.False(t, isRetryablePingError(fmt.Errorf("error")), "Expect RetryablePingError to be true if non-http error passed in")
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
	assert.NoError(t, err, "Expect no error for creating docker client with retry on network error")
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
	assert.NoError(t, err, "Expect no error for creating docker client with retry on HTTP status not OK")
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
	assert.Error(t, err, "Expect error when creating docker client with no retry")
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
	assert.Error(t, err, "Expect error when creating docker client with no retry")
}

func TestNewDockerClientGivesUpRetryingOnUnavailableSocket(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClientFactory := NewMockdockerClientFactory(ctrl)
	mockBackoff := NewMockBackoff(ctrl)

	realDockerClient, dockerClientErr := godockerClientFactory{}.NewVersionedClient("unix:///a/bad/docker.sock", dockerClientAPIVersion)

	require.False(t, dockerClientErr != nil, "there should be no errors trying to set up a docker client (intentionally bad with nonexistent socket path")

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
	require.Error(t, err, "expect an error when creating docker client")

	// We expect that the error will be a net.OpError wrapped by a
	// url.Error.
	_, isExpectedError := err.(*url.Error)
	assert.True(t, isExpectedError, "expect net.OpError wrapped by url.Error")
}
