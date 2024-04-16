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
package session

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"testing"
	"time"

	acsclient "github.com/aws/amazon-ecs-agent/ecs-agent/acs/client"
	mock_session "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	mock_ecs "github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/mocks"
	rolecredentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_credentials "github.com/aws/amazon-ecs-agent/ecs-agent/credentials/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	metricsfactory "github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	mock_retry "github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry/mock"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	mock_wsclient "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// samplePayloadMessage, sampleRefreshCredentialsMessage, and sampleAttachResourceMessage are required to be type
// string because the channel used by the test fake ACS server to read in messages in the tests where these consts
// are used is a string channel (i.e., not a channel of any particular message type).
const (
	samplePayloadMessage = `
{
  "type": "PayloadMessage",
  "message": {
    "messageId": "123",
    "tasks": [
      {
        "taskDefinitionAccountId": "123",
        "containers": [
          {
            "environment": {},
            "name": "name",
            "cpu": 1,
            "essential": true,
            "memory": 1,
            "portMappings": [],
            "overrides": "{}",
            "image": "i",
            "mountPoints": [],
            "volumesFrom": []
          }
        ],
        "elasticNetworkInterfaces":[{
                "attachmentArn": "eni_attach_arn",
                "ec2Id": "eni_id",
                "ipv4Addresses":[{
                    "primary": true,
                    "privateAddress": "ipv4"
                }],
                "ipv6Addresses": [{
                    "address": "ipv6"
                }],
                "subnetGatewayIpv4Address": "ipv4/20",
                "macAddress": "mac"
        }],
        "roleCredentials": {
          "credentialsId": "credsId",
          "accessKeyId": "accessKeyId",
          "expiration": "2016-03-25T06:17:19.318+0000",
          "roleArn": "r1",
          "secretAccessKey": "secretAccessKey",
          "sessionToken": "token"
        },
        "version": "3",
        "volumes": [],
        "family": "f",
        "arn": "arn",
        "desiredStatus": "RUNNING"
      }
    ],
    "generatedAt": 1,
    "clusterArn": "1",
    "containerInstanceArn": "1",
    "seqNum": 1
  }
}
`
	sampleRefreshCredentialsMessage = `
{
  "type": "IAMRoleCredentialsMessage",
  "message": {
    "messageId": "123",
    "clusterArn": "default",
    "taskArn": "t1",
    "roleType": "TaskApplication",
    "roleCredentials": {
      "credentialsId": "credsId",
      "accessKeyId": "newakid",
      "expiration": "later",
      "roleArn": "r1",
      "secretAccessKey": "newskid",
      "sessionToken": "newstkn"
    }
  }
}
`
	sampleAttachResourceMessage = `
{
  "type": "ConfirmAttachmentMessage",
  "message": {
    "messageId": "123",
    "clusterArn": "arn:aws:ecs:us-west-2:123456789012:cluster/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE",
    "containerInstanceArn": "arn:aws:ecs:us-west-2:123456789012:container-instance/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE",
    "taskArn": "arn:aws:ecs:us-west-2:1234567890:task/test-cluster/abc",
    "waitTimeoutMs": 1000,
    "attachment": {
      "attachmentArn": "arn:aws:ecs:us-west-2:123456789012:ephemeral-storage/a1b2c3d4-5678-90ab-cdef-11111EXAMPLE",
      "attachmentProperties": [
        {
          "name": "resourceID",
          "value": "id1"
        },
        {
          "name": "volumeID",
          "value": "id1"
        },
        {
          "name": "volumeSizeInGiB",
          "value": "size1"
        },
        {
          "name": "requestedSizeInGiB",
          "value": "size1"
        },
        {
          "name": "resourceType",
          "value": "EphemeralStorage"
        },
        {
          "name": "deviceName",
          "value": "device1"
        }
      ]
    }
  }
}
`
	agentVersion      = "1.23.4"
	agentGitShortHash = "ffffffff"
	dockerVersion     = "1.2.3"
	acsURL            = "http://endpoint.tld"
)

var inactiveInstanceError = errors.New("InactiveInstanceException")
var noopFunc = func() {}
var testCreds = credentials.NewStaticCredentials("test-id", "test-secret", "test-token")
var testMinAgentConfig = &wsclient.WSClientMinAgentConfig{
	AcceptInsecureCert: true,
	AWSRegion:          "us-west-2",
	DockerEndpoint:     "unix:///var/run/docker.sock",
	IsDocker:           true,
}

// TestACSURL tests that the URL is constructed correctly when connecting to ACS.
func TestACSURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	acsSession := session{
		sendCredentials:      true,
		containerInstanceARN: testconst.ContainerInstanceARN,
		cluster:              testconst.ClusterARN,
		agentVersion:         agentVersion,
		agentHash:            agentGitShortHash,
		dockerVersion:        dockerVersion,
	}
	wsurl := acsSession.acsURL(acsURL)

	parsed, err := url.Parse(wsurl)
	assert.NoError(t, err, "should be able to parse URL")
	assert.Equal(t, "/ws", parsed.Path, "wrong path")
	assert.Equal(t, testconst.ClusterARN, parsed.Query().Get("clusterArn"), "wrong cluster")
	assert.Equal(t, testconst.ContainerInstanceARN, parsed.Query().Get("containerInstanceArn"),
		"wrong container instance")
	assert.Equal(t, agentVersion, parsed.Query().Get("agentVersion"), "wrong agent version")
	assert.Equal(t, agentGitShortHash, parsed.Query().Get("agentHash"), "wrong agent hash")
	assert.Equal(t, "DockerVersion: "+dockerVersion, parsed.Query().Get("dockerVersion"), "wrong docker version")
	assert.Equal(t, "true", parsed.Query().Get("sendCredentials"),
		"wrong sendCredentials value, it should be true on the first connection to ACS")
	assert.Equal(t, "1", parsed.Query().Get("seqNum"), "wrong seqNum")
	protocolVersion, _ := strconv.Atoi(parsed.Query().Get("protocolVersion"))
	assert.True(t, protocolVersion > 1, "ACS protocol version should be greater than 1")
}

// TestSessionReconnectsOnConnectErrors tests that Session retries reconnecting
// to establish the session with ACS when ClientServer.Connect() returns errors.
func TestSessionReconnectsOnConnectErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Serve(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		// Connect fails 10 times.
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, io.EOF).Times(10),
		// Cancel trying to connect to ACS on the 11th attempt.
		// Failure to retry on Connect() errors should cause the test to time out as the context is never canceled.
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			cancel()
		}).Return(time.NewTimer(wsclient.DisconnectTimeout), nil).MinTimes(1),
	)
	acsSession := session{
		containerInstanceARN: testconst.ContainerInstanceARN,
		ecsClient:            ecsClient,
		clientFactory:        mockClientFactory,
		heartbeatTimeout:     20 * time.Millisecond,
		heartbeatJitter:      10 * time.Millisecond,
		disconnectTimeout:    30 * time.Millisecond,
		disconnectJitter:     10 * time.Millisecond,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestIsInactiveInstanceErrorReturnsTrueForInactiveInstance tests that 'InactiveInstance'
// exception is identified correctly.
func TestIsInactiveInstanceErrorReturnsTrueForInactiveInstance(t *testing.T) {
	assert.True(t, isInactiveInstanceError(inactiveInstanceError),
		"inactive instance exception message parsed incorrectly")
}

// TestIsInactiveInstanceErrorReturnsFalseForActiveInstance tests that non-'InactiveInstance'
// exceptions are identified correctly.
func TestIsInactiveInstanceErrorReturnsFalseForActiveInstance(t *testing.T) {
	assert.False(t, isInactiveInstanceError(io.EOF),
		"inactive instance exception message parsed incorrectly")
}

// TestReconnectDelayForInactiveInstance tests that the reconnect delay is computed
// correctly for an inactive instance.
func TestReconnectDelayForInactiveInstance(t *testing.T) {
	acsSession := session{
		inactiveInstanceReconnectDelay: inactiveInstanceReconnectDelay,
		inactiveInstanceCB:             noopFunc,
	}
	delay, ok := acsSession.reconnectDelay(inactiveInstanceError)
	assert.True(t, ok, "Delaying reconnect should be OK for inactive instance")
	assert.Equal(t, inactiveInstanceReconnectDelay, delay,
		"Reconnect delay doesn't match expected value for inactive instance")
}

// TestReconnectDelayForActiveInstanceOnUnexpectedDisconnect tests that the reconnect delay is computed
// correctly for an active instance.
func TestReconnectDelayForActiveInstanceOnUnexpectedDisconnect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackoff := mock_retry.NewMockBackoff(ctrl)
	mockBackoff.EXPECT().Duration().Return(connectionBackoffMax)

	acsSession := session{backoff: mockBackoff}
	delay, ok := acsSession.reconnectDelay(fmt.Errorf("unexpcted disconnect error"))

	assert.True(t, ok, "Delaying reconnect should be OK for active instance on unexpected disconnect")
	assert.Equal(t, connectionBackoffMax, delay,
		"Reconnect delay doesn't match expected value for active instance on unexpected disconnect")
}

// TestWaitForDurationReturnsTrueWhenContextNotCanceled tests that the waitForDuration function behaves correctly when
// the passed in context is not canceled.
func TestWaitForDurationReturnsTrueWhenContextNotCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.True(t, waitForDuration(ctx, time.Millisecond),
		"waitForDuration should return true when uninterrupted")
}

// TestWaitForDurationReturnsFalseWhenContextCanceled tests that the waitForDuration function behaves correctly when
// the passed in context is canceled.
func TestWaitForDurationReturnsFalseWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, waitForDuration(ctx, time.Millisecond),
		"waitForDuration should return false when interrupted")
}

func TestShouldReconnectWithoutBackoffReturnsTrueForEOF(t *testing.T) {
	assert.True(t, shouldReconnectWithoutBackoff(io.EOF),
		"Reconnect without backoff should return true when connection is closed")
}

func TestShouldReconnectWithoutBackoffReturnsFalseForNonEOF(t *testing.T) {
	assert.False(t, shouldReconnectWithoutBackoff(fmt.Errorf("not EOF")),
		"Reconnect without backoff should return false for non io.EOF error")
}

// TestSessionReconnectsWithoutBackoffOnEOFError tests that the Session reconnects
// to ACS without any delay when the connection is closed with the io.EOF error.
func TestSessionReconnectsWithoutBackoffOnEOFError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	mockBackoff := mock_retry.NewMockBackoff(ctrl)
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, io.EOF),
		// The backoff.Reset() method is expected to be invoked when the connection is closed with io.EOF.
		mockBackoff.EXPECT().Reset(),
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			// Cancel the context on the 2nd connect attempt, which should stop the test.
			cancel()
		}).Return(nil, io.EOF),
		mockBackoff.EXPECT().Reset().AnyTimes(),
	)
	acsSession := session{
		containerInstanceARN:           testconst.ContainerInstanceARN,
		ecsClient:                      ecsClient,
		inactiveInstanceCB:             noopFunc,
		backoff:                        mockBackoff,
		clientFactory:                  mockClientFactory,
		heartbeatTimeout:               20 * time.Millisecond,
		heartbeatJitter:                10 * time.Millisecond,
		disconnectTimeout:              30 * time.Millisecond,
		disconnectJitter:               10 * time.Millisecond,
		inactiveInstanceReconnectDelay: inactiveInstanceReconnectDelay,
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestSessionReconnectsWithoutBackoffOnEOFError tests that the Session reconnects
// to ACS after a backoff duration when the connection is closed with non io.EOF error.
func TestSessionReconnectsWithBackoffOnNonEOFError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	mockBackoff := mock_retry.NewMockBackoff(ctrl)
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil,
			fmt.Errorf("not EOF")),
		// The backoff.Duration() method is expected to be invoked when the connection is closed with a non-EOF error
		// code to compute the backoff. Also, no calls to backoff.Reset() are expected in this code path.
		mockBackoff.EXPECT().Duration().Return(time.Millisecond),
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			cancel()
		}).Return(nil, io.EOF),
		mockBackoff.EXPECT().Reset().AnyTimes(),
	)
	acsSession := session{
		containerInstanceARN: testconst.ContainerInstanceARN,
		ecsClient:            ecsClient,
		inactiveInstanceCB:   noopFunc,
		backoff:              mockBackoff,
		clientFactory:        mockClientFactory,
		heartbeatTimeout:     20 * time.Millisecond,
		heartbeatJitter:      10 * time.Millisecond,
		disconnectTimeout:    30 * time.Millisecond,
		disconnectJitter:     10 * time.Millisecond,
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestSessionCallsInactiveInstanceCB tests that the Session calls its inactiveInstanceCB func (which in this test
// generates an event into a deregister instance event stream) when the ACS connection is closed
// with inactive instance error.
func TestSessionCallsInactiveInstanceCB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	deregisterInstanceEventStream := eventstream.NewEventStream("DeregisterContainerInstance", ctx)

	// receiverFunc cancels the context when invoked.
	// Any event on the deregister instance event stream would trigger this.
	receiverFunc := func(...interface{}) error {
		cancel()
		return nil
	}
	err := deregisterInstanceEventStream.Subscribe("DeregisterContainerInstance", receiverFunc)
	assert.NoError(t, err, "Error adding deregister instance event stream subscriber")
	deregisterInstanceEventStream.StartListening()
	inactiveInstanceCB := func() {
		err := deregisterInstanceEventStream.WriteToEventStream(struct{}{})
		assert.NoError(t, err, "Error writing to deregister container instance event stream")
	}
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, inactiveInstanceError)
	inactiveInstanceReconnectDelay := 200 * time.Millisecond
	acsSession := session{
		containerInstanceARN:           testconst.ContainerInstanceARN,
		ecsClient:                      ecsClient,
		inactiveInstanceCB:             inactiveInstanceCB,
		clientFactory:                  mockClientFactory,
		heartbeatTimeout:               20 * time.Millisecond,
		heartbeatJitter:                10 * time.Millisecond,
		disconnectTimeout:              30 * time.Millisecond,
		disconnectJitter:               10 * time.Millisecond,
		inactiveInstanceReconnectDelay: inactiveInstanceReconnectDelay,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	err = acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestSessionReconnectDelayForInactiveInstanceError tests that the Session applies the proper reconnect delay with ACS
// when ClientServer.Connect() returns the InstanceInactive error.
func TestSessionReconnectDelayForInactiveInstanceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	var firstConnectionAttemptTime time.Time
	inactiveInstanceReconnectDelay := 200 * time.Millisecond
	gomock.InOrder(
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			firstConnectionAttemptTime = time.Now()
		}).Return(nil, inactiveInstanceError),
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			reconnectDelay := time.Now().Sub(firstConnectionAttemptTime)
			reconnectDelayTime := time.Now()
			t.Logf("Delay between successive connections: %v", reconnectDelay)
			timeSubFuncSlopAllowed := 2 * time.Millisecond
			if reconnectDelay < inactiveInstanceReconnectDelay {
				// On windows platform, we found issue with time.Now().Sub(...) reporting 199.9989 even
				// after the code has already waited for time.NewTimer(200)ms.
				assert.WithinDuration(t, reconnectDelayTime,
					firstConnectionAttemptTime.Add(inactiveInstanceReconnectDelay), timeSubFuncSlopAllowed)
			}
			cancel()
		}).Return(nil, io.EOF),
	)
	acsSession := session{
		containerInstanceARN:           testconst.ContainerInstanceARN,
		ecsClient:                      ecsClient,
		inactiveInstanceCB:             noopFunc,
		clientFactory:                  mockClientFactory,
		heartbeatTimeout:               20 * time.Millisecond,
		heartbeatJitter:                10 * time.Millisecond,
		disconnectTimeout:              30 * time.Millisecond,
		disconnectJitter:               10 * time.Millisecond,
		inactiveInstanceReconnectDelay: inactiveInstanceReconnectDelay,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestSessionReconnectsOnServeErrors tests that the Session retries to establish the connection with ACS when
// ClientServer.Serve() returns errors.
func TestSessionReconnectsOnServeErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.NewTimer(wsclient.DisconnectTimeout), nil).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		// Serve fails 10 times.
		mockWsClient.EXPECT().Serve(gomock.Any()).Return(io.EOF).Times(10),
		// Cancel trying to Serve ACS requests on the 11th attempt.
		// Failure to retry on Serve() errors should cause the test to time out as the context is never canceled.
		mockWsClient.EXPECT().Serve(gomock.Any()).Do(func(interface{}) {
			cancel()
		}),
	)

	acsSession := session{
		containerInstanceARN: testconst.ContainerInstanceARN,
		ecsClient:            ecsClient,
		inactiveInstanceCB:   noopFunc,
		clientFactory:        mockClientFactory,
		heartbeatTimeout:     20 * time.Millisecond,
		heartbeatJitter:      10 * time.Millisecond,
		disconnectTimeout:    30 * time.Millisecond,
		disconnectJitter:     10 * time.Millisecond,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestSessionStopsWhenContextIsCanceled tests that the Session's Start() method returns
// when its context is canceled.
func TestSessionStopsWhenContextIsCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.NewTimer(wsclient.DisconnectTimeout), nil).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	gomock.InOrder(
		mockWsClient.EXPECT().Serve(gomock.Any()).Return(io.EOF),
		mockWsClient.EXPECT().Serve(gomock.Any()).Do(func(interface{}) {
			cancel()
		}).Return(inactiveInstanceError),
	)
	acsSession := session{
		containerInstanceARN: testconst.ContainerInstanceARN,
		ecsClient:            ecsClient,
		inactiveInstanceCB:   noopFunc,
		clientFactory:        mockClientFactory,
		heartbeatTimeout:     20 * time.Millisecond,
		heartbeatJitter:      10 * time.Millisecond,
		disconnectTimeout:    30 * time.Millisecond,
		disconnectJitter:     10 * time.Millisecond,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestSessionStopsWhenContextIsErrorDueToTimeout tests that Session's Start() method returns
// when its context is in error due to timeout on reconnect delay.
func TestSessionStopsWhenContextIsErrorDueToTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Millisecond)
	defer cancel()

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.NewTimer(wsclient.DisconnectTimeout), nil).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Serve(gomock.Any()).Return(inactiveInstanceError).AnyTimes()

	acsSession := session{
		containerInstanceARN:           testconst.ContainerInstanceARN,
		ecsClient:                      ecsClient,
		inactiveInstanceCB:             noopFunc,
		clientFactory:                  mockClientFactory,
		heartbeatTimeout:               20 * time.Millisecond,
		heartbeatJitter:                10 * time.Millisecond,
		inactiveInstanceReconnectDelay: 1 * time.Hour,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	err := acsSession.Start(ctx)
	assert.Equal(t, context.DeadlineExceeded, err)
}

// TestSessionReconnectsOnDiscoverPollEndpointError tests that the Session retries to establish the connection with ACS
// on DiscoverPollEndpoint errors.
func TestSessionReconnectsOnDiscoverPollEndpointError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Serve(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	// Connect method being called means preceding DiscoverPollEndpoint did not return an error (i.e., was successful).
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
		interface{}, interface{}) {
		// Serve() cancels the context.
		cancel()
	}).Return(time.NewTimer(wsclient.DisconnectTimeout), nil).MinTimes(1)

	gomock.InOrder(
		// DiscoverPollEndpoint returns an error on its first invocation.
		ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return("", fmt.Errorf("oops")),
		// Second invocation returns a success.
		ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil),
	)
	acsSession := session{
		containerInstanceARN: testconst.ContainerInstanceARN,
		ecsClient:            ecsClient,
		inactiveInstanceCB:   noopFunc,
		clientFactory:        mockClientFactory,
		heartbeatTimeout:     20 * time.Millisecond,
		heartbeatJitter:      10 * time.Millisecond,
		disconnectTimeout:    30 * time.Millisecond,
		disconnectJitter:     10 * time.Millisecond,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	start := time.Now()
	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)

	// Measure the duration between retries.
	timeSinceStart := time.Since(start)
	assert.GreaterOrEqual(t, timeSinceStart, connectionBackoffMin,
		"Duration since start is less than minimum threshold for backoff: %s", timeSinceStart.String())

	// The upper limit here should really be connectionBackoffMin + (connectionBackoffMin * jitter),
	// but it can be off by a few milliseconds to account for execution of other instructions.
	// In any case, it should never be higher than 4*connectionBackoffMin.
	assert.LessOrEqual(t, timeSinceStart, 4*connectionBackoffMin,
		"Duration since start is greater than maximum anticipated wait time: %v", timeSinceStart.String())
}

// TestConnectionIsClosedOnIdle tests that the connection to ACS is closed when the connection is idle.
// It also tests whether the session's lastConnectedTime field will be updated properly upon invocation
// of startSessionOnce() and whether the session's GetLastConnectedTime() interface works as expected.
func TestConnectionIsClosedOnIdle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).Do(func(v interface{}) {}).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).Do(func(v interface{}) {}).AnyTimes()
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.NewTimer(wsclient.DisconnectTimeout), nil)
	connectionInactive := false
	mockWsClient.EXPECT().Serve(gomock.Any()).Do(func(interface{}) {
		// Pretend as if the maximum heartbeatTimeout duration has been breached while Serving requests.
		time.Sleep(30 * time.Millisecond)
		connectionInactive = true
	}).Return(io.EOF)
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).MinTimes(1)
	acsSession := session{
		containerInstanceARN: testconst.ContainerInstanceARN,
		ecsClient:            ecsClient,
		inactiveInstanceCB:   noopFunc,
		clientFactory:        mockClientFactory,
		heartbeatTimeout:     20 * time.Millisecond,
		heartbeatJitter:      10 * time.Millisecond,
		disconnectTimeout:    30 * time.Millisecond,
		disconnectJitter:     10 * time.Millisecond,
		backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
			connectionBackoffJitter, connectionBackoffMultiplier),
	}

	// The Session's lastConnectedTime field was initialized with time.Time{}, which is the default zero value for time.Time.
	// At this point, since the Session has not connected to ACS yet, the Session's lastConnectedTime should still be zero.
	assert.True(t, acsSession.GetLastConnectedTime().IsZero())

	// Wait for connection to be closed. If the connection is not closed due to inactivity, the test will time out.
	err := acsSession.startSessionOnce(ctx)
	assert.EqualError(t, err, io.EOF.Error())
	assert.True(t, connectionInactive)

	// The Session's lastConnectedTime field should be updated with the latest connection timestamp after
	// invocation of startSessionOnce(). Check that the field is no longer zero.
	assert.False(t, acsSession.GetLastConnectedTime().IsZero())
}

func TestSessionDoesntLeakGoroutines(t *testing.T) {
	// Skip this test on "windows" platform as we have observed this to
	// fail often after upgrading the windows builds to golang v1.17.
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	payloadMessageHandler := mock_session.NewMockPayloadMessageHandler(ctrl)
	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())

	closeWS := make(chan bool)
	fakeServer, serverIn, requests, errs, err := startFakeACSServer(closeWS)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			select {
			case <-requests:
			case <-errs:
			case <-ctx.Done():
				return
			}
		}
	}()

	timesConnected := 0
	ecsClient.EXPECT().DiscoverPollEndpoint(testconst.ContainerInstanceARN).Return(fakeServer.URL, nil).
		AnyTimes().Do(func(_ interface{}) {
		timesConnected++
	})
	payloadMessageHandler.EXPECT().ProcessMessage(gomock.Any(), gomock.Any()).AnyTimes().
		Do(func(interface{}, interface{}) {
			go func() {
				time.Sleep(5 * time.Millisecond) // do some work
			}()
		})

	emptyDoctor, _ := doctor.NewDoctor([]doctor.Healthcheck{}, testconst.ClusterARN, testconst.ContainerInstanceARN)

	ended := make(chan bool, 1)
	go func() {
		acsSession := session{
			containerInstanceARN:  testconst.ContainerInstanceARN,
			credentialsProvider:   testCreds,
			dockerVersion:         dockerVersion,
			minAgentConfig:        testMinAgentConfig,
			ecsClient:             ecsClient,
			inactiveInstanceCB:    noopFunc,
			clientFactory:         acsclient.NewACSClientFactory(),
			metricsFactory:        metricsfactory.NewNopEntryFactory(),
			payloadMessageHandler: payloadMessageHandler,
			heartbeatTimeout:      1 * time.Second,
			doctor:                emptyDoctor,
			backoff: retry.NewExponentialBackoff(connectionBackoffMin, connectionBackoffMax,
				connectionBackoffJitter, connectionBackoffMultiplier),
		}
		acsSession.Start(ctx)
		ended <- true
	}()
	// Warm it up.
	serverIn <- `{"type":"HeartbeatMessage","message":{"healthy":true,"messageId":"123"}}`
	serverIn <- samplePayloadMessage

	beforeGoroutines := runtime.NumGoroutine()
	for i := 0; i < 40; i++ {
		serverIn <- `{"type":"HeartbeatMessage","message":{"healthy":true,"messageId":"123"}}`
		serverIn <- samplePayloadMessage
		closeWS <- true
	}

	cancel()
	<-ended

	afterGoroutines := runtime.NumGoroutine()

	t.Logf("Goroutines after 1 and after %v acs messages: %v and %v", timesConnected, beforeGoroutines, afterGoroutines)

	if timesConnected < 20 {
		t.Fatal("Expected times connected to be a large number, was ", timesConnected)
	}
	if afterGoroutines > beforeGoroutines+2 {
		t.Error("Goroutine leak, oh no!")
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	}
}

// TestStartSessionHandlesRefreshCredentialsMessages tests the scenario where a refresh credentials message is
// processed immediately on connection establishment with ACS.
func TestStartSessionHandlesRefreshCredentialsMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	credentialsMetadataSetter := mock_session.NewMockCredentialsMetadataSetter(ctrl)
	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	closeWS := make(chan bool)
	fakeServer, serverIn, requestsChan, errChan, err := startFakeACSServer(closeWS)
	if err != nil {
		t.Fatal(err)
	}
	defer close(serverIn)

	go func() {
		for {
			select {
			case <-requestsChan:
				// Cancel the context when we get the ACK request.
				cancel()
			}
		}
	}()

	// DiscoverPollEndpoint returns the URL for the server that we started.
	ecsClient.EXPECT().DiscoverPollEndpoint(testconst.ContainerInstanceARN).Return(fakeServer.URL, nil)

	credentialsManager := mock_credentials.NewMockManager(ctrl)

	ended := make(chan bool, 1)
	go func() {
		acsSession := NewSession(testconst.ContainerInstanceARN,
			testconst.ClusterARN,
			ecsClient,
			testCreds,
			noopFunc,
			acsclient.NewACSClientFactory(),
			metricsfactory.NewNopEntryFactory(),
			agentVersion,
			agentGitShortHash,
			dockerVersion,
			testMinAgentConfig,
			nil,
			credentialsManager,
			credentialsMetadataSetter,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)

		acsSession.Start(ctx)
		// Start should never return unless the context is canceled.
		ended <- true
	}()

	updatedCredentials := rolecredentials.TaskIAMRoleCredentials{}
	credentialsIdInRefreshMessage := "credsId"
	// Ensure that credentials manager interface methods are invoked in the correct order, with expected arguments.
	gomock.InOrder(
		// The last invocation of SetCredentials is to update
		// credentials when a refresh message is received by the handler
		credentialsManager.EXPECT().SetTaskCredentials(gomock.Any()).
			Do(func(creds *rolecredentials.TaskIAMRoleCredentials) {
				updatedCredentials = *creds
				// Validate parsed credentials after the update
				expectedCreds := rolecredentials.TaskIAMRoleCredentials{
					ARN: "t1",
					IAMRoleCredentials: rolecredentials.IAMRoleCredentials{
						RoleArn:         "r1",
						AccessKeyID:     "newakid",
						SecretAccessKey: "newskid",
						SessionToken:    "newstkn",
						Expiration:      "later",
						CredentialsID:   credentialsIdInRefreshMessage,
						RoleType:        rolecredentials.ApplicationRoleType,
					},
				}
				assert.Equal(t, expectedCreds, updatedCredentials, "Mismatch between expected and updated credentials")
			}).Return(nil),
		credentialsMetadataSetter.EXPECT().SetTaskRoleCredentialsMetadata(gomock.Any()).Return(nil),
	)
	serverIn <- sampleRefreshCredentialsMessage

	select {
	case err := <-errChan:
		t.Fatal("Error should not have been returned from server", err)
	case <-ctx.Done():
		// Context is canceled when requestsChan receives an ACK.
	}

	fakeServer.Close()
	// Cancel context should close the session.
	<-ended
}

// TestSessionCorrectlySetsSendCredentials tests that the Session's 'sendCredentials' field
// is set correctly for successive invocations of startSessionOnce.
func TestSessionCorrectlySetsSendCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const numInvocations = 10
	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()
	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Serve(gomock.Any()).Return(io.EOF).AnyTimes()

	acsSession := NewSession(testconst.ContainerInstanceARN,
		testconst.ClusterARN,
		ecsClient,
		nil,
		noopFunc,
		mockClientFactory,
		metricsfactory.NewNopEntryFactory(),
		agentVersion,
		agentGitShortHash,
		dockerVersion,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	acsSession.(*session).heartbeatTimeout = 20 * time.Millisecond
	acsSession.(*session).heartbeatJitter = 10 * time.Millisecond
	acsSession.(*session).disconnectTimeout = 30 * time.Millisecond
	acsSession.(*session).disconnectJitter = 10 * time.Millisecond
	gomock.InOrder(
		// When the websocket client connects to ACS for the first time, 'sendCredentials' should be set to true.
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			assert.Equal(t, true, acsSession.(*session).sendCredentials)
		}).Return(time.NewTimer(wsclient.DisconnectTimeout), nil),
		// For all subsequent connections to ACS, 'sendCredentials' should be set to false.
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			assert.Equal(t, false, acsSession.(*session).sendCredentials)
		}).Return(time.NewTimer(wsclient.DisconnectTimeout), nil).Times(numInvocations-1),
	)

	go func() {
		for i := 0; i < numInvocations; i++ {
			acsSession.(*session).startSessionOnce(ctx)
		}
		cancel()
	}()

	// Wait for context to be canceled.
	select {
	case <-ctx.Done():
	}
}

// TestSessionReconnectCorrectlySetsAcsUrl tests that the ACS URL is set correctly for the Session's initial connection
// and subsequent connections with ACS.
func TestSessionReconnectCorrectlySetsAcsUrl(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()
	ctx, cancel := context.WithCancel(context.Background())

	mockBackoff := mock_retry.NewMockBackoff(ctrl)
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockBackoff.EXPECT().Reset().AnyTimes()
	mockWsClient.EXPECT().WriteCloseMessage().AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Serve(gomock.Any()).Return(io.EOF).AnyTimes()

	// On the initial connection, sendCredentials must be true because Agent forces ACS to send credentials.
	initialAcsURL := fmt.Sprintf(
		"http://endpoint.tld/ws?agentHash=%s&agentVersion=%s&clusterArn=%s&containerInstanceArn=%s&"+
			"dockerVersion=DockerVersion%%3A+%s&protocolVersion=%v&sendCredentials=true&seqNum=1",
		agentGitShortHash, agentVersion, url.QueryEscape(testconst.ClusterARN),
		url.QueryEscape(testconst.ContainerInstanceARN), dockerVersion, acsProtocolVersion)

	// But after that, ACS sends credentials at ACS's own cadence, so sendCredentials must be false.
	subsequentAcsURL := fmt.Sprintf(
		"http://endpoint.tld/ws?agentHash=%s&agentVersion=%s&clusterArn=%s&containerInstanceArn=%s&"+
			"dockerVersion=DockerVersion%%3A+%s&protocolVersion=%v&sendCredentials=false&seqNum=1",
		agentGitShortHash, agentVersion, url.QueryEscape(testconst.ClusterARN),
		url.QueryEscape(testconst.ContainerInstanceARN), dockerVersion, acsProtocolVersion)

	gomock.InOrder(
		mockClientFactory.EXPECT().
			New(initialAcsURL, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(mockWsClient),
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(time.NewTimer(wsclient.DisconnectTimeout), nil),
		mockClientFactory.EXPECT().
			New(subsequentAcsURL, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(mockWsClient),
		mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(interface{},
			interface{}, interface{}) {
			cancel()
		}).Return(time.NewTimer(wsclient.DisconnectTimeout), nil),
	)
	acsSession := NewSession(testconst.ContainerInstanceARN,
		testconst.ClusterARN,
		ecsClient,
		nil,
		noopFunc,
		mockClientFactory,
		metricsfactory.NewNopEntryFactory(),
		agentVersion,
		agentGitShortHash,
		dockerVersion,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	acsSession.(*session).backoff = mockBackoff
	acsSession.(*session).heartbeatTimeout = 20 * time.Millisecond
	acsSession.(*session).heartbeatJitter = 10 * time.Millisecond
	acsSession.(*session).disconnectTimeout = 30 * time.Millisecond
	acsSession.(*session).disconnectJitter = 10 * time.Millisecond

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
}

// TestStartSessionHandlesAttachResourceMessages tests that the Session is able to handle attach
// resource messages when the Session's resourceHandler is not nil and its dockerVersion is "containerd".
func TestStartSessionHandlesAttachResourceMessages(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resourceHandler := mock_session.NewMockResourceHandler(ctrl)
	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	closeWS := make(chan bool)
	fakeServer, serverIn, requestsChan, errChan, err := startFakeACSServer(closeWS)
	if err != nil {
		t.Fatal(err)
	}
	defer close(serverIn)

	go func() {
		for {
			select {
			case <-requestsChan:
				// Cancel the context when we get the ACK request.
				cancel()
			}
		}
	}()

	// DiscoverPollEndpoint returns the URL for the server that we started.
	ecsClient.EXPECT().DiscoverPollEndpoint(testconst.ContainerInstanceARN).Return(fakeServer.URL, nil)

	ended := make(chan bool, 1)
	go func() {
		acsSession := NewSession(testconst.ContainerInstanceARN,
			testconst.ClusterARN,
			ecsClient,
			testCreds,
			noopFunc,
			acsclient.NewACSClientFactory(),
			metricsfactory.NewNopEntryFactory(),
			agentVersion,
			agentGitShortHash,
			"containerd",
			testMinAgentConfig,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			resourceHandler,
			nil,
		)

		acsSession.Start(ctx)
		// Start should never return unless the context is canceled.
		ended <- true
	}()

	// WaitGroup is necessary to wait for HandleResourceAttachment to be called in separate goroutine before exiting
	// the test.
	wg := sync.WaitGroup{}
	wg.Add(1)
	resourceHandler.EXPECT().HandleResourceAttachment(gomock.Any()).Do(func(arg0 interface{}) {
		defer wg.Done() // decrement WaitGroup counter now that HandleResourceAttachment function has been called
	})

	serverIn <- sampleAttachResourceMessage

	select {
	case err := <-errChan:
		t.Fatal("Error should not have been returned from server", err)
	case <-ctx.Done():
		// Context is canceled when requestsChan receives an ACK.
	}

	wg.Wait()

	fakeServer.Close()
	// Cancel context should close the session.
	<-ended
}

// TestSessionCallsAddUpdateRequestHandlers tests that the Session calls the function contained in its struct field
// 'addUpdateRequestHandlers' is called if it is not nil and the Session's dockerVersion is not "containerd".
func TestSessionCallsAddUpdateRequestHandlers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	addUpdateRequestHandlersCalled := false
	addUpdateRequestHandlers := func(cs wsclient.ClientServer) {
		addUpdateRequestHandlersCalled = true
	}

	ecsClient := mock_ecs.NewMockECSClient(ctrl)
	ecsClient.EXPECT().DiscoverPollEndpoint(gomock.Any()).Return(acsURL, nil).AnyTimes()
	ctx, cancel := context.WithCancel(context.Background())

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockClientFactory := mock_wsclient.NewMockClientFactory(ctrl)
	mockClientFactory.EXPECT().
		New(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(mockWsClient).AnyTimes()
	mockWsClient.EXPECT().SetAnyRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().AddRequestHandler(gomock.Any()).AnyTimes()
	mockWsClient.EXPECT().Connect(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.NewTimer(wsclient.DisconnectTimeout), nil).AnyTimes()
	mockWsClient.EXPECT().Serve(gomock.Any()).Do(func(interface{}) {
		if addUpdateRequestHandlersCalled {
			cancel()
		}
	})
	mockWsClient.EXPECT().WriteCloseMessage().Return(nil).AnyTimes()
	mockWsClient.EXPECT().Close().Return(nil).AnyTimes()

	acsSession := NewSession(testconst.ContainerInstanceARN,
		testconst.ClusterARN,
		ecsClient,
		nil,
		noopFunc,
		mockClientFactory,
		metricsfactory.NewNopEntryFactory(),
		agentVersion,
		agentGitShortHash,
		dockerVersion,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		addUpdateRequestHandlers,
	)

	err := acsSession.Start(ctx)
	assert.Equal(t, context.Canceled, err)
	assert.True(t, addUpdateRequestHandlersCalled)
}

func startFakeACSServer(closeWS <-chan bool) (*httptest.Server, chan<- string, <-chan string, <-chan error, error) {
	serverChan := make(chan string, 1)
	requestsChan := make(chan string, 1)
	errChan := make(chan error, 1)

	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)

		if err != nil {
			errChan <- err
		}

		go func() {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
			} else {
				requestsChan <- string(msg)
			}
		}()
		for {
			select {
			case str := <-serverChan:
				err := ws.WriteMessage(websocket.TextMessage, []byte(str))
				if err != nil {
					errChan <- err
				}

			case <-closeWS:
				ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure,
					""))
				ws.Close()
				errChan <- io.EOF
				// Quit listening to serverChan if we've been closed.
				return
			}

		}
	})

	fakeServer := httptest.NewTLSServer(handler)
	return fakeServer, serverChan, requestsChan, errChan, nil
}
