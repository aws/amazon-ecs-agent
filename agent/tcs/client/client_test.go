// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/stats"
	"github.com/aws/amazon-ecs-agent/agent/stats/mock"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/wsconn/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testPublishMetricsInterval = 1 * time.Second
	testMessageId              = "testMessageId"
	testCluster                = "default"
	testContainerInstance      = "containerInstance"
	rwTimeout                  = time.Second
)

type mockStatsEngine struct{}

func (*mockStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("uninitialized")
}

func (*mockStatsEngine) ContainerDockerStats(taskARN string, id string) (*docker.Stats, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*mockStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

type emptyStatsEngine struct{}

func (*emptyStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("empty stats")
}

func (*emptyStatsEngine) ContainerDockerStats(taskARN string, id string) (*docker.Stats, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*emptyStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
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

func (*idleStatsEngine) ContainerDockerStats(taskARN string, id string) (*docker.Stats, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*idleStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

type nonIdleStatsEngine struct {
	numTasks int
}

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

func (*nonIdleStatsEngine) ContainerDockerStats(taskARN string, id string) (*docker.Stats, error) {
	return nil, fmt.Errorf("not implemented")
}

func (*nonIdleStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
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
	testCreds := credentials.AnonymousCredentials
	cfg := &config.Config{
		AWSRegion:          "us-east-1",
		AcceptInsecureCert: true,
	}
	cs := New("localhost:443", cfg, testCreds, &mockStatsEngine{},
		testPublishMetricsInterval, rwTimeout, false).(*clientServer)
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

// TestMetricsDisabled tests that if metrics is disabled, only health metrics will be sent
func TestMetricsDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)

	cfg := config.DefaultConfig()
	testCreds := credentials.AnonymousCredentials

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval, rwTimeout, true)
	cs.SetConnection(conn)
	metricsPublished := make(chan struct{})

	// stats engine should only be called for getting health metrics
	mockStatsEngine.EXPECT().GetTaskHealthMetrics().Return(&ecstcs.HealthMetadata{
		Cluster:           aws.String("TestMetricsDisabled"),
		ContainerInstance: aws.String("container_instance"),
		Fin:               aws.Bool(true),
		MessageId:         aws.String("message_id"),
	}, []*ecstcs.TaskHealth{{}}, nil).MinTimes(1)
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Return(1, nil, nil).MinTimes(1)
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Do(func(messageType int, data []byte) {
		metricsPublished <- struct{}{}
	}).Return(nil).MinTimes(1)

	go cs.Serve()
	<-metricsPublished
}

func TestCreatePublishHealthRequestsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	mockStatsEngine := mock_stats.NewMockEngine(ctrl)
	cfg := config.DefaultConfig()
	testCreds := credentials.AnonymousCredentials

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval, rwTimeout, true)
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
	testCreds := credentials.AnonymousCredentials

	cs := New("", &cfg, testCreds, mockStatsEngine, testPublishMetricsInterval, rwTimeout, true)
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
