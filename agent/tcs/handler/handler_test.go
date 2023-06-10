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
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"context"

	mock_api "github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/config"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	tcsclient "github.com/aws/amazon-ecs-agent/ecs-agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	wsmock "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/mock/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/docker/docker/api/types"
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
)

type mockStatsEngine struct {
	metricsChannel       chan<- ecstcs.TelemetryMessage
	healthChannel        chan<- ecstcs.HealthMessage
	publishMetricsTicker *time.Ticker
}

var testCreds = credentials.NewStaticCredentials("test-id", "test-secret", "test-token")

var testCfg = &config.Config{
	AcceptInsecureCert: true,
	AWSRegion:          "us-east-1",
}

var emptyDoctor, _ = doctor.NewDoctor([]doctor.Healthcheck{}, "test-cluster", "this:is:an:instance:arn")

func (*mockStatsEngine) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	req := createPublishMetricsRequest()
	return req.Metadata, req.TaskMetrics, nil
}

func (*mockStatsEngine) ContainerDockerStats(taskARN string, id string) (*types.StatsJSON, *stats.NetworkStatsPerSec, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

func (*mockStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*mockStatsEngine) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*mockStatsEngine) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (engine *mockStatsEngine) GetPublishMetricsTicker() *time.Ticker {
	return engine.publishMetricsTicker
}

// SimulateMetricsPublishToChannel simulates the behavior of `StartMetricsPublish` in DockerStatsEngine, which feeds metrics
// to channel to TCS Client. There has to be at least one valid metrics sent, otherwise no request will be made to mockServer
// in TestStartSession, specifically blocking `request := <-requestChan`
func (engine *mockStatsEngine) SimulateMetricsPublishToChannel(ctx context.Context) {
	engine.publishMetricsTicker = time.NewTicker(testPublishMetricsInterval)
	for {
		select {
		case <-engine.publishMetricsTicker.C:
			engine.metricsChannel <- ecstcs.TelemetryMessage{
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

			engine.healthChannel <- ecstcs.HealthMessage{
				Metadata:      &ecstcs.HealthMetadata{},
				HealthMetrics: []*ecstcs.TaskHealth{},
			}

		case <-ctx.Done():
			defer close(engine.metricsChannel)
			defer close(engine.healthChannel)
			return
		}
	}
}

// TestDisableMetrics tests the StartMetricsSession will return immediately if
// the metrics was disabled
func TestDisableMetrics(t *testing.T) {
	params := TelemetrySessionParams{
		Cfg: &config.Config{
			DisableMetrics:           config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
			DisableDockerHealthCheck: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
		},
	}

	StartMetricsSession(&params)
}

func TestFormatURL(t *testing.T) {
	endpoint := "http://127.0.0.0.1/"
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)

	taskEngine.EXPECT().Version().Return("Docker version result", nil)
	wsurl := formatURL(endpoint, testClusterArn, testInstanceArn, taskEngine)
	parsed, err := url.Parse(wsurl)
	assert.NoError(t, err, "should be able to parse URL")
	assert.Equal(t, "/ws", parsed.Path, "wrong path")
	assert.Equal(t, testClusterArn, parsed.Query().Get("cluster"), "wrong cluster")
	assert.Equal(t, testInstanceArn, parsed.Query().Get("containerInstance"), "wrong container instance")
	assert.Equal(t, version.Version, parsed.Query().Get("agentVersion"), "wrong agent version")
	assert.Equal(t, version.GitHashString(), parsed.Query().Get("agentHash"), "wrong agent hash")
	assert.Equal(t, "Docker version result", parsed.Query().Get("dockerVersion"), "wrong docker version")
}

func TestStartSession(t *testing.T) {
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

	mockEngine := &mockStatsEngine{
		metricsChannel: telemetryMessages,
		healthChannel:  healthMessages,
	}

	// Start a session with the test server.
	go startSession(ctx, server.URL, testCfg, testCreds, telemetryMessages, healthMessages,
		defaultHeartbeatTimeout, defaultHeartbeatJitter,
		testPublishMetricsInterval, deregisterInstanceEventStream, emptyDoctor)
	// Wait for 100 ms to make sure the session is ready to receive message from channel
	time.Sleep(testSendMetricsToChannelWaitTime)
	go mockEngine.SimulateMetricsPublishToChannel(ctx)

	// startSession internally starts publishing metrics from the mockStatsEngine object (poll msg out of channel).
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

	// Start a session with the test server.
	err = startSession(ctx, server.URL, testCfg, testCreds, telemetryMessages, healthMessages, defaultHeartbeatTimeout,
		defaultHeartbeatJitter, testPublishMetricsInterval, deregisterInstanceEventStream, emptyDoctor)

	if err == nil {
		t.Error("Expected io.EOF on closed connection")
	}
	if err != io.EOF {
		t.Error("Expected io.EOF on closed connection, got: ", err)
	}
}

// TestConnectionInactiveTimeout tests the tcs client reconnect when it loses network
// connection or it's inactive for too long
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

	// Start a session with the test server.
	err = startSession(ctx, server.URL, testCfg, testCreds, telemetryMessages, healthMessages, 50*time.Millisecond,
		100*time.Millisecond, testPublishMetricsInterval, deregisterInstanceEventStream, emptyDoctor)
	// if we are not blocked here, then the test pass as it will reconnect in StartSession
	assert.NoError(t, err, "Close the connection should cause the tcs client return error")

	assert.True(t, websocket.IsCloseError(<-serverErr, websocket.CloseAbnormalClosure),
		"Read from closed connection should produce an io.EOF error")

	closeSocket(closeWS)
}

func TestDiscoverEndpointAndStartSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEcs := mock_api.NewMockECSClient(ctrl)
	mockEcs.EXPECT().DiscoverTelemetryEndpoint(gomock.Any()).Return("", errors.New("error"))

	err := startTelemetrySession(&TelemetrySessionParams{ECSClient: mockEcs})
	if err == nil {
		t.Error("Expected error from startTelemetrySession when DiscoverTelemetryEndpoint returns error")
	}
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
