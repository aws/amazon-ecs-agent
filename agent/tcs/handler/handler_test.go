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
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	tcsclient "github.com/aws/amazon-ecs-agent/agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	wsmock "github.com/aws/amazon-ecs-agent/agent/wsclient/mock/utils"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

const (
	testTaskArn                              = "arn:aws:ecs:us-east-1:123:task/def"
	testTaskDefinitionFamily                 = "task-def"
	testClusterArn                           = "arn:aws:ecs:us-east-1:123:cluster/default"
	testInstanceArn                          = "arn:aws:ecs:us-east-1:123:container-instance/abc"
	testMessageId                            = "testMessageId"
	testPublishMetricsInterval               = 1 * time.Millisecond
	testPublishInstanceHealthMetricsInterval = 1 * time.Millisecond
)

type mockStatsEngine struct{}

var testCreds = credentials.NewStaticCredentials("test-id", "test-secret", "test-token")

var testCfg = &config.Config{
	AcceptInsecureCert: true,
	AWSRegion:          "us-east-1",
}

func (*mockStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	req := createPublishMetricsRequest()
	return req.Metadata, req.TaskMetrics, nil
}

func (*mockStatsEngine) ContainerDockerStats(taskARN string, id string) (*types.StatsJSON, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*mockStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*mockStatsEngine) GetInstanceHealthMetadata() *ecstcs.StartTelemetrySessionInput {
	return nil
}

// TestDisableMetrics tests the StartMetricsSession will return immediately if
// the metrics was disabled
func TestDisableMetrics(t *testing.T) {
	params := TelemetrySessionParams{
		Cfg: &config.Config{
			DisableMetrics:           true,
			DisableDockerHealthCheck: true,
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
	// Start a session with the test server.
	go startSession(ctx, server.URL, testCfg, testCreds, &mockStatsEngine{},
		defaultHeartbeatTimeout, defaultHeartbeatJitter,
		testPublishMetricsInterval, testPublishInstanceHealthMetricsInterval,
		deregisterInstanceEventStream)

	// startSession internally starts publishing metrics from the mockStatsEngine object.
	time.Sleep(testPublishMetricsInterval)

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

	// Start a session with the test server.
	err = startSession(ctx, server.URL, testCfg, testCreds, &mockStatsEngine{},
		defaultHeartbeatTimeout, defaultHeartbeatJitter,
		testPublishMetricsInterval, testPublishInstanceHealthMetricsInterval,
		deregisterInstanceEventStream)

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
	// Start a session with the test server.
	err = startSession(ctx, server.URL, testCfg, testCreds, &mockStatsEngine{},
		50*time.Millisecond, 100*time.Millisecond,
		testPublishMetricsInterval, testPublishInstanceHealthMetricsInterval,
		deregisterInstanceEventStream)
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

	err := startTelemetrySession(&TelemetrySessionParams{ECSClient: mockEcs}, nil)
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
