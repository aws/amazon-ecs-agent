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

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	mock_session "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TO-DO: move below from agent and ecs-agent to testconst
const (
	dummyInt = 123
)

var testStopVerificationAck = &ecsacs.TaskStopVerificationAck{
	GeneratedAt: aws.Int64(dummyInt),
	MessageId:   aws.String(testconst.MessageID),
	StopTasks: []*ecsacs.TaskIdentifier{
		{
			TaskArn:        aws.String(testconst.TaskARN),
			TaskClusterArn: aws.String(testconst.ClusterName),
			DesiredStatus:  aws.String("STOPPED"),
		},
	},
}

// TestTaskStopVerificationAckResponderStopsTasks tests the case where a task must be stopped
// upon receiving a task stop verification ACK.
func TestTaskStopVerificationAckResponderStopsTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskStopVerificationAckTaskHasBeenStopped := false

	taskStopper := mock_session.NewMockTaskStopper(ctrl)
	manifestMessageIDAccessor := mock_session.NewMockManifestMessageIDAccessor(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	mockEntry.EXPECT().WithFields(gomock.Any()).Return(mockEntry)
	mockEntry.EXPECT().Done(nil)

	gomock.InOrder(
		manifestMessageIDAccessor.EXPECT().GetMessageID().Return(testconst.MessageID),
		manifestMessageIDAccessor.EXPECT().SetMessageID(""),
		metricsFactory.EXPECT().New(metrics.TaskStoppedMetricName).Return(mockEntry),
		taskStopper.EXPECT().StopTask(aws.StringValue(testStopVerificationAck.StopTasks[0].TaskArn)).
			Do(func(interface{}) {
				taskStopVerificationAckTaskHasBeenStopped = true
			}),
	)

	taskStopVerificationAckResponder := NewTaskStopVerificationACKResponder(
		taskStopper,
		manifestMessageIDAccessor,
		metricsFactory)
	handleTaskStopVerificationAck :=
		taskStopVerificationAckResponder.HandlerFunc().(func(message *ecsacs.TaskStopVerificationAck))
	handleTaskStopVerificationAck(testStopVerificationAck)

	assert.True(t, taskStopVerificationAckTaskHasBeenStopped)
}

// TestTaskStopVerificationAckResponderStopsTasks tests the case where an invalid
// task stop verification ACK is received.
func TestTaskStopVerificationAckResponderInvalidAck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskStopper := mock_session.NewMockTaskStopper(ctrl)
	manifestMessageIDAccessor := mock_session.NewMockManifestMessageIDAccessor(ctrl)
	metricsFactory := mock_metrics.NewMockEntryFactory(ctrl)

	differentMessageID := "000"

	gomock.InOrder(
		manifestMessageIDAccessor.EXPECT().GetMessageID().Return(differentMessageID),
		// Include statements below with .Times(0) to explicitly assert that we expect these methods not to be called.
		manifestMessageIDAccessor.EXPECT().SetMessageID(gomock.Any()).Times(0),
		metricsFactory.EXPECT().New(gomock.Any()).Times(0),
		taskStopper.EXPECT().StopTask(gomock.Any()).Times(0),
	)

	taskStopVerificationAckResponder := NewTaskStopVerificationACKResponder(
		taskStopper,
		manifestMessageIDAccessor,
		metricsFactory)
	handleTaskStopVerificationAck :=
		taskStopVerificationAckResponder.HandlerFunc().(func(message *ecsacs.TaskStopVerificationAck))
	handleTaskStopVerificationAck(testStopVerificationAck)

	assert.NotEqual(t, aws.StringValue(testStopVerificationAck.MessageId), differentMessageID)
}
