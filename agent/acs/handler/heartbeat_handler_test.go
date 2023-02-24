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

package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/doctor"
	mock_wsclient "github.com/aws/amazon-ecs-agent/agent/wsclient/mock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	heartbeatMessageId = "heartbeatMessageId"
)

func TestAckHeartbeatMessage(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatMessage{
		MessageId: aws.String(heartbeatMessageId),
		Healthy:   aws.Bool(true),
	}

	heartbeatAckExpected := &ecsacs.HeartbeatAckRequest{
		MessageId: aws.String(heartbeatMessageId),
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func TestAckHeartbeatMessageNotHealthy(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatMessage{
		MessageId: aws.String(heartbeatMessageId),
		// ECS Agent currently ignores this field so we expect no behavior change
		Healthy: aws.Bool(false),
	}

	heartbeatAckExpected := &ecsacs.HeartbeatAckRequest{
		MessageId: aws.String(heartbeatMessageId),
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func TestAckHeartbeatMessageWithoutMessageId(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatMessage{
		Healthy: aws.Bool(true),
	}

	heartbeatAckExpected := &ecsacs.HeartbeatAckRequest{
		MessageId: nil,
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func TestAckHeartbeatMessageEmpty(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatMessage{}

	heartbeatAckExpected := &ecsacs.HeartbeatAckRequest{
		MessageId: nil,
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func validateHeartbeatAck(t *testing.T, heartbeatReceived *ecsacs.HeartbeatMessage, heartbeatAckExpected *ecsacs.HeartbeatAckRequest) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	var heartbeatAckSent *ecsacs.HeartbeatAckRequest

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(message *ecsacs.HeartbeatAckRequest) {
		heartbeatAckSent = message
		cancel()
	}).Times(1)

	dockerClient := mock_dockerapi.NewMockDockerClient(ctrl)
	dockerClient.EXPECT().SystemPing(gomock.Any(), gomock.Any()).AnyTimes()

	emptyHealthchecksList := []doctor.Healthcheck{}
	emptyDoctor, _ := doctor.NewDoctor(emptyHealthchecksList, "testCluster", "this:is:an:instance:arn")

	handler := newHeartbeatHandler(ctx, mockWsClient, emptyDoctor)

	go handler.sendHeartbeatAck()

	handler.handleSingleHeartbeatMessage(heartbeatReceived)

	// wait till we get an ack from heartbeatAckMessageBuffer
	<-ctx.Done()

	require.Equal(t, heartbeatAckExpected, heartbeatAckSent)
}

func TestHeartbeatHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	emptyHealthCheckList := []doctor.Healthcheck{}
	emptyDoctor, _ := doctor.NewDoctor(emptyHealthCheckList, "testCluster",
		"this:is:an:instance:arn")
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Return(nil).Times(1)
	handler := newHeartbeatHandler(ctx, mockWSClient, emptyDoctor)

	wg := sync.WaitGroup{}
	wg.Add(2)

	// write a dummy ack into the heartbeatAckMessageBuffer
	go func() {
		handler.heartbeatAckMessageBuffer <- &ecsacs.HeartbeatAckRequest{}
		wg.Done()
	}()

	// sleep here to ensure that the sending go routine executes before the receiving one below. if not, then the
	// receiving go routine will finish without receiving the ack since sendPendingHeartbeatAck() is non-blocking.
	time.Sleep(1 * time.Second)

	go func() {
		handler.sendPendingHeartbeatAck()
		wg.Done()
	}()

	// wait for both go routines above to finish before we verify that ack channel is empty and exit the test.
	// this also ensures that the mock MakeRequest call happened as expected.
	wg.Wait()

	// verify that the heartbeatAckMessageBuffer channel is empty
	assert.Equal(t, 0, len(handler.heartbeatAckMessageBuffer))
}
