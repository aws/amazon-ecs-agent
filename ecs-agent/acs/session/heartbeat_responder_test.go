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
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecsacs "github.com/aws/aws-sdk-go-v2/service/acs"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	heartbeatMessageId = "heartbeatMessageId"
)

func TestAckHeartbeatMessage(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatInput{
		MessageId: aws.String(heartbeatMessageId),
		Healthy:   aws.Bool(true),
	}

	heartbeatAckExpected := &ecsacs.HeartbeatOutput{
		MessageId: aws.String(heartbeatMessageId),
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func TestAckHeartbeatMessageNotHealthy(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatInput{
		MessageId: aws.String(heartbeatMessageId),
		// ECS Agent currently ignores this field so we expect no behavior change
		Healthy: aws.Bool(false),
	}

	heartbeatAckExpected := &ecsacs.HeartbeatOutput{
		MessageId: aws.String(heartbeatMessageId),
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func TestAckHeartbeatMessageWithoutMessageId(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatInput{
		Healthy: aws.Bool(true),
	}

	heartbeatAckExpected := &ecsacs.HeartbeatOutput{
		MessageId: nil,
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func TestAckHeartbeatMessageEmpty(t *testing.T) {
	heartbeatReceived := &ecsacs.HeartbeatInput{}

	heartbeatAckExpected := &ecsacs.HeartbeatOutput{
		MessageId: nil,
	}

	validateHeartbeatAck(t, heartbeatReceived, heartbeatAckExpected)
}

func validateHeartbeatAck(t *testing.T, heartbeatReceived *ecsacs.HeartbeatInput, heartbeatAckExpected *ecsacs.HeartbeatOutput) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ackSent := make(chan *ecsacs.HeartbeatOutput)

	emptyHealthchecksList := []doctor.Healthcheck{}
	emptyDoctor, _ := doctor.NewDoctor(emptyHealthchecksList, "testCluster", "this:is:an:instance:arn")

	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.HeartbeatOutput)
		ackSent <- resp
		return nil
	}
	testHeartbeatResponder := NewHeartbeatResponder(emptyDoctor, testResponseSender)
	testHeartbeatResponder.(*heartbeatResponder).processHeartbeatMessage(heartbeatReceived)

	// wait till we send an
	heartbeatAckSent := <-ackSent

	require.Equal(t, heartbeatAckExpected, heartbeatAckSent)
}
