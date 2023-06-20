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

package tcshandler

import (
	"errors"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	tcsclient "github.com/aws/amazon-ecs-agent/ecs-agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	wsmock "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/mock/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

const (
	testTaskArn                           = "arn:aws:ecs:us-east-1:123:task/def"
	testTaskDefinitionFamily              = "task-def"
	testClusterArn                        = "arn:aws:ecs:us-east-1:123:cluster/default"
	testInstanceArn                       = "arn:aws:ecs:us-east-1:123:container-instance/abc"
	testMessageId                         = "testMessageId"
	testPublishMetricsInterval            = 1 * time.Second
	testSendMetricsToChannelWaitTime      = 100 * time.Millisecond
	testTelemetryChannelDefaultBufferSize = 10
	testDockerEndpoint                    = "testEndpoint"
	testAgentVersion                      = "testAgentVersion"
	testAgentHash                         = "testAgentHash"
	testContainerRuntimeVersion           = "testContainerRuntimeVersion"
	testHeartbeatTimeout                  = 1 * time.Minute
	testHeartbeatJitter                   = 1 * time.Minute
	testDisconnectionTimeout              = 15 * time.Minute
	testDisconnectionJitter               = 30 * time.Minute
)

type mockStatsSource struct {
	metricsChannel       chan<- ecstcs.TelemetryMessage
	healthChannel        chan<- ecstcs.HealthMessage
	publishMetricsTicker *time.Ticker
}

var testCreds = credentials.NewStaticCredentials("test-id", "test-secret", "test-token")

var testCfg = &wsclient.WSClientMinAgentConfig{
	AWSRegion:          "us-east-1",
	AcceptInsecureCert: true,
	DockerEndpoint:     testDockerEndpoint,
	IsDocker:           true,
}

var emptyDoctor, _ = doctor.NewDoctor([]doctor.Healthcheck{}, "test-cluster", "this:is:an:instance:arn")

func (*mockStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	req := createPublishMetricsRequest()
	return req.Metadata, req.TaskMetrics, nil
}

func (*mockStatsSource) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*mockStatsSource) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*mockStatsSource) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (Source *mockStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return Source.publishMetricsTicker
}

// SimulateMetricsPublishToChannel simulates the behavior of `StartMetricsPublish` in DockerStatsSource, which feeds metrics
// to channel to TCS Client. There has to be at least one valid metrics sent, otherwise no request will be made to mockServer
// in TestStartTelemetrySession, specifically blocking `request := <-requestChan`
func (Source *mockStatsSource) SimulateMetricsPublishToChannel(ctx context.Context) {
	Source.publishMetricsTicker = time.NewTicker(testPublishMetricsInterval)
	for {
		select {
		case <-Source.publishMetricsTicker.C:
			Source.metricsChannel <- ecstcs.TelemetryMessage{
				Metadata: &ecstcs.MetricsMetadata{
					Cluster:           aws.String(testClusterArn),
					ContainerInstance: aws.String(testInstanceArn),
					Fin:               aws.Bool(false),
					Idle:              aws.Bool(false),
					MessageId:         aws.String(testMessageId),
				},
				TaskMetrics: []*ecstcs.TaskMetric{
					&ecstcs.TaskMetric{},
				},
			}

			Source.healthChannel <- ecstcs.HealthMessage{
				Metadata:      &ecstcs.HealthMetadata{},
				HealthMetrics: []*ecstcs.TaskHealth{},
			}

		case <-ctx.Done():
			defer close(Source.metricsChannel)
			defer close(Source.healthChannel)
			return
		}
	}
}

func TestFormatURL(t *testing.T) {
	endpoint := "http://127.0.0.0.1/"
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wsurl := formatURL(endpoint, testClusterArn, testInstanceArn, testAgentVersion, testAgentHash,
		ContainerRuntimeDocker, testContainerRuntimeVersion)
	parsed, err := url.Parse(wsurl)
	assert.NoError(t, err, "should be able to parse URL")
	assert.Equal(t, "/ws", parsed.Path, "wrong path")
	assert.Equal(t, testClusterArn, parsed.Query().Get("cluster"), "wrong cluster")
	assert.Equal(t, testInstanceArn, parsed.Query().Get("containerInstance"), "wrong container instance")
	assert.Equal(t, testAgentVersion, parsed.Query().Get("agentVersion"), "wrong agent version")
	assert.Equal(t, testAgentHash, parsed.Query().Get("agentHash"), "wrong agent hash")
	assert.Equal(t, testContainerRuntimeVersion, parsed.Query().Get("dockerVersion"), "wrong docker version")
}

func TestStartTelemetrySession(t *testing.T) {
	// Start test server.
	closeWS := make(chan []byte)
	server, serverChan, requestChan, serverErr, err := wsmock.GetMockServer(closeWS)
	server.StartTLS()
	defer server.Close()
	if err != nil {
		t.Fatal(err)
	}

	telemetryMessages := make(chan ecstcs.TelemetryMessage, testTelemetryChannelDefaultBufferSize)
	healthMessages := make(chan ecstcs.HealthMessage, testTelemetryChannelDefaultBufferSize)

	wait := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	wait.Add(1)
	go func() {
		select {
		case sErr := <-serverErr:
			t.Error(sErr)
		case <-ctx.Done():
		}
		wait.Done()
	}()
	defer func() {
		closeSocket(closeWS)
		close(serverChan)
	}()

	deregisterInstanceEventStream := eventstream.NewEventStream("Deregister_Instance", context.Background())

	mockSource := &mockStatsSource{
		metricsChannel: telemetryMessages,
		healthChannel:  healthMessages,
	}

	session := NewTelemetrySession(
		testInstanceArn,
		testClusterArn,
		testAgentVersion,
		testAgentHash,
		testContainerRuntimeVersion,
		server.URL,
		false,
		testCreds,
		testCfg,
		deregisterInstanceEventStream,
		testHeartbeatTimeout,
		testHeartbeatJitter,
		testDisconnectionTimeout,
		testDisconnectionJitter,
		nil,
		telemetryMessages,
		healthMessages,
		emptyDoctor,
	)

	// Start a session with the test server.
	go session.StartTelemetrySession(ctx, server.URL)

	// Wait for 100 ms to make sure the session is ready to receive message from channel
	time.Sleep(testSendMetricsToChannelWaitTime)
	go mockSource.SimulateMetricsPublishToChannel(ctx)

	// startTelemetrySession internally starts publishing metrics from the mockStatsSource object (poll msg out of channel).
	time.Sleep(testPublishMetricsInterval * 2)

	// Read request channel to get the metric data published to the server.
	request := <-requestChan
	cancel()
	wait.Wait()
	go func() {
		for {
			select {
			case <-requestChan:
			}
		}
	}()

	// Decode and verify the metric data.
	payload, err := getPayloadFromRequest(request)
	if err != nil {
		t.Fatal("Error decoding payload: ", err)
	}

	// Decode and verify the metric data.
	_, responseType, err := wsclient.DecodeData([]byte(payload), tcsclient.NewTCSDecoder())
	if err != nil {
		t.Fatal("error decoding data: ", err)
	}
	if responseType != "PublishMetricsRequest" {
		t.Fatal("Unexpected responseType: ", responseType)
	}
}

func TestSessionConnectionClosedByRemote(t *testing.T) {
	// Start test server.
	closeWS := make(chan []byte)
	server, serverChan, _, serverErr, err := wsmock.GetMockServer(closeWS)
	server.StartTLS()
	defer server.Close()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		serr := <-serverErr
		if !websocket.IsCloseError(serr, websocket.CloseNormalClosure) {
			t.Error(serr)
		}
	}()
	sleepBeforeClose := 10 * time.Millisecond
	go func() {
		time.Sleep(sleepBeforeClose)
		closeSocket(closeWS)
		close(serverChan)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	deregisterInstanceEventStream := eventstream.NewEventStream("Deregister_Instance", ctx)
	deregisterInstanceEventStream.StartListening()
	defer cancel()

	telemetryMessages := make(chan ecstcs.TelemetryMessage, testTelemetryChannelDefaultBufferSize)
	healthMessages := make(chan ecstcs.HealthMessage, testTelemetryChannelDefaultBufferSize)

	session := NewTelemetrySession(
		testInstanceArn,
		testClusterArn,
		testAgentVersion,
		testAgentHash,
		testContainerRuntimeVersion,
		server.URL,
		false,
		testCreds,
		testCfg,
		deregisterInstanceEventStream,
		testHeartbeatTimeout,
		testHeartbeatJitter,
		testDisconnectionTimeout,
		testDisconnectionJitter,
		nil,
		telemetryMessages,
		healthMessages,
		emptyDoctor,
	)

	// Start a session with the test server.
	err = session.StartTelemetrySession(ctx, server.URL)

	if err == nil {
		t.Error("Expected io.EOF on closed connection")
	}
	if err != io.EOF {
		t.Error("Expected io.EOF on closed connection, got: ", err)
	}
}

// TestConnectionInactiveTimeout tests the tcs client reconnect when it loses network
// connection, or it's inactive for too long
func TestConnectionInactiveTimeout(t *testing.T) {
	// Start test server.
	closeWS := make(chan []byte)
	server, _, requestChan, serverErr, err := wsmock.GetMockServer(closeWS)
	server.StartTLS()
	defer server.Close()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			select {
			case <-requestChan:
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	deregisterInstanceEventStream := eventstream.NewEventStream("Deregister_Instance", ctx)
	deregisterInstanceEventStream.StartListening()
	defer cancel()

	telemetryMessages := make(chan ecstcs.TelemetryMessage, testTelemetryChannelDefaultBufferSize)
	healthMessages := make(chan ecstcs.HealthMessage, testTelemetryChannelDefaultBufferSize)

	session := NewTelemetrySession(
		testInstanceArn,
		testClusterArn,
		testAgentVersion,
		testAgentHash,
		testContainerRuntimeVersion,
		server.URL,
		false,
		testCreds,
		testCfg,
		deregisterInstanceEventStream,
		50*time.Millisecond,
		100*time.Millisecond,
		testDisconnectionTimeout,
		testDisconnectionJitter,
		nil,
		telemetryMessages,
		healthMessages,
		emptyDoctor,
	)

	// Start a session with the test server.
	err = session.StartTelemetrySession(ctx, server.URL)

	assert.NoError(t, err, "Close the connection should cause the tcs client return error")

	assert.True(t, websocket.IsCloseError(<-serverErr, websocket.CloseAbnormalClosure),
		"Read from closed connection should produce an io.EOF error")

	closeSocket(closeWS)
}

func getPayloadFromRequest(request string) (string, error) {
	lines := strings.Split(request, "\r\n")
	if len(lines) > 0 {
		return lines[len(lines)-1], nil
	}

	return "", errors.New("Could not get payload")
}

// closeSocket tells the server to send a close frame. This lets us test
// what happens if the connection is closed by the remote server.
func closeSocket(ws chan<- []byte) {
	ws <- websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	close(ws)
}

func createPublishMetricsRequest() *ecstcs.PublishMetricsRequest {
	cluster := testClusterArn
	ci := testInstanceArn
	taskArn := testTaskArn
	taskDefinitionFamily := testTaskDefinitionFamily
	var fval float64
	fval = rand.Float64()
	var ival int64
	ival = rand.Int63n(10)
	ts := time.Now()
	idle := false
	messageId := testMessageId
	return &ecstcs.PublishMetricsRequest{
		Metadata: &ecstcs.MetricsMetadata{
			Cluster:           &cluster,
			ContainerInstance: &ci,
			Idle:              &idle,
			MessageId:         &messageId,
		},
		TaskMetrics: []*ecstcs.TaskMetric{
			{
				ContainerMetrics: []*ecstcs.ContainerMetric{
					{
						CpuStatsSet: &ecstcs.CWStatsSet{
							Max:         &fval,
							Min:         &fval,
							SampleCount: &ival,
							Sum:         &fval,
						},
						MemoryStatsSet: &ecstcs.CWStatsSet{
							Max:         &fval,
							Min:         &fval,
							SampleCount: &ival,
							Sum:         &fval,
						},
					},
				},
				TaskArn:              &taskArn,
				TaskDefinitionFamily: &taskDefinitionFamily,
			},
		},
		Timestamp: &ts,
	}
}

func TestStartTelemetrySessionMetricsChannelPauseWhenClientClosed(t *testing.T) {
	telemetryMessages := make(chan ecstcs.TelemetryMessage, testTelemetryChannelDefaultBufferSize)
	healthMessages := make(chan ecstcs.HealthMessage, testTelemetryChannelDefaultBufferSize)

	// Start test server.
	closeWS := make(chan []byte)
	server, _, _, _, _ := wsmock.GetMockServer(closeWS)
	server.StartTLS()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	deregisterInstanceEventStream := eventstream.NewEventStream("Deregister_Instance", context.Background())
	deregisterInstanceEventStream.StartListening()

	session := NewTelemetrySession(
		testInstanceArn,
		testClusterArn,
		testAgentVersion,
		testAgentHash,
		testContainerRuntimeVersion,
		server.URL,
		false,
		testCreds,
		testCfg,
		deregisterInstanceEventStream,
		testHeartbeatTimeout,
		testHeartbeatJitter,
		testDisconnectionTimeout,
		testDisconnectionJitter,
		nil,
		telemetryMessages,
		healthMessages,
		emptyDoctor,
	)

	go session.StartTelemetrySession(ctx, server.URL)
	telemetryMessages <- ecstcs.TelemetryMessage{}
	for len(telemetryMessages) != 0 {
		time.Sleep(1 * time.Second)
	} // wait till tcs client is up and is polling message

	cancel()
	time.Sleep(5 * time.Second) // wait till tcs client is stopped (returned from StartTelemetrySession)

	// Send message while TCS Client is closed and verify the message is not polled but stays in the channel
	for it := 0; it < testTelemetryChannelDefaultBufferSize; it++ {
		telemetryMessages <- ecstcs.TelemetryMessage{}
	}

	// check messages filled the channel
	assert.Len(t, telemetryMessages, testTelemetryChannelDefaultBufferSize)
	// check after channel is full, message will be dropped

	// simulating retry after backoff
	newCtx, _ := context.WithCancel(context.Background())
	go session.StartTelemetrySession(newCtx, server.URL)
	for len(telemetryMessages) != 0 {
		time.Sleep(1 * time.Second)
	} // test will time out if after tcs client startup, message does not resume flowing
}
