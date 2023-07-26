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
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	mock_metrics "github.com/aws/amazon-ecs-agent/ecs-agent/metrics/mocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// defaulTestRefreshCredentialsMessage returns a baseline refresh credentials message to be used in testing.
func defaulTestRefreshCredentialsMessage() *ecsacs.IAMRoleCredentialsMessage {
	return &ecsacs.IAMRoleCredentialsMessage{
		MessageId: aws.String(testconst.MessageID),
		TaskArn:   aws.String(testconst.TaskARN),
		RoleCredentials: &ecsacs.IAMRoleCredentials{
			CredentialsId: aws.String(testconst.CredentialsID),
		},
		RoleType: aws.String(credentials.ApplicationRoleType),
	}
}

// TestValidateRefreshMessageWithNilMessage tests if a validation error
// is returned while validating an empty credentials message.
func TestValidateRefreshMessageWithNilMessage(t *testing.T) {
	err := validateIAMRoleCredentialsMessage(nil)
	assert.Error(t, err, "Expected validation error validating an empty message")
}

// TestValidateInvalidRefreshCredentialsMessage performs validation on various invalid refresh credentials messages.
func TestValidateInvalidRefreshMessages(t *testing.T) {
	testCases := []struct {
		name            string
		messageMutation func(message *ecsacs.IAMRoleCredentialsMessage)
		failureMsg      string
	}{
		{
			name: "nil message ID",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.MessageId = nil
			},
			failureMsg: "Expected validation error validating a message with no message ID",
		},
		{
			name: "empty message ID",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.MessageId = aws.String("")
			},
			failureMsg: "Expected validation error validating a message with empty message ID",
		},
		{
			name: "nil task ARN",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.TaskArn = nil
			},
			failureMsg: "Expected validation error validating a message with no task ARN",
		},
		{
			name: "empty task ARN",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.TaskArn = aws.String("")
			},
			failureMsg: "Expected validation error validating a message with empty task ARN",
		},
		{
			name: "nil role credentials",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.RoleCredentials = nil
			},
			failureMsg: "Expected validation error validating a message with no role credentials",
		},
		{
			name: "nil credentials ID",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.RoleCredentials = &ecsacs.IAMRoleCredentials{}
			},
			failureMsg: "Expected validation error validating a message with no credentials ID",
		},
		{
			name: "empty credentials ID",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.RoleCredentials = &ecsacs.IAMRoleCredentials{CredentialsId: aws.String("")}
			},
			failureMsg: "Expected validation error validating a message with empty credentials ID",
		},
		{
			name: "invalid role type",
			messageMutation: func(message *ecsacs.IAMRoleCredentialsMessage) {
				message.RoleType = aws.String("not a valid role type")
			},
			failureMsg: "Expected validation error validating a message with an invalid role type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testMessage := defaulTestRefreshCredentialsMessage()
			tc.messageMutation(testMessage)
			err := validateIAMRoleCredentialsMessage(testMessage)
			assert.Error(t, err, tc.failureMsg)
		})
	}
}

// TestValidateRefreshMessageSuccess tests if a valid credentials message
// is validated without any errors.
func TestValidateRefreshMessageSuccess(t *testing.T) {
	testMessage := defaulTestRefreshCredentialsMessage()

	err := validateIAMRoleCredentialsMessage(testMessage)
	assert.NoError(t, err, "Error validating credentials message: %w", err)
}

// TestRefreshCredentialsAckHappyPath tests the happy path for a typical IAMRoleCredentialsMessage and confirms expected
// ACK request is made.
func TestRefreshCredentialsAckHappyPath(t *testing.T) {
	testCases := []struct {
		name     string
		roleType string
	}{
		{
			name:     "task role type",
			roleType: credentials.ApplicationRoleType,
		},
		{
			name:     "execution role type",
			roleType: credentials.ExecutionRoleType,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			testMessage := defaulTestRefreshCredentialsMessage()
			var ackSent *ecsacs.IAMRoleCredentialsAckRequest
			credentialsManager := credentials.NewManager()
			mockCredsMetadataSetter := mock_session.NewMockCredentialsMetadataSetter(ctrl)
			switch tc.roleType {
			case credentials.ApplicationRoleType:
				testMessage.RoleType = aws.String(credentials.ApplicationRoleType)
				mockCredsMetadataSetter.EXPECT().
					SetTaskRoleMetadata(gomock.Any()).
					Return(nil)
			case credentials.ExecutionRoleType:
				testMessage.RoleType = aws.String(credentials.ExecutionRoleType)
				mockCredsMetadataSetter.EXPECT().
					SetExecRoleMetadata(gomock.Any()).
					Return(nil)
			default:
				t.Fatal("invalid role type used in happy path test, role type should be valid for happy path")
				return
			}
			mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
			mockEntry := mock_metrics.NewMockEntry(ctrl)
			mockEntry.EXPECT().WithFields(gomock.Any()).Return(mockEntry)
			mockEntry.EXPECT().Done(nil)
			mockMetricsFactory.EXPECT().New(metrics.CredentialsRefreshSuccess).Return(mockEntry)

			testResponseSender := func(response interface{}) error {
				ackSent = response.(*ecsacs.IAMRoleCredentialsAckRequest)
				return nil
			}
			testRefreshCredentialsResponder := NewRefreshCredentialsResponder(credentialsManager,
				mockCredsMetadataSetter,
				mockMetricsFactory,
				testResponseSender)

			handleCredentialsMessage :=
				testRefreshCredentialsResponder.HandlerFunc().(func(*ecsacs.IAMRoleCredentialsMessage))

			handleCredentialsMessage(testMessage)

			assert.Equal(t, aws.StringValue(testMessage.MessageId),
				aws.StringValue(ackSent.MessageId))

			creds, exist := credentialsManager.GetTaskCredentials(testconst.CredentialsID)
			assert.True(t, exist, "Expected credentials to exist for the task")
			assert.Equal(t, aws.StringValue(testMessage.RoleCredentials.CredentialsId),
				creds.IAMRoleCredentials.CredentialsID)
		})
	}
}

// TestRefreshCredentialsWhenUnableToSetCredentialsMetadata tests the error case where the responder is not able to
// successfully set credentials metadata.
func TestRefreshCredentialsWhenUnableToSetCredentialsMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testMessage := defaulTestRefreshCredentialsMessage()
	ackSent := false
	credentialsManager := credentials.NewManager()
	mockCredsMetadataSetter := mock_session.NewMockCredentialsMetadataSetter(ctrl)
	mockCredsMetadataSetter.EXPECT().
		SetTaskRoleMetadata(gomock.Any()).
		Return(errors.Errorf("unable to set credentials metadata"))
	mockMetricsFactory := mock_metrics.NewMockEntryFactory(ctrl)
	mockEntry := mock_metrics.NewMockEntry(ctrl)
	mockEntry.EXPECT().WithFields(gomock.Any()).Return(mockEntry)
	mockEntry.EXPECT().Done(gomock.Any())
	mockMetricsFactory.EXPECT().New(metrics.CredentialsRefreshFailure).Return(mockEntry)

	testResponseSender := func(response interface{}) error {
		ackSent = true
		return nil
	}
	testRefreshCredentialsResponder := NewRefreshCredentialsResponder(credentialsManager,
		mockCredsMetadataSetter,
		mockMetricsFactory,
		testResponseSender)

	handleCredentialsMessage :=
		testRefreshCredentialsResponder.HandlerFunc().(func(*ecsacs.IAMRoleCredentialsMessage))

	handleCredentialsMessage(testMessage)
	assert.False(t, ackSent,
		"Expected no ACK of refresh credentials message when unable to successfully set credentials metadata")
}
