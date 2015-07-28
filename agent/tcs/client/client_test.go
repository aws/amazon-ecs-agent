// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/gorilla/websocket"
)

const (
	testPublishMetricsInterval = 1 * time.Second
	testMessageId              = "testMessageId"
	testCluster                = "default"
	testContainerInstance      = "containerInstance"
)

type messageLogger struct {
	writes [][]byte
	reads  [][]byte
	closed bool
}

func (ml *messageLogger) WriteMessage(_ int, data []byte) error {
	if ml.closed {
		return errors.New("can't write to closed ws")
	}
	ml.writes = append(ml.writes, data)
	return nil
}

func (ml *messageLogger) Close() error {
	ml.closed = true
	return nil
}

func (ml *messageLogger) ReadMessage() (int, []byte, error) {
	for len(ml.reads) == 0 && !ml.closed {
		time.Sleep(1 * time.Millisecond)
	}
	if ml.closed {
		return 0, []byte{}, errors.New("can't read from a closed websocket")
	}
	read := ml.reads[len(ml.reads)-1]
	ml.reads = ml.reads[0 : len(ml.reads)-1]
	return websocket.TextMessage, read, nil
}

type mockStatsEngine struct{}

func (engine *mockStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("uninitialized")
}

type idleStatsEngine struct{}

func (engine *idleStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	idle := true
	messageId := testMessageId
	cluster := testCluster
	containerInstance := testContainerInstance
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           &cluster,
		ContainerInstance: &containerInstance,
		Idle:              &idle,
		MessageId:         &messageId,
	}
	return metadata, []*ecstcs.TaskMetric{}, nil
}

type nonIdleStatsEngine struct {
	numTasks int
}

func (engine *nonIdleStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	idle := false
	messageId := testMessageId
	cluster := testCluster
	containerInstance := testContainerInstance
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           &cluster,
		ContainerInstance: &containerInstance,
		Idle:              &idle,
		MessageId:         &messageId,
	}
	var taskMetrics []*ecstcs.TaskMetric
	for i := 0; i < engine.numTasks; i++ {
		taskMetrics = append(taskMetrics, &ecstcs.TaskMetric{})
	}
	return metadata, taskMetrics, nil
}

func newNonIdleStatsEngine(numTasks int) *nonIdleStatsEngine {
	return &nonIdleStatsEngine{numTasks: numTasks}
}

func TestPayloadHandlerCalled(t *testing.T) {
	cs, ml := testCS()

	var handledPayload *ecstcs.AckPublishMetric
	reqHandler := func(payload *ecstcs.AckPublishMetric) {
		handledPayload = payload
	}
	cs.AddRequestHandler(reqHandler)

	ml.reads = [][]byte{[]byte(`{"type":"AckPublishMetric","message":{}}`)}

	var isClosed bool
	go func() {
		err := cs.Serve()
		if !isClosed && err != nil {
			t.Fatal("Premature end of serving", err)
		}
	}()

	time.Sleep(1 * time.Millisecond)
	if handledPayload == nil {
		t.Fatal("Handler was not called")
	}

	isClosed = true
	cs.Close()
}

func TestPublishMetricsRequest(t *testing.T) {
	cs, _ := testCS()
	err := cs.MakeRequest(&ecstcs.PublishMetricsRequest{})
	if err != nil {
		t.Fatal(err)
	}

	cs.Close()
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
	numTasks := (tasksInMessage * (expectedRequests - 1)) + 1
	cs := clientServer{
		statsEngine: newNonIdleStatsEngine(numTasks),
	}
	requests, err := cs.metricsToPublishMetricRequests()
	if err != nil {
		t.Fatal("Error creating publishmetricrequests: ", err)
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

func testCS() (wsclient.ClientServer, *messageLogger) {
	testCreds := credentials.AnonymousCredentials
	cs := New("localhost:443", "us-east-1", testCreds, true, &mockStatsEngine{}, testPublishMetricsInterval).(*clientServer)
	ml := &messageLogger{make([][]byte, 0), make([][]byte, 0), false}
	cs.Conn = ml
	return cs, ml
}
