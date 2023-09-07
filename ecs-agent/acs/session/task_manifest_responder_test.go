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

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	mock_session "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	StartingTaskManifestSequenceNumber = 15
	StaleTaskManifestSequenceNumber    = 10
)

// setupTestManifestMessage initializes a dummy TaskManifestMessage for testing
func setupTestManifestMessage() *ecsacs.TaskManifestMessage {
	return &ecsacs.TaskManifestMessage{
		ClusterArn:           aws.String(testconst.ClusterARN),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
		MessageId:            aws.String(testconst.MessageID),
		Tasks:                []*ecsacs.TaskIdentifier{},
		Timeline:             aws.Int64(StartingTaskManifestSequenceNumber),
	}
}

// TestManifestAckHappyPath tests the happy path for a typical TaskManifestMessage and confirms
// expected ack is made
func TestManifestAckHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testManifest := setupTestManifestMessage()

	testResponseSender := func(response interface{}) error {
		ackRequest, isAckRequest := response.(*ecsacs.AckRequest)
		if isAckRequest {
			// Validate ack request fields when happy path is reached.
			require.Equal(t, aws.StringValue(ackRequest.MessageId), testconst.MessageID)
		} else {
			stopVerification, isStopVerification := response.(*ecsacs.TaskStopVerificationMessage)
			if isStopVerification {
				// We expect only one task marked for termination.
				require.Equal(t, len(stopVerification.StopCandidates), 1)
				require.Equal(t, aws.StringValue(stopVerification.StopCandidates[0].TaskArn), testconst.TaskARN)
			}
		}
		return nil
	}

	mockMetrics := mock_metrics.NewMockEntryFactory(ctrl)
	mockComparer := mock_session.NewMockTaskComparer(ctrl)
	mockIDA := mock_session.NewMockManifestMessageIDAccessor(ctrl)
	mockSNA := mock_session.NewMockSequenceNumberAccessor(ctrl)

	testTaskManifestResponder := NewTaskManifestResponder(mockComparer, mockSNA, mockIDA, mockMetrics, testResponseSender)

	// Expect to get the latest sequence number for stale check, then set it as we expect.
	mockSNA.EXPECT().GetLatestSequenceNumber()
	mockSNA.EXPECT().SetLatestSequenceNumber(*testManifest.Timeline).Return(nil)

	// After we get the latest sequence number, we will expect to set the latest message ID.
	mockIDA.EXPECT().SetMessageID(testconst.MessageID).Return(nil)

	// Test Metric publication when our responder starts and finishes message processing.
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	mockMetrics.EXPECT().New(metrics.TaskManifestHandlingDuration).Return(mockEntry)
	mockEntry.EXPECT().Done(nil).AnyTimes()

	// When the client queries tasks to be stopped, inject this task to be stopped.
	mockComparer.EXPECT().CompareRunningTasksOnInstanceWithManifest(testManifest).Return(
		[]*ecsacs.TaskIdentifier{
			{DesiredStatus: aws.String("STOPPED"), TaskArn: aws.String(testconst.TaskARN)},
		},
		nil,
	)

	// Handle the task manifest message update.
	handleManifestMessage := testTaskManifestResponder.HandlerFunc().(func(*ecsacs.TaskManifestMessage))
	handleManifestMessage(testManifest)
}

// TestTaskManifestStaleMessage will make sure the manifest can update, retrieve and catch stale
// Manifest sequence numbers appropriately.
func TestTaskManifestStaleMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testResponseSender := func(response interface{}) error {
		return nil
	}

	mockComparer := mock_session.NewMockTaskComparer(ctrl)
	mockIDA := mock_session.NewMockManifestMessageIDAccessor(ctrl)
	mockSNA := mock_session.NewMockSequenceNumberAccessor(ctrl)

	testTaskManifestResponder := NewTaskManifestResponder(mockComparer, mockSNA, mockIDA, metrics.NewNopEntryFactory(), testResponseSender)

	// Create test task manifest.
	testManifest := setupTestManifestMessage()

	// Set up a new manifest with a stale number and distinct message ID.
	newManifest := &ecsacs.TaskManifestMessage{
		ClusterArn:           aws.String(testconst.ClusterARN),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
		MessageId:            aws.String("456"),
		Tasks:                []*ecsacs.TaskIdentifier{},
		Timeline:             aws.Int64(StaleTaskManifestSequenceNumber),
	}

	// Expect GetLatestSequenceNumber once in each call to handleManifestMessage.
	// The first time, LatestSequenceNumber has not been set, so allow it to pass by comparing
	// against an older one.
	mockSNA.EXPECT().GetLatestSequenceNumber().Return(int64(StaleTaskManifestSequenceNumber))
	mockSNA.EXPECT().GetLatestSequenceNumber().Return(int64(StartingTaskManifestSequenceNumber))

	// The test manifest should be valid, updating sequence number and message ID only once.
	mockSNA.EXPECT().SetLatestSequenceNumber(*testManifest.Timeline)
	mockIDA.EXPECT().SetMessageID(testconst.MessageID)
	mockComparer.EXPECT().CompareRunningTasksOnInstanceWithManifest(testManifest).Return([]*ecsacs.TaskIdentifier{}, nil)

	handleManifestMessage := testTaskManifestResponder.HandlerFunc().(func(*ecsacs.TaskManifestMessage))

	// Handle the task manifest message update, this should correctly set the message ID and sequence number.
	handleManifestMessage(testManifest)

	// Now try to update manifest with a stale message. The responder should discard and ignore this message.
	handleManifestMessage(newManifest)
}
