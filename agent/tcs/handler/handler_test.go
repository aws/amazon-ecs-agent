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

package tcshandler

import (
	"errors"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/mocks"
	"github.com/aws/amazon-ecs-agent/agent/tcs/client"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/mock/utils"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/golang/mock/gomock"
)

const (
	testTaskArn                = "arn:aws:ecs:us-east-1:123:task/def"
	testTaskDefinitionFamily   = "task-def"
	testClusterArn             = "arn:aws:ecs:us-east-1:123:cluster/default"
	testInstanceArn            = "arn:aws:ecs:us-east-1:123:container-instance/abc"
	testMessageId              = "testMessageId"
	testPublishMetricsInterval = 1 * time.Millisecond
)

type mockStatsEngine struct{}

func (engine *mockStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	req := createPublishMetricsRequest()
	return req.Metadata, req.TaskMetrics, nil
}

func TestFormatURL(t *testing.T) {
	endpoint := "http://127.0.0.0.1/"
	wsurl := formatURL(endpoint, testClusterArn, testInstanceArn)

	parsed, err := url.Parse(wsurl)
	if err != nil {
		t.Fatal("Should be able to parse url")
	}

	if parsed.Path != "/ws" {
		t.Fatal("Wrong path")
	}

	if parsed.Query().Get("cluster") != testClusterArn {
		t.Fatal("Wrong cluster")
	}
	if parsed.Query().Get("containerInstance") != testInstanceArn {
		t.Fatal("Wrong cluster")
	}
}

func TestStartSession(t *testing.T) {
	// Start test server.
	closeWS := make(chan bool)
	server, serverChan, requestChan, serverErr, err := mockwsutils.StartMockServer(t, closeWS)
	defer server.Close()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		t.Error(<-serverErr)
	}()
	defer func() {
		closeWS <- true
		close(serverChan)
	}()

	// Start a session with the test server.
	go startSession(server.URL, "us-east-1", credentials.AnonymousCredentials, true, &mockStatsEngine{}, testPublishMetricsInterval)

	// startSession internally starts publishing metrics from the mockStatsEngine object.
	time.Sleep(testPublishMetricsInterval)

	// Read request channel to get the metric data published to the server.
	request := <-requestChan

	// Decode and verify the metric data.
	payload, err := getPayloadFromRequest(request)
	if err != nil {
		t.Fatal("Error decoding payload: ", err)
	}

	// Decode and verify the metric data.
	_, responseType, err := wsclient.DecodeData([]byte(payload), &tcsclient.TcsDecoder{})
	if err != nil {
		t.Fatal("error decoding data: ", err)
	}
	if responseType != "PublishMetricsRequest" {
		t.Fatal("Unexpected responseType: ", responseType)
	}
}

func TestSessionConenctionClosedByRemote(t *testing.T) {
	// Start test server.
	closeWS := make(chan bool)
	server, serverChan, _, serverErr, err := mockwsutils.StartMockServer(t, closeWS)
	defer server.Close()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		t.Error(<-serverErr)
	}()
	sleepBeforeClose := 10 * time.Millisecond
	go func() {
		time.Sleep(sleepBeforeClose)
		closeWS <- true
		close(serverChan)
	}()

	// Start a session with the test server.
	err = startSession(server.URL, "us-east-1", credentials.AnonymousCredentials, true, &mockStatsEngine{}, testPublishMetricsInterval)

	if err == nil {
		t.Error("Expected io.EOF on closed connection")
	}
	if err != io.EOF {
		t.Error("Expected io.EOF on closed connection, got: ", err)
	}
}

func TestDiscoverEndpointAndStartSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEcs := mock_api.NewMockECSClient(ctrl)
	mockEcs.EXPECT().DiscoverTelemetryEndpoint(gomock.Any()).Return("", errors.New("error"))

	err := startTelemetrySession(TelemetrySessionParams{EcsClient: mockEcs}, nil)
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
			&ecstcs.TaskMetric{
				ContainerMetrics: []*ecstcs.ContainerMetric{
					&ecstcs.ContainerMetric{
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
