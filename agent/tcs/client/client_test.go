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

// Package tcsclient wraps the generated aws-sdk-go client to provide marshalling
// and unmarshalling of data over a websocket connection in the format expected
// by TCS. It allows for bidirectional communication and acts as both a
// client-and-server in terms of requests, but only as a client in terms of
// connecting.

package tcsclient

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/metrics/instancehealth"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	mock_stats "github.com/aws/amazon-ecs-agent/agent/stats/mock"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	mock_wsconn "github.com/aws/amazon-ecs-agent/agent/wsclient/wsconn/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPublishMetricsInterval               = 1 * time.Second
	testPublishInstanceHealthMetricsInterval = 1 * time.Second
	testMessageId                            = "testMessageId"
	testCluster                              = "default"
	testContainerInstance                    = "containerInstance"
	rwTimeout                                = time.Second
)

var testCreds = credentials.NewStaticCredentials("test-id", "test-secret", "test-token")

type mockStatsEngine struct{}

func (*mockStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("uninitialized")
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

type emptyStatsEngine struct{}

func (*emptyStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("empty stats")
}

func (*emptyStatsEngine) ContainerDockerStats(taskARN string, id string) (*types.StatsJSON, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*emptyStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*emptyStatsEngine) GetInstanceHealthMetadata() *ecstcs.StartTelemetrySessionInput {
	return nil
}

type idleStatsEngine struct{}

func (*idleStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(testCluster),
		ContainerInstance: aws.String(testContainerInstance),
		Idle:              aws.Bool(true),
		MessageId:         aws.String(testMessageId),
	}
	return metadata, []*ecstcs.TaskMetric{}, nil
}

func (*idleStatsEngine) ContainerDockerStats(taskARN string, id string) (*types.StatsJSON, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*idleStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*idleStatsEngine) GetInstanceHealthMetadata() *ecstcs.StartTelemetrySessionInput {
	return nil
}

type nonIdleStatsEngine struct {
	numTasks int
}

// instancehealthMu is a lock to make sure that two tests relying on the instancehealth.DockerMetric
// global do not execute in parallel, which could mess with the behavior that they
// are expecting.
var instancehealthMu sync.Mutex

func (engine *nonIdleStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(testCluster),
		ContainerInstance: aws.String(testContainerInstance),
		Idle:              aws.Bool(false),
		MessageId:         aws.String(testMessageId),
	}
	var taskMetrics []*ecstcs.TaskMetric
	var i int64
	for i = 0; int(i) < engine.numTasks; i++ {
		taskArn := "task/" + strconv.FormatInt(i, 10)
		taskMetrics = append(taskMetrics, &ecstcs.TaskMetric{TaskArn: &taskArn})
	}
	return metadata, taskMetrics, nil
}

func (*nonIdleStatsEngine) ContainerDockerStats(taskARN string, id string) (*types.StatsJSON, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*nonIdleStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*nonIdleStatsEngine) GetInstanceHealthMetadata() *ecstcs.StartTelemetrySessionInput {
	return nil
}

func newNonIdleStatsEngine(numTasks int) *nonIdleStatsEngine {
	return &nonIdleStatsEngine{numTasks: numTasks}
}

func TestPayloadHandlerCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn)

	// Messages should be read from the connection at least once
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Return(1,
		[]byte(`{"type":"AckPublishMetric","message":{}}`), nil).MinTimes(1)
	// Invoked when closing the connection
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().Close()

	handledPayload := make(chan *ecstcs.AckPublishMetric)

	reqHandler := func(payload *ecstcs.AckPublishMetric) {
		handledPayload <- payload
	}
	cs.AddRequestHandler(reqHandler)

	go cs.Serve()
	defer cs.Close()

	t.Log("Waiting for handler to return payload.")
	<-handledPayload
}

func TestPublishMetricsRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	// Invoked when closing the connection
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil).Times(2)
	conn.EXPECT().Close()
	// TODO: should use explicit values
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any())

	cs := testCS(conn)
	defer cs.Close()
	err := cs.MakeRequest(&ecstcs.PublishMetricsRequest{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublishMetricsOnceEmptyStatsError(t *testing.T) {
	cs := clientServer{
		statsEngine: &emptyStatsEngine{},
	}
	err := cs.publishMetricsOnce()

	assert.Error(t, err, "Failed: expecting publishMerticOnce return err ")
}

func TestPublishOnceIdleStatsEngine(t *testing.T) {
	cs := clientServer{
		statsEngine: &idleStatsEngine{},
	}
	requests, err := cs.metricsToPublishMetricRequests()
	if err != nil {
		t.Fatal("Error creating publishmetricrequests: ", err)
	}
	if len(requests) != 1 {
		t.Errorf("Expected %d requests, got %d", 1, len(requests))
	}
	lastRequest := requests[0]
	if !*lastRequest.Metadata.Fin {
		t.Error("Fin not set to true in Last request")
	}
}

func TestPublishOnceNonIdleStatsEngine(t *testing.T) {
	expectedRequests := 3
	// Cretes 21 task metrics, which translate to 3 batches,
	// {[Task1, Task2, ...Task10], [Task11, Task12, ...Task20], [Task21]}
	numTasks := (tasksInMetricMessage * (expectedRequests - 1)) + 1
	cs := clientServer{
		statsEngine: newNonIdleStatsEngine(numTasks),
	}
	requests, err := cs.metricsToPublishMetricRequests()
	if err != nil {
		t.Fatal("Error creating publishmetricrequests: ", err)
	}
	taskArns := make(map[string]bool)
	for _, request := range requests {
		for _, taskMetric := range request.TaskMetrics {
			_, exists := taskArns[*taskMetric.TaskArn]
			if exists {
				t.Fatal("Duplicate task arn in requests: ", *taskMetric.TaskArn)
			}
			taskArns[*taskMetric.TaskArn] = true
		}
	}
	if len(requests) != expectedRequests {
		t.Errorf("Expected %d requests, got %d", expectedRequests, len(requests))
	}
	lastRequest := requests[expectedRequests-1]
	if !*lastRequest.Metadata.Fin {
		t.Error("Fin not set to true in last request")
	}
	requests = requests[:(expectedRequests - 1)]
	for i, request := range requests {
		if *request.Metadata.Fin {
			t.Errorf("Fin set to true in request %d/%d", i, (expectedRequests - 1))
		}
	}
}

func testCS(conn *mock_wsconn.MockWebsocketConn) wsclient.ClientServer {
	cfg := &config.Config{
		AWSRegion:          "us-east-1",
		AcceptInsecureCert: true,
	}
	cs := New("https://aws.amazon.com/ecs", cfg, testCreds, &mockStatsEngine{},
		testPublishMetricsInterval, testPublishInstanceHealthMetricsInterval, rwTimeout, false).(*clientServer)
	cs.SetConnection(conn)
	return cs
}

// TestCloseClientServer tests the ws connection will be closed by tcs client when
// received the deregisterInstanceStream
func TestCloseClientServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn)

	gomock.InOrder(
		conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil),
		conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()),
		conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil),
		conn.EXPECT().Close(),
	)

	err := cs.MakeRequest(&ecstcs.PublishMetricsRequest{})
	assert.Nil(t, err)

	err = cs.Disconnect()
	assert.Nil(t, err)
}

func TestAckPublishHealthHandlerCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn)

	// Messages should be read from the connection at least once
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Return(1,
		[]byte(`{"type":"AckPublishHealth","message":{}}`), nil).MinTimes(1)
	// Invoked when closing the connection
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().Close()

	handledPayload := make(chan *ecstcs.AckPublishHealth)

	reqHandler := func(payload *ecstcs.AckPublishHealth) {
		handledPayload <- payload
	}
	cs.AddRequestHandler(reqHandler)

	go cs.Serve()
	defer cs.Close()

	t.Log("Waiting for handler to return payload.")
	<-handledPayload
}

// TestMetricsDisabled tests that if metrics is disabled, only container health and instance health metrics will be sent
func TestMetricsDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	instancehealthMu.Lock()
	defer instancehealthMu.Unlock()
	// seed dummy instance stats:
	for i := 0; i < int(instancehealth.MinimumSampleCount*2); i++ {
		instancehealth.DockerMetric.IncrementCallCount()
	}

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)

	cfg := config.DefaultConfig()

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval,
		testPublishInstanceHealthMetricsInterval, rwTimeout, true)
	cs.SetConnection(conn)

	published := make(chan struct{})
	readed := make(chan struct{})

	// stats engine should only be called for getting health metrics
	mockStatsEngine.EXPECT().GetTaskHealthMetrics().Return(&ecstcs.HealthMetadata{
		Cluster:           aws.String("TestMetricsDisabled"),
		ContainerInstance: aws.String("container_instance"),
		Fin:               aws.Bool(true),
		MessageId:         aws.String("message_id"),
	}, []*ecstcs.TaskHealth{{}}, nil).MinTimes(1)
	mockStatsEngine.EXPECT().GetInstanceHealthMetadata().Return(&ecstcs.StartTelemetrySessionInput{
		Cluster:           aws.String("TestMetricsDisabled"),
		ContainerInstance: aws.String("container_instance"),
	}).MinTimes(1)
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Do(func() {
		readed <- struct{}{}
	}).Return(1, nil, nil).MinTimes(1)
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Do(func(messageType int, data []byte) {
		published <- struct{}{}
	}).Return(nil).MinTimes(1)

	go cs.Serve()
	<-published
	<-readed
}

func TestCreatePublishHealthRequestsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)
	cfg := config.DefaultConfig()

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval,
		testPublishInstanceHealthMetricsInterval, rwTimeout, true)
	cs.SetConnection(conn)

	mockStatsEngine.EXPECT().GetTaskHealthMetrics().Return(nil, nil, stats.EmptyHealthMetricsError)
	_, err := cs.(*clientServer).createPublishHealthRequests()
	assert.Equal(t, err, stats.EmptyHealthMetricsError)

	mockStatsEngine.EXPECT().GetTaskHealthMetrics().Return(nil, nil, nil)
	_, err = cs.(*clientServer).createPublishHealthRequests()
	assert.NoError(t, err)
}

func TestCreatePublishHealthRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)
	cfg := config.DefaultConfig()

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval,
		testPublishInstanceHealthMetricsInterval, rwTimeout, true)
	cs.SetConnection(conn)

	testMetadata := &ecstcs.HealthMetadata{
		Cluster:           aws.String("TestCreatePublishHealthRequests"),
		ContainerInstance: aws.String("container_instance"),
		Fin:               aws.Bool(true),
		MessageId:         aws.String("message_id"),
	}

	testHealthMetrics := []*ecstcs.TaskHealth{
		{
			Containers: []*ecstcs.ContainerHealth{
				{
					ContainerName: aws.String("container1"),
					HealthStatus:  aws.String("HEALTHY"),
					StatusSince:   aws.Time(time.Now()),
				},
			},
			TaskArn:               aws.String("t1"),
			TaskDefinitionFamily:  aws.String("tdf1"),
			TaskDefinitionVersion: aws.String("1"),
		},
		{
			Containers: []*ecstcs.ContainerHealth{
				{
					ContainerName: aws.String("container2"),
					HealthStatus:  aws.String("HEALTHY"),
					StatusSince:   aws.Time(time.Now()),
				},
			},
			TaskArn:               aws.String("t2"),
			TaskDefinitionFamily:  aws.String("tdf2"),
			TaskDefinitionVersion: aws.String("2"),
		},
	}

	mockStatsEngine.EXPECT().GetTaskHealthMetrics().Return(testMetadata, testHealthMetrics, nil)
	request, err := cs.(*clientServer).createPublishHealthRequests()

	assert.NoError(t, err)
	assert.Len(t, request, 1)
	assert.Len(t, request[0].Tasks, 2)

	assert.Equal(t, request[0].Metadata, testMetadata)
	assert.Equal(t, request[0].Tasks, testHealthMetrics)
}

func TestCreatePublishInstanceHealthRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	instancehealthMu.Lock()
	defer instancehealthMu.Unlock()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)
	cfg := config.DefaultConfig()

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval,
		testPublishInstanceHealthMetricsInterval, rwTimeout, true)
	cs.SetConnection(conn)

	testMetadata := &ecstcs.StartTelemetrySessionInput{
		Cluster:           aws.String("TestCreatePublishInstanceHealthRequest"),
		ContainerInstance: aws.String("container_instance"),
	}
	testError := dockerapi.CannotInspectContainerError{FromError: errors.New("TestCreatePublishInstanceHealthRequest")}

	var wg sync.WaitGroup
	wg.Add(2)
	// go routine simulates metric collection
	go func() {
		defer wg.Done()
		defer instancehealth.DockerMetric.IncrementCallCount()
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < int(instancehealth.MinimumSampleCount); i++ {
			instancehealth.DockerMetric.IncrementCallCount()
			instancehealth.DockerMetric.RecordError(testError)
		}
	}()
	wg.Wait()

	mockStatsEngine.EXPECT().GetInstanceHealthMetadata().Return(testMetadata)
	request, err := cs.(*clientServer).createPublishInstanceHealthRequest()
	require.NoError(t, err)
	require.NotNil(t, request)

	expectedCallCount := int64(instancehealth.MinimumSampleCount + 1)
	expectedErrorCount := int64(instancehealth.MinimumSampleCount)
	expectedErrorMessage := "CannotInspectContainerError: TestCreatePublishInstanceHealthRequest -- "

	assert.Equal(t, testMetadata, request.Metadata)
	assert.Equal(t, expectedCallCount, *request.ContainerRuntimeSampleCount)
	assert.Equal(t, expectedErrorCount, *request.ContainerRuntimeErrors)
	assert.Equal(t, expectedErrorMessage, *request.ContainerRuntimeErrorReason)
}

func TestCreatePublishInstanceHealthRequest_InsufficientSampleCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	instancehealthMu.Lock()
	defer instancehealthMu.Unlock()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)
	cfg := config.DefaultConfig()

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval,
		testPublishInstanceHealthMetricsInterval, rwTimeout, true)
	cs.SetConnection(conn)

	testError := dockerapi.CannotInspectContainerError{FromError: errors.New("TestCreatePublishInstanceHealthRequest")}
	instancehealth.DockerMetric.RecordError(testError)
	instancehealth.DockerMetric.IncrementCallCount()

	request, err := cs.(*clientServer).createPublishInstanceHealthRequest()
	require.Error(t, err)
	require.Nil(t, request)
}

func TestSessionClosed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn)

	// Messages should be read from the connection at least once
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Return(1,
		[]byte(`{"type":"AckPublishMetric","message":{}}`), nil).MinTimes(1)
	// Invoked when closing the connection
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().Close()

	handledPayload := make(chan *ecstcs.AckPublishMetric)
	reqHandler := func(payload *ecstcs.AckPublishMetric) {
		handledPayload <- payload
	}
	cs.AddRequestHandler(reqHandler)

	go cs.Serve()
	// wait for the session start
	<-handledPayload
	cs.Close()
	_, ok := <-cs.(*clientServer).ctx.Done()
	assert.False(t, ok, "channel should be closed")
}
