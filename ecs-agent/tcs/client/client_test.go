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

// Package tcsclient wraps the generated aws-sdk-go client to provide marshalling
// and unmarshalling of data over a websocket connection in the format expected
// by TCS. It allows for bidirectional communication and acts as both a
// client-and-server in terms of requests, but only as a client in terms of
// connecting.

package tcsclient

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	mock_wsconn "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/wsconn/mock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	testPublishMetricsInterval                                = 1 * time.Second
	testMessageId                                             = "testMessageId"
	testCluster                                               = "default"
	testContainerInstance                                     = "containerInstance"
	rwTimeout                                                 = time.Second
	testPublishMetricRequestSizeLimitSC                       = 1024
	testPublishMetricRequestSizeLimitNonSC                    = 230
	testPublishMetricRequestSizeLimitNonSCWithInstanceMetrics = 300
	testTelemetryChannelDefaultBufferSize                     = 10
	testIncludeScStats                                        = true
	testNotIncludeScStats                                     = false
)

type trueHealthcheck struct{}

func (tc *trueHealthcheck) RunCheck() doctor.HealthcheckStatus                   { return doctor.HealthcheckStatusOk }
func (tc *trueHealthcheck) SetHealthcheckStatus(status doctor.HealthcheckStatus) {}
func (tc *trueHealthcheck) GetHealthcheckType() string                           { return doctor.HealthcheckTypeAgent }
func (tc *trueHealthcheck) GetHealthcheckStatus() doctor.HealthcheckStatus {
	return doctor.HealthcheckStatusInitializing
}
func (tc *trueHealthcheck) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	return doctor.HealthcheckStatusInitializing
}
func (tc *trueHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *trueHealthcheck) GetStatusChangeTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *trueHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}

type falseHealthcheck struct{}

func (fc *falseHealthcheck) RunCheck() doctor.HealthcheckStatus {
	return doctor.HealthcheckStatusImpaired
}
func (fc *falseHealthcheck) SetHealthcheckStatus(status doctor.HealthcheckStatus) {}
func (fc *falseHealthcheck) GetHealthcheckType() string                           { return doctor.HealthcheckTypeAgent }
func (fc *falseHealthcheck) GetHealthcheckStatus() doctor.HealthcheckStatus {
	return doctor.HealthcheckStatusInitializing
}
func (fc *falseHealthcheck) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	return doctor.HealthcheckStatusInitializing
}
func (fc *falseHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (fc *falseHealthcheck) GetStatusChangeTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (fc *falseHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}

var testCreds = credentials.NewStaticCredentialsProvider("test-id", "test-secret", "test-token")

var emptyDoctor, _ = doctor.NewDoctor([]doctor.Healthcheck{}, "test-cluster", "this:is:an:instance:arn")

type mockStatsSource struct{}

func (*mockStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("uninitialized")
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

func (*mockStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return time.NewTicker(DefaultContainerMetricsPublishInterval)
}

type emptyStatsSource struct{}

func (*emptyStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	return nil, nil, fmt.Errorf("empty stats")
}

func (*emptyStatsSource) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*emptyStatsSource) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*emptyStatsSource) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (*emptyStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return time.NewTicker(DefaultContainerMetricsPublishInterval)
}

type idleStatsSource struct{}

func (*idleStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(testCluster),
		ContainerInstance: aws.String(testContainerInstance),
		Idle:              aws.Bool(true),
		MessageId:         aws.String(testMessageId),
	}
	return metadata, []*ecstcs.TaskMetric{}, nil
}

func (*idleStatsSource) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*idleStatsSource) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*idleStatsSource) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (*idleStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return time.NewTicker(DefaultContainerMetricsPublishInterval)
}

type nonIdleStatsSource struct {
	numTasks int
}

func (engine *nonIdleStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
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

func (*nonIdleStatsSource) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*nonIdleStatsSource) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*nonIdleStatsSource) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (*nonIdleStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return time.NewTicker(DefaultContainerMetricsPublishInterval)
}

func newNonIdleStatsSource(numTasks int) *nonIdleStatsSource {
	return &nonIdleStatsSource{numTasks: numTasks}
}

type serviceConnectStatsSource struct {
	numTasks int
}

func (engine *serviceConnectStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(testCluster),
		ContainerInstance: aws.String(testContainerInstance),
		Idle:              aws.Bool(false),
		MessageId:         aws.String(testMessageId),
	}
	var taskMetrics []*ecstcs.TaskMetric
	var i int64
	var fval float64
	fval = rand.Float64()
	var ival int64
	ival = rand.Int63n(10)
	for i = 0; int(i) < engine.numTasks; i++ {
		taskArn := "task/" + strconv.FormatInt(i, 10)
		taskMetric := ecstcs.TaskMetric{
			TaskArn: &taskArn,
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
		}
		if includeServiceConnectStats {
			var serviceConnectMetrics []*ecstcs.GeneralMetricsWrapper
			var generalMetrics []*ecstcs.GeneralMetric
			metricType := "2"
			dimensionKey := "ClusterName"
			dimentsionValue := "TestClusterName"
			metricName := "HTTPCode_Target_2XX_Count"
			metricValue := 3.0
			var metricCount int64 = 1

			// generate a task metric with size more than testPublishMetricRequestSizeLimitSC i.e 1kB
			generalMetric := ecstcs.GeneralMetric{
				MetricName:   &metricName,
				MetricValues: []*float64{&metricValue},
				MetricCounts: []*int64{&metricCount},
			}
			generalMetrics = append(generalMetrics, &generalMetric)
			generalMetrics = append(generalMetrics, &generalMetric)
			generalMetrics = append(generalMetrics, &generalMetric)
			generalMetrics = append(generalMetrics, &generalMetric)
			generalMetricsWrapper := ecstcs.GeneralMetricsWrapper{
				MetricType: &metricType,
				Dimensions: []*ecstcs.Dimension{
					{
						Key:   &dimensionKey,
						Value: &dimentsionValue,
					},
				},
				GeneralMetrics: generalMetrics,
			}
			serviceConnectMetrics = append(serviceConnectMetrics, &generalMetricsWrapper)
			serviceConnectMetrics = append(serviceConnectMetrics, &generalMetricsWrapper)
			taskMetric.ServiceConnectMetricsWrapper = serviceConnectMetrics
		}
		taskMetrics = append(taskMetrics, &taskMetric)
	}
	return metadata, taskMetrics, nil
}

func (*serviceConnectStatsSource) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*serviceConnectStatsSource) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*serviceConnectStatsSource) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (*serviceConnectStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return time.NewTicker(DefaultContainerMetricsPublishInterval)
}

func newServiceConnectStatsSource(numTasks int) *serviceConnectStatsSource {
	return &serviceConnectStatsSource{numTasks: numTasks}
}

type nonIdleInstanceMetricsStatsSource struct {
	numTasks int
}

func (engine *nonIdleInstanceMetricsStatsSource) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.InstanceMetrics, *ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	metadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(testCluster),
		ContainerInstance: aws.String(testContainerInstance),
		Idle:              aws.Bool(false),
		MessageId:         aws.String(testMessageId),
	}
	var instanceMetrics *ecstcs.InstanceMetrics
	var storageMetrics *ecstcs.InstanceStorageMetrics
	var taskMetrics []*ecstcs.TaskMetric
	var i int64
	for i = 0; int(i) < engine.numTasks; i++ {
		taskArn := "task/" + strconv.FormatInt(i, 10)
		taskMetrics = append(taskMetrics, &ecstcs.TaskMetric{TaskArn: &taskArn})
	}
	storageMetrics = &ecstcs.InstanceStorageMetrics{DataFilesystem: aws.Float64(25), RootFilesystem: aws.Float64(50)}
	instanceMetrics = &ecstcs.InstanceMetrics{Storage: storageMetrics}
	return instanceMetrics, metadata, taskMetrics, nil
}

func (*nonIdleInstanceMetricsStatsSource) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	return nil, nil, nil
}

func (*nonIdleInstanceMetricsStatsSource) GetPublishServiceConnectTickerInterval() int32 {
	return 0
}

func (*nonIdleInstanceMetricsStatsSource) SetPublishServiceConnectTickerInterval(counter int32) {
	return
}

func (*nonIdleInstanceMetricsStatsSource) GetPublishMetricsTicker() *time.Ticker {
	return time.NewTicker(DefaultContainerMetricsPublishInterval)
}

func newNonIdleInstanceMetricsStatsSource(numTasks int) *nonIdleInstanceMetricsStatsSource {
	return &nonIdleInstanceMetricsStatsSource{numTasks: numTasks}
}

func TestPayloadHandlerCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn, nil, nil)

	ctx, _ := context.WithCancel(context.TODO())

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

	go cs.Serve(ctx)
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

	cs := testCS(conn, nil, nil)
	defer cs.Close()
	err := cs.MakeRequest(&ecstcs.PublishMetricsRequest{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMetricsToPublishMetricRequestsIdleStatsSource(t *testing.T) {
	cs := tcsClientServer{}
	statsSource := idleStatsSource{}
	metadata, taskMetrics, _ := statsSource.GetInstanceMetrics(testNotIncludeScStats)
	requests, err := cs.metricsToPublishMetricRequests(ecstcs.TelemetryMessage{
		Metadata:    metadata,
		TaskMetrics: taskMetrics,
	})
	if err != nil {
		t.Fatal("Error creating publishMetricRequests: ", err)
	}
	if len(requests) != 1 {
		t.Errorf("Expected %d requests, got %d", 1, len(requests))
	}
	lastRequest := requests[0]
	if !*lastRequest.Metadata.Fin {
		t.Error("Fin not set to true in Last request")
	}
}

// TestMetricsToPublishMetricRequestsNonIdleStatsSourcePaginationWithMetricsSize checks the correct pagination behavior
// due to number of tasks
func TestMetricsToPublishMetricRequestsNonIdleStatsSourcePaginationWithTaskNumber(t *testing.T) {
	expectedRequests := 3
	// Creates 21 task metrics, which translate to 3 batches,
	// {[Task1, Task2, ...Task10], [Task11, Task12, ...Task20], [Task21]}
	numTasks := (tasksInMetricMessage * (expectedRequests - 1)) + 1
	cs := tcsClientServer{}
	statsSource := nonIdleStatsSource{
		numTasks: numTasks,
	}
	metadata, taskMetrics, err := statsSource.GetInstanceMetrics(testNotIncludeScStats)
	requests, err := cs.metricsToPublishMetricRequests(ecstcs.TelemetryMessage{
		Metadata:    metadata,
		TaskMetrics: taskMetrics,
	})
	if err != nil {
		t.Fatal("Error creating publishMetricRequests: ", err)
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

// TestMetricsToPublishMetricRequestsNonIdleStatsSourcePaginationWithMetricsSize checks the correct pagination behavior
// due to metric size limit
func TestMetricsToPublishMetricRequestsNonIdleStatsSourcePaginationWithMetricsSize(t *testing.T) {
	tempLimit := publishMetricRequestSizeLimit
	publishMetricRequestSizeLimit = testPublishMetricRequestSizeLimitNonSC
	defer func() {
		publishMetricRequestSizeLimit = tempLimit
	}()

	expectedRequests := 2
	// Creates 3 task metrics, which translate to 2 batches,
	// {[Task1, Task2], [Task3]}
	numTasks := 3
	cs := tcsClientServer{}
	statsSource := nonIdleStatsSource{
		numTasks: numTasks,
	}
	metadata, taskMetrics, err := statsSource.GetInstanceMetrics(testNotIncludeScStats)
	requests, err := cs.metricsToPublishMetricRequests(ecstcs.TelemetryMessage{
		Metadata:    metadata,
		TaskMetrics: taskMetrics,
	})
	if err != nil {
		t.Fatal("Error creating publishMetricRequests: ", err)
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

// TestMetricsToPublishMetricRequestsNonIdleInstanceMetricsStatsSource checks the Instance Metrics are included
// only in the first request of a batch.
func TestMetricsToPublishMetricRequestsNonIdleInstanceMetricsStatsSource(t *testing.T) {
	tempLimit := publishMetricRequestSizeLimit
	publishMetricRequestSizeLimit = testPublishMetricRequestSizeLimitNonSCWithInstanceMetrics
	defer func() {
		publishMetricRequestSizeLimit = tempLimit
	}()

	expectedRequests := 2
	// Creates 3 task metrics, which translate to 2 batches,
	// {[Task1, Task2], [Task3]}
	numTasks := 3
	cs := tcsClientServer{}
	statsSource := newNonIdleInstanceMetricsStatsSource(numTasks)
	instanceMetrics, metadata, taskMetrics, err := statsSource.GetInstanceMetrics(testNotIncludeScStats)
	if err != nil {
		t.Errorf("Error getting Instance Metrics")
	}
	requests, err := cs.metricsToPublishMetricRequests(ecstcs.TelemetryMessage{
		InstanceMetrics: instanceMetrics,
		Metadata:        metadata,
		TaskMetrics:     taskMetrics,
	})
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
	for i, req := range requests {
		if i == 0 && req.InstanceMetrics == nil {
			t.Error("Instance Metrics not included in the first request")
		}
		if i != 0 && req.InstanceMetrics != nil {
			t.Error("Instance Metrics included in the requests other than the first request")
		}
	}
}

func TestMetricsToPublishMetricRequestsServiceConnectStatsSource(t *testing.T) {
	tempLimit := publishMetricRequestSizeLimit
	publishMetricRequestSizeLimit = testPublishMetricRequestSizeLimitSC
	defer func() {
		publishMetricRequestSizeLimit = tempLimit
	}()

	testCases := []struct {
		name             string
		numTasks         int
		expectedRequests int
	}{
		{
			name:             "publish metrics requests with under 10 tasks with service connect stats",
			numTasks:         3,
			expectedRequests: 6,
		},
		{
			name:             "publish metrics requests with more than 10 tasks with service connect stats",
			numTasks:         20,
			expectedRequests: 40,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cs := tcsClientServer{}
			statsSource := newServiceConnectStatsSource(tc.numTasks)
			metadata, taskMetrics, _ := statsSource.GetInstanceMetrics(testIncludeScStats)
			requests, err := cs.metricsToPublishMetricRequests(ecstcs.TelemetryMessage{
				Metadata:    metadata,
				TaskMetrics: taskMetrics,
			})
			if err != nil {
				t.Fatal("Error creating publishMetricRequests: ", err)
			}

			taskArns := make(map[string]bool)
			for _, request := range requests {
				for _, taskMetric := range request.TaskMetrics {
					_, exists := taskArns[*taskMetric.TaskArn]
					// if it is first part of task metric or a complete task metric being sent in this request
					// validate that ContainerMetrics is not empty
					if !exists {
						assert.NotEmpty(t, taskMetric.ContainerMetrics, "Expected Container metrics to be not empty")
					} else {
						// task metric with remaining service connect metrics being sent in the next request
						// validate that ContainerMetrics is empty
						assert.Empty(t, taskMetric.ContainerMetrics, "Expected Container metrics to be empty, got %d", len(taskMetric.ContainerMetrics))
					}
					taskArns[*taskMetric.TaskArn] = true
				}
			}
			assert.Equal(t, tc.expectedRequests, len(requests), "Wrong number of requests generated")
			lastRequest := requests[tc.expectedRequests-1]
			assert.True(t, *lastRequest.Metadata.Fin, "Fin not set to true in last request")
			requests = requests[:(tc.expectedRequests - 1)]
			for i, request := range requests {
				assert.False(t, *request.Metadata.Fin, "Fin set to true in request %d/%d", i, (tc.expectedRequests - 1))
			}
		})
	}
}

func testCS(conn *mock_wsconn.MockWebsocketConn, metricsMessages <-chan ecstcs.TelemetryMessage, healthMessages <-chan ecstcs.HealthMessage) wsclient.ClientServer {
	cfg := &wsclient.WSClientMinAgentConfig{
		AWSRegion:          "us-east-1",
		AcceptInsecureCert: true,
	}
	cs := New("https://aws.amazon.com/ecs", cfg, emptyDoctor, false, testPublishMetricsInterval,
		aws.NewCredentialsCache(testCreds), rwTimeout, metricsMessages, healthMessages, nil, metrics.NewNopEntryFactory()).(*tcsClientServer)
	cs.SetConnection(conn)
	return cs
}

// TestCloseClientServer tests the ws connection will be closed by tcs client when
// received the deregisterInstanceStream
func TestCloseClientServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn, nil, nil)

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
	cs := testCS(conn, nil, nil)

	ctx, _ := context.WithCancel(context.TODO())

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

	go cs.Serve(ctx)
	defer cs.Close()

	t.Log("Waiting for handler to return payload.")
	<-handledPayload
}

func TestHealthToPublishHealthRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)

	cfg := &wsclient.WSClientMinAgentConfig{
		AWSRegion:          "us-east-1",
		AcceptInsecureCert: true,
		IsDocker:           true,
	}

	cs := New("", cfg, emptyDoctor, true, testPublishMetricsInterval, aws.NewCredentialsCache(testCreds), rwTimeout, nil, nil, nil, metrics.NewNopEntryFactory())
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
					StatusSince:   (*utils.Timestamp)(aws.Time(time.Now())),
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
					StatusSince:   (*utils.Timestamp)(aws.Time(time.Now())),
				},
			},
			TaskArn:               aws.String("t2"),
			TaskDefinitionFamily:  aws.String("tdf2"),
			TaskDefinitionVersion: aws.String("2"),
		},
	}

	request, err := cs.(*tcsClientServer).healthToPublishHealthRequests(ecstcs.HealthMessage{
		Metadata:      testMetadata,
		HealthMetrics: testHealthMetrics,
	})

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
	cs := testCS(conn, nil, nil)

	ctx, _ := context.WithCancel(context.TODO())

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

	go cs.Serve(ctx)
	// wait for the session start
	<-handledPayload
	cs.Close()
}

func TestGetInstanceStatuses(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}
	trueStatus := &ecstcs.InstanceStatus{
		LastStatusChange: (*utils.Timestamp)(aws.Time(trueCheck.GetStatusChangeTime())),
		LastUpdated:      (*utils.Timestamp)(aws.Time(trueCheck.GetLastHealthcheckTime())),
		Status:           aws.String(trueCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(trueCheck.GetHealthcheckType()),
	}
	falseStatus := &ecstcs.InstanceStatus{
		LastStatusChange: (*utils.Timestamp)(aws.Time(falseCheck.GetStatusChangeTime())),
		LastUpdated:      (*utils.Timestamp)(aws.Time(falseCheck.GetLastHealthcheckTime())),
		Status:           aws.String(falseCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(falseCheck.GetHealthcheckType()),
	}

	testcases := []struct {
		name           string
		checks         []doctor.Healthcheck
		expectedResult []*ecstcs.InstanceStatus
	}{
		{
			name:           "empty checks",
			checks:         []doctor.Healthcheck{},
			expectedResult: nil,
		},
		{
			name:           "all true checks",
			checks:         []doctor.Healthcheck{trueCheck},
			expectedResult: []*ecstcs.InstanceStatus{trueStatus},
		},
		{
			name:           "all false checks",
			checks:         []doctor.Healthcheck{falseCheck},
			expectedResult: []*ecstcs.InstanceStatus{falseStatus},
		},
		{
			name:           "mixed checks",
			checks:         []doctor.Healthcheck{trueCheck, falseCheck},
			expectedResult: []*ecstcs.InstanceStatus{trueStatus, falseStatus},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctor, _ := doctor.NewDoctor(tc.checks, testCluster, testContainerInstance)
			cs := tcsClientServer{
				doctor: newDoctor,
			}
			cs.doctor.RunHealthchecks()

			instanceStatuses := cs.getInstanceStatuses()
			assert.Equal(t, instanceStatuses, tc.expectedResult)
		})
	}
}

func TestGetPublishInstanceStatusRequest(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}
	trueStatus := &ecstcs.InstanceStatus{
		LastStatusChange: (*utils.Timestamp)(aws.Time(trueCheck.GetStatusChangeTime())),
		LastUpdated:      (*utils.Timestamp)(aws.Time(trueCheck.GetLastHealthcheckTime())),
		Status:           aws.String(trueCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(trueCheck.GetHealthcheckType()),
	}
	falseStatus := &ecstcs.InstanceStatus{
		LastStatusChange: (*utils.Timestamp)(aws.Time(falseCheck.GetStatusChangeTime())),
		LastUpdated:      (*utils.Timestamp)(aws.Time(falseCheck.GetLastHealthcheckTime())),
		Status:           aws.String(falseCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(falseCheck.GetHealthcheckType()),
	}

	testcases := []struct {
		name             string
		checks           []doctor.Healthcheck
		expectedStatuses []*ecstcs.InstanceStatus
	}{
		{
			name:             "empty checks",
			checks:           []doctor.Healthcheck{},
			expectedStatuses: nil,
		},
		{
			name:             "all true checks",
			checks:           []doctor.Healthcheck{trueCheck},
			expectedStatuses: []*ecstcs.InstanceStatus{trueStatus},
		},
		{
			name:             "all false checks",
			checks:           []doctor.Healthcheck{falseCheck},
			expectedStatuses: []*ecstcs.InstanceStatus{falseStatus},
		},
		{
			name:             "mixed checks",
			checks:           []doctor.Healthcheck{trueCheck, falseCheck},
			expectedStatuses: []*ecstcs.InstanceStatus{trueStatus, falseStatus},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctor, _ := doctor.NewDoctor(tc.checks, testCluster, testContainerInstance)
			cs := tcsClientServer{
				doctor: newDoctor,
			}
			cs.doctor.RunHealthchecks()

			// note: setting RequestId and Timestamp to nil so I can make the comparison
			metadata := &ecstcs.InstanceStatusMetadata{
				Cluster:           aws.String(testCluster),
				ContainerInstance: aws.String(testContainerInstance),
				RequestId:         nil,
			}

			testResult, err := cs.getPublishInstanceStatusRequest()

			if tc.expectedStatuses != nil {
				expectedResult := &ecstcs.PublishInstanceStatusRequest{
					Metadata:  metadata,
					Statuses:  tc.expectedStatuses,
					Timestamp: nil,
				}
				// note: setting RequestId and Timestamp to nil so I can make the comparison
				testResult.Timestamp = nil
				testResult.Metadata.RequestId = nil
				assert.Equal(t, testResult, expectedResult)
			} else {
				assert.Error(t, err, "Test failed")
			}
		})
	}
}

func TestAckPublishInstanceStatusHandlerCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn, nil, nil)

	ctx, _ := context.WithCancel(context.TODO())

	// Messages should be read from the connection at least once
	conn.EXPECT().SetReadDeadline(gomock.Any()).Return(nil).MinTimes(1)
	conn.EXPECT().ReadMessage().Return(1,
		[]byte(`{"type":"AckPublishInstanceStatus","message":{}}`), nil).MinTimes(1)
	// Invoked when closing the connection
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().Close()

	handledPayload := make(chan *ecstcs.AckPublishInstanceStatus)

	reqHandler := func(payload *ecstcs.AckPublishInstanceStatus) {
		handledPayload <- payload
	}
	cs.AddRequestHandler(reqHandler)

	go cs.Serve(ctx)
	defer cs.Close()

	t.Log("Waiting for handler to return payload.")
	<-handledPayload
}

func TestEmptyChannelNonBlocking(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx, cancel := context.WithCancel(context.TODO())

	telemetryMessages := make(chan ecstcs.TelemetryMessage, 10)
	healthMessages := make(chan ecstcs.HealthMessage, 10)

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	go cancelAfterWait(cancel)

	// verify publishMessages returns (empty channels) after context cancels
	cs.publishMessages(ctx)

	// verify message is polled out
	assert.Len(t, telemetryMessages, 0)
	assert.Len(t, healthMessages, 0)
}

func cancelAfterWait(cancel context.CancelFunc) {
	time.Sleep(5 * time.Second)
	cancel()
}

func TestInvalidFormatMessageOnChannel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx, _ := context.WithCancel(context.TODO())

	telemetryMessages := make(chan ecstcs.TelemetryMessage, 10)
	healthMessages := make(chan ecstcs.HealthMessage, 10)

	// channel will do type check when sending message. We can only test nil attribute case.
	telemetryMessages <- ecstcs.TelemetryMessage{}
	healthMessages <- ecstcs.HealthMessage{}

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	go cs.publishMessages(ctx)
	time.Sleep(1 * time.Second) // wait for message polled

	// verify message is polled out
	assert.Len(t, telemetryMessages, 0)
	assert.Len(t, healthMessages, 0)

	// verify no request was made from the two ill-formed message
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Times(0)
}

// TestNewConstructorWithInstanceStatusChannel tests the constructor with instanceStatus channel parameter.
func TestNewConstructorWithInstanceStatusChannel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                       string
		url                        string
		disableResourceMetrics     bool
		publishMetricsInterval     time.Duration
		metricsMessages            <-chan ecstcs.TelemetryMessage
		healthMessages             <-chan ecstcs.HealthMessage
		instanceStatusMessages     <-chan ecstcs.InstanceStatusMessage
		expectedInstanceStatusChan bool
	}{
		{
			name:                       "constructor with valid instanceStatus channel",
			url:                        "https://aws.amazon.com/ecs",
			disableResourceMetrics:     false,
			publishMetricsInterval:     testPublishMetricsInterval,
			metricsMessages:            make(chan ecstcs.TelemetryMessage, 1),
			healthMessages:             make(chan ecstcs.HealthMessage, 1),
			instanceStatusMessages:     make(chan ecstcs.InstanceStatusMessage, 1),
			expectedInstanceStatusChan: true,
		},
		{
			name:                       "constructor with nil instanceStatus channel",
			url:                        "https://aws.amazon.com/ecs",
			disableResourceMetrics:     true,
			publishMetricsInterval:     testPublishMetricsInterval,
			metricsMessages:            make(chan ecstcs.TelemetryMessage, 1),
			healthMessages:             make(chan ecstcs.HealthMessage, 1),
			instanceStatusMessages:     nil,
			expectedInstanceStatusChan: false,
		},
		{
			name:                       "constructor with all channels nil",
			url:                        "https://aws.amazon.com/ecs",
			disableResourceMetrics:     false,
			publishMetricsInterval:     testPublishMetricsInterval,
			metricsMessages:            nil,
			healthMessages:             nil,
			instanceStatusMessages:     nil,
			expectedInstanceStatusChan: false,
		},
		{
			name:                       "constructor with different URL and settings",
			url:                        "https://test.example.com",
			disableResourceMetrics:     true,
			publishMetricsInterval:     2 * time.Second,
			metricsMessages:            make(chan ecstcs.TelemetryMessage, 5),
			healthMessages:             make(chan ecstcs.HealthMessage, 5),
			instanceStatusMessages:     make(chan ecstcs.InstanceStatusMessage, 5),
			expectedInstanceStatusChan: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &wsclient.WSClientMinAgentConfig{
				AWSRegion:          "us-east-1",
				AcceptInsecureCert: true,
			}

			cs := New(
				tc.url,
				cfg,
				emptyDoctor,
				tc.disableResourceMetrics,
				tc.publishMetricsInterval,
				aws.NewCredentialsCache(testCreds),
				rwTimeout,
				tc.metricsMessages,
				tc.healthMessages,
				tc.instanceStatusMessages,
				metrics.NewNopEntryFactory(),
			).(*tcsClientServer)

			// Verify that the channel is properly stored in the struct
			if tc.expectedInstanceStatusChan {
				assert.NotNil(t, cs.instanceStatus, "instanceStatus channel should be stored when provided")
				assert.Equal(t, tc.instanceStatusMessages, cs.instanceStatus, "instanceStatus channel should match the provided channel")
			} else {
				assert.Nil(t, cs.instanceStatus, "instanceStatus channel should be nil when not provided")
			}

			// Verify other fields are properly set
			assert.Equal(t, tc.disableResourceMetrics, cs.disableResourceMetrics, "disableResourceMetrics should match")
			assert.Equal(t, tc.publishMetricsInterval, cs.publishMetricsInterval, "publishMetricsInterval should match")

			// Verify channels are set correctly (checking for nil/non-nil rather than exact equality due to type conversion)
			if tc.metricsMessages != nil {
				assert.NotNil(t, cs.metrics, "metrics channel should be set when provided")
			} else {
				assert.Nil(t, cs.metrics, "metrics channel should be nil when not provided")
			}

			if tc.healthMessages != nil {
				assert.NotNil(t, cs.health, "health channel should be set when provided")
			} else {
				assert.Nil(t, cs.health, "health channel should be nil when not provided")
			}

			assert.Equal(t, emptyDoctor, cs.doctor, "doctor should match")
			assert.Equal(t, tc.url, cs.URL, "URL should match")
		})
	}
}

// TestNewConstructorBackwardCompatibility tests backward compatibility of the constructor.
func TestNewConstructorBackwardCompatibility(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                   string
		instanceStatusMessages <-chan ecstcs.InstanceStatusMessage
		description            string
	}{
		{
			name:                   "nil instanceStatus channel maintains compatibility",
			instanceStatusMessages: nil,
			description:            "Constructor should work with nil instanceStatusMessages parameter",
		},
		{
			name:                   "valid instanceStatus channel works correctly",
			instanceStatusMessages: make(chan ecstcs.InstanceStatusMessage, 1),
			description:            "Constructor should work with valid instanceStatusMessages parameter",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &wsclient.WSClientMinAgentConfig{
				AWSRegion:          "us-east-1",
				AcceptInsecureCert: true,
			}

			metricsMessages := make(chan ecstcs.TelemetryMessage, 1)
			healthMessages := make(chan ecstcs.HealthMessage, 1)

			// Test that constructor works without errors
			cs := New(
				"https://aws.amazon.com/ecs",
				cfg,
				emptyDoctor,
				false,
				testPublishMetricsInterval,
				aws.NewCredentialsCache(testCreds),
				rwTimeout,
				metricsMessages,
				healthMessages,
				tc.instanceStatusMessages,
				metrics.NewNopEntryFactory(),
			)

			// Verify that the client server is created successfully
			assert.NotNil(t, cs, "ClientServer should be created successfully")

			// Verify that it implements the expected interface
			_, ok := cs.(wsclient.ClientServer)
			assert.True(t, ok, "Returned object should implement wsclient.ClientServer interface")

			// Cast to concrete type to verify internal state
			tcsCS := cs.(*tcsClientServer)

			// Verify existing functionality is not affected
			assert.NotNil(t, tcsCS.metrics, "metrics channel should be set")
			assert.NotNil(t, tcsCS.health, "health channel should be set")
			assert.Equal(t, emptyDoctor, tcsCS.doctor, "doctor should be properly set")
			assert.Equal(t, testPublishMetricsInterval, tcsCS.publishMetricsInterval, "publishMetricsInterval should be properly set")

			// Verify instanceStatus field is handled correctly
			if tc.instanceStatusMessages != nil {
				assert.NotNil(t, tcsCS.instanceStatus, "instanceStatus channel should be set when provided")
			} else {
				assert.Nil(t, tcsCS.instanceStatus, "instanceStatus channel should be nil when not provided")
			}

			// Verify basic interface compliance without calling Close() which requires a connection
			assert.NotNil(t, cs, "ClientServer should implement the interface correctly")
		})
	}
}

// TestPublishMessagesInstanceStatusReception tests instanceStatus message reception and processing.
func TestPublishMessagesInstanceStatusReception(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                  string
		instanceStatusMessage ecstcs.InstanceStatusMessage
		expectPublishCall     bool
		mockSetup             func(*mock_wsconn.MockWebsocketConn)
		expectedError         bool
	}{
		{
			name: "successful instanceStatus message processing",
			instanceStatusMessage: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
			expectPublishCall: true,
			mockSetup: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "instanceStatus message with multiple statuses",
			instanceStatusMessage: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
					{
						Status: aws.String("IMPAIRED"),
						Type:   aws.String("DOCKER"),
					},
				},
			},
			expectPublishCall: true,
			mockSetup: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "instanceStatus message with empty statuses",
			instanceStatusMessage: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{},
			},
			expectPublishCall: true,
			mockSetup: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			conn := mock_wsconn.NewMockWebsocketConn(ctrl)
			instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

			cs := testCS(conn, nil, nil).(*tcsClientServer)
			cs.instanceStatus = instanceStatusMessages

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			if tc.expectPublishCall {
				tc.mockSetup(conn)
			}

			// Start publishMessages in a goroutine
			go cs.publishMessages(ctx)

			// Send the instanceStatus message
			instanceStatusMessages <- tc.instanceStatusMessage

			// Give some time for message processing
			time.Sleep(100 * time.Millisecond)

			// Cancel context to stop publishMessages
			cancel()

			// Verify message was consumed from channel
			assert.Len(t, instanceStatusMessages, 0, "instanceStatus message should be consumed from channel")
		})
	}
}

// TestPublishMessagesConcurrentHandling tests concurrent handling of all three message types.
func TestPublishMessagesConcurrentHandling(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	telemetryMessages := make(chan ecstcs.TelemetryMessage, 1)
	healthMessages := make(chan ecstcs.HealthMessage, 1)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	cs.instanceStatus = instanceStatusMessages

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Expect three WriteMessage calls for the three different message types
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil).Times(3)

	// Start publishMessages in a goroutine
	go cs.publishMessages(ctx)

	// Create test messages
	telemetryMessage := ecstcs.TelemetryMessage{
		Metadata: &ecstcs.MetricsMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			Idle:              aws.Bool(true),
			MessageId:         aws.String("test-message"),
		},
		TaskMetrics: []*ecstcs.TaskMetric{},
	}

	healthMessage := ecstcs.HealthMessage{
		Metadata: &ecstcs.HealthMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			MessageId:         aws.String("test-message"),
		},
		HealthMetrics: []*ecstcs.TaskHealth{},
	}

	instanceStatusMessage := ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			RequestId:         aws.String("test-request"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("OK"),
				Type:   aws.String("AGENT"),
			},
		},
	}

	// Send all three message types
	telemetryMessages <- telemetryMessage
	healthMessages <- healthMessage
	instanceStatusMessages <- instanceStatusMessage

	// Give some time for message processing
	time.Sleep(200 * time.Millisecond)

	// Cancel context to stop publishMessages
	cancel()

	// Verify all messages were consumed from channels
	assert.Len(t, telemetryMessages, 0, "telemetry message should be consumed from channel")
	assert.Len(t, healthMessages, 0, "health message should be consumed from channel")
	assert.Len(t, instanceStatusMessages, 0, "instanceStatus message should be consumed from channel")
}

// TestPublishMessagesErrorHandling tests error handling in publishMessages.
func TestPublishMessagesErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		setupMock           func(*mock_wsconn.MockWebsocketConn)
		sendMessage         func(chan ecstcs.InstanceStatusMessage)
		expectedErrorLogged bool
	}{
		{
			name: "publishInstanceStatusOnce fails with connection error",
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("connection error"))
			},
			sendMessage: func(ch chan ecstcs.InstanceStatusMessage) {
				ch <- ecstcs.InstanceStatusMessage{
					Metadata: &ecstcs.InstanceStatusMetadata{
						Cluster:           aws.String("test-cluster"),
						ContainerInstance: aws.String("test-instance"),
						RequestId:         aws.String("test-request"),
					},
					Statuses: []*ecstcs.InstanceStatus{
						{
							Status: aws.String("OK"),
							Type:   aws.String("AGENT"),
						},
					},
				}
			},
			expectedErrorLogged: true,
		},
		{
			name: "publishInstanceStatusOnce fails with write deadline error",
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("write deadline exceeded"))
			},
			sendMessage: func(ch chan ecstcs.InstanceStatusMessage) {
				ch <- ecstcs.InstanceStatusMessage{
					Metadata: &ecstcs.InstanceStatusMetadata{
						Cluster:           aws.String("test-cluster"),
						ContainerInstance: aws.String("test-instance"),
						RequestId:         aws.String("test-request"),
					},
					Statuses: []*ecstcs.InstanceStatus{
						{
							Status: aws.String("IMPAIRED"),
							Type:   aws.String("DOCKER"),
						},
					},
				}
			},
			expectedErrorLogged: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			conn := mock_wsconn.NewMockWebsocketConn(ctrl)
			instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

			cs := testCS(conn, nil, nil).(*tcsClientServer)
			cs.instanceStatus = instanceStatusMessages

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			tc.setupMock(conn)

			// Start publishMessages in a goroutine
			go cs.publishMessages(ctx)

			// Send the message that should cause an error
			tc.sendMessage(instanceStatusMessages)

			// Give some time for message processing and error logging
			time.Sleep(100 * time.Millisecond)

			// Cancel context to stop publishMessages
			cancel()

			// Verify message was consumed from channel even when error occurred
			assert.Len(t, instanceStatusMessages, 0, "instanceStatus message should be consumed from channel even on error")
		})
	}
}

// TestPublishMessagesErrorsDoNotAffectOtherMessageTypes tests that errors in instanceStatus processing don't affect other message types.
func TestPublishMessagesErrorsDoNotAffectOtherMessageTypes(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	telemetryMessages := make(chan ecstcs.TelemetryMessage, 1)
	healthMessages := make(chan ecstcs.HealthMessage, 1)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	cs.instanceStatus = instanceStatusMessages

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Set up mock expectations: instanceStatus fails, but telemetry and health succeed
	gomock.InOrder(
		conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("instanceStatus error")), // instanceStatus fails
		conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil),                                // telemetry succeeds
		conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil),                                // health succeeds
	)

	// Start publishMessages in a goroutine
	go cs.publishMessages(ctx)

	// Create test messages
	instanceStatusMessage := ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			RequestId:         aws.String("test-request"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("OK"),
				Type:   aws.String("AGENT"),
			},
		},
	}

	telemetryMessage := ecstcs.TelemetryMessage{
		Metadata: &ecstcs.MetricsMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			Idle:              aws.Bool(true),
			MessageId:         aws.String("test-message"),
		},
		TaskMetrics: []*ecstcs.TaskMetric{},
	}

	healthMessage := ecstcs.HealthMessage{
		Metadata: &ecstcs.HealthMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			MessageId:         aws.String("test-message"),
		},
		HealthMetrics: []*ecstcs.TaskHealth{},
	}

	// Send instanceStatus message first (which will fail)
	instanceStatusMessages <- instanceStatusMessage

	// Give some time for the error to be processed
	time.Sleep(50 * time.Millisecond)

	// Send telemetry and health messages (which should succeed)
	telemetryMessages <- telemetryMessage
	healthMessages <- healthMessage

	// Give some time for message processing
	time.Sleep(150 * time.Millisecond)

	// Cancel context to stop publishMessages
	cancel()

	// Verify all messages were consumed from channels
	assert.Len(t, instanceStatusMessages, 0, "instanceStatus message should be consumed from channel")
	assert.Len(t, telemetryMessages, 0, "telemetry message should be consumed from channel")
	assert.Len(t, healthMessages, 0, "health message should be consumed from channel")
}

// TestPublishMessagesContextCancellation tests context cancellation behavior.
func TestPublishMessagesContextCancellation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

	cs := testCS(conn, nil, nil).(*tcsClientServer)
	cs.instanceStatus = instanceStatusMessages

	ctx, cancel := context.WithCancel(context.TODO())

	// Start publishMessages in a goroutine
	done := make(chan bool)
	go func() {
		cs.publishMessages(ctx)
		done <- true
	}()

	// Cancel context immediately
	cancel()

	// Wait for publishMessages to return
	select {
	case <-done:
		// publishMessages returned as expected
	case <-time.After(1 * time.Second):
		t.Fatal("publishMessages did not return after context cancellation")
	}

	// Verify that any pending messages in channels are not processed after cancellation
	instanceStatusMessages <- ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			RequestId:         aws.String("test-request"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("OK"),
				Type:   aws.String("AGENT"),
			},
		},
	}

	// Give some time to ensure no processing occurs
	time.Sleep(50 * time.Millisecond)

	// Message should still be in channel since publishMessages has stopped
	assert.Len(t, instanceStatusMessages, 1, "instanceStatus message should remain in channel after context cancellation")
}

// TestPublishMessagesWithInstanceStatusChannelSimple tests that publishMessages handles instanceStatus messages correctly.
func TestPublishMessagesWithInstanceStatusChannelSimple(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)

	// Create all channels to avoid nil channel blocking
	telemetryMessages := make(chan ecstcs.TelemetryMessage, 1)
	healthMessages := make(chan ecstcs.HealthMessage, 1)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	cs.instanceStatus = instanceStatusMessages

	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Second)
	defer cancel()

	// Expect SetWriteDeadline and WriteMessage for instanceStatus
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)

	// Start publishMessages in a goroutine
	go cs.publishMessages(ctx)

	// Send instanceStatus message
	instanceStatusMessage := ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			RequestId:         aws.String("test-request"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("OK"),
				Type:   aws.String("AGENT"),
			},
		},
	}

	instanceStatusMessages <- instanceStatusMessage

	// Give time for processing
	time.Sleep(200 * time.Millisecond)

	// Verify message was consumed
	assert.Len(t, instanceStatusMessages, 0, "instanceStatus message should be consumed from channel")
}

// TestPublishMessagesInstanceStatusErrorSimple tests error handling for instanceStatus messages.
func TestPublishMessagesInstanceStatusErrorSimple(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)

	// Create all channels to avoid nil channel blocking
	telemetryMessages := make(chan ecstcs.TelemetryMessage, 1)
	healthMessages := make(chan ecstcs.HealthMessage, 1)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	cs.instanceStatus = instanceStatusMessages

	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Second)
	defer cancel()

	// Expect SetWriteDeadline and WriteMessage that fails
	conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
	conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("connection error"))

	// Start publishMessages in a goroutine
	go cs.publishMessages(ctx)

	// Send instanceStatus message
	instanceStatusMessage := ecstcs.InstanceStatusMessage{
		Metadata: &ecstcs.InstanceStatusMetadata{
			Cluster:           aws.String("test-cluster"),
			ContainerInstance: aws.String("test-instance"),
			RequestId:         aws.String("test-request"),
		},
		Statuses: []*ecstcs.InstanceStatus{
			{
				Status: aws.String("IMPAIRED"),
				Type:   aws.String("DOCKER"),
			},
		},
	}

	instanceStatusMessages <- instanceStatusMessage

	// Give time for processing
	time.Sleep(200 * time.Millisecond)

	// Verify message was consumed even with error
	assert.Len(t, instanceStatusMessages, 0, "instanceStatus message should be consumed from channel even on error")
}

// TestPublishMessagesContextCancellationSimple tests context cancellation behavior.
func TestPublishMessagesContextCancellationSimple(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := mock_wsconn.NewMockWebsocketConn(ctrl)

	// Create all channels to avoid nil channel blocking
	telemetryMessages := make(chan ecstcs.TelemetryMessage, 1)
	healthMessages := make(chan ecstcs.HealthMessage, 1)
	instanceStatusMessages := make(chan ecstcs.InstanceStatusMessage, 1)

	cs := testCS(conn, telemetryMessages, healthMessages).(*tcsClientServer)
	cs.instanceStatus = instanceStatusMessages

	ctx, cancel := context.WithCancel(context.TODO())

	// Start publishMessages in a goroutine
	done := make(chan bool)
	go func() {
		cs.publishMessages(ctx)
		done <- true
	}()

	// Cancel context immediately
	cancel()

	// Wait for publishMessages to return
	select {
	case <-done:
		// publishMessages returned as expected
	case <-time.After(1 * time.Second):
		t.Fatal("publishMessages did not return after context cancellation")
	}
}

// TestPublishInstanceStatusOnce tests successful instanceStatus publishing.
func TestPublishInstanceStatusOnce(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		message       ecstcs.InstanceStatusMessage
		expectedError bool
		setupMock     func(*mock_wsconn.MockWebsocketConn)
	}{
		{
			name: "successful publish with single status",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
			expectedError: false,
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "successful publish with multiple statuses",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("production-cluster"),
					ContainerInstance: aws.String("i-1234567890abcdef0"),
					RequestId:         aws.String("req-12345"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
					{
						Status: aws.String("IMPAIRED"),
						Type:   aws.String("DOCKER"),
					},
				},
			},
			expectedError: false,
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "successful publish with empty statuses",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{},
			},
			expectedError: false,
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
		{
			name: "successful publish with nil metadata fields",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           nil,
					ContainerInstance: nil,
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
			expectedError: false,
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			conn := mock_wsconn.NewMockWebsocketConn(ctrl)
			cs := testCS(conn, nil, nil).(*tcsClientServer)

			tc.setupMock(conn)

			err := cs.publishInstanceStatusOnce(tc.message)

			if tc.expectedError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got: %v", err)
			}
		})
	}
}

// TestPublishInstanceStatusOnceErrorHandling tests error handling in publishInstanceStatusOnce.
func TestPublishInstanceStatusOnceErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		message       ecstcs.InstanceStatusMessage
		setupMock     func(*mock_wsconn.MockWebsocketConn)
		expectedError string
	}{
		{
			name: "MakeRequest fails with connection error",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("connection error"))
			},
			expectedError: "connection error",
		},
		{
			name: "MakeRequest fails with write deadline error",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("IMPAIRED"),
						Type:   aws.String("DOCKER"),
					},
				},
			},
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("write deadline exceeded"))
			},
			expectedError: "write deadline exceeded",
		},
		{
			name: "MakeRequest fails with network timeout",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("production-cluster"),
					ContainerInstance: aws.String("i-1234567890abcdef0"),
					RequestId:         aws.String("req-timeout"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
					{
						Status: aws.String("OK"),
						Type:   aws.String("DOCKER"),
					},
				},
			},
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("network timeout"))
			},
			expectedError: "network timeout",
		},
		{
			name: "MakeRequest fails with SetWriteDeadline error",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
			setupMock: func(mockConn *mock_wsconn.MockWebsocketConn) {
				mockConn.EXPECT().SetWriteDeadline(gomock.Any()).Return(fmt.Errorf("deadline error"))
				// Even when SetWriteDeadline fails, WriteMessage is still called
				mockConn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).Return(fmt.Errorf("deadline error"))
			},
			expectedError: "deadline error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			conn := mock_wsconn.NewMockWebsocketConn(ctrl)
			cs := testCS(conn, nil, nil).(*tcsClientServer)

			tc.setupMock(conn)

			err := cs.publishInstanceStatusOnce(tc.message)

			assert.Error(t, err, "Expected error but got none")
			assert.Contains(t, err.Error(), tc.expectedError, "Error message should contain expected text")
		})
	}
}

// TestPublishInstanceStatusOnceRequestStructure tests proper PublishInstanceStatusRequest creation.
func TestPublishInstanceStatusOnceRequestStructure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		message ecstcs.InstanceStatusMessage
	}{
		{
			name: "request structure with complete metadata",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("test-cluster"),
					ContainerInstance: aws.String("test-instance"),
					RequestId:         aws.String("test-request"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
				},
			},
		},
		{
			name: "request structure with multiple statuses",
			message: ecstcs.InstanceStatusMessage{
				Metadata: &ecstcs.InstanceStatusMetadata{
					Cluster:           aws.String("production-cluster"),
					ContainerInstance: aws.String("i-1234567890abcdef0"),
					RequestId:         aws.String("req-12345"),
				},
				Statuses: []*ecstcs.InstanceStatus{
					{
						Status: aws.String("OK"),
						Type:   aws.String("AGENT"),
					},
					{
						Status: aws.String("IMPAIRED"),
						Type:   aws.String("DOCKER"),
					},
					{
						Status: aws.String("OK"),
						Type:   aws.String("EBS_CSI"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			conn := mock_wsconn.NewMockWebsocketConn(ctrl)
			cs := testCS(conn, nil, nil).(*tcsClientServer)

			// Capture the request structure by examining the WriteMessage call
			conn.EXPECT().SetWriteDeadline(gomock.Any()).Return(nil)
			conn.EXPECT().WriteMessage(gomock.Any(), gomock.Any()).DoAndReturn(
				func(messageType int, data []byte) error {
					// Verify that the request contains the expected structure
					// The data should contain the serialized PublishInstanceStatusRequest
					assert.NotEmpty(t, data, "Request data should not be empty")

					// Verify that the data contains expected fields from the message
					dataStr := string(data)
					if tc.message.Metadata != nil {
						if tc.message.Metadata.Cluster != nil {
							assert.Contains(t, dataStr, *tc.message.Metadata.Cluster, "Request should contain cluster name")
						}
						if tc.message.Metadata.ContainerInstance != nil {
							assert.Contains(t, dataStr, *tc.message.Metadata.ContainerInstance, "Request should contain container instance")
						}
						if tc.message.Metadata.RequestId != nil {
							assert.Contains(t, dataStr, *tc.message.Metadata.RequestId, "Request should contain request ID")
						}
					}

					// Verify that status information is included
					for _, status := range tc.message.Statuses {
						if status.Status != nil {
							assert.Contains(t, dataStr, *status.Status, "Request should contain status value")
						}
						if status.Type != nil {
							assert.Contains(t, dataStr, *status.Type, "Request should contain status type")
						}
					}

					// Verify that timestamp is included (should be present in all requests)
					assert.Contains(t, dataStr, "timestamp", "Request should contain timestamp field")

					return nil
				},
			)

			err := cs.publishInstanceStatusOnce(tc.message)
			assert.NoError(t, err, "Expected no error but got: %v", err)
		})
	}
}
