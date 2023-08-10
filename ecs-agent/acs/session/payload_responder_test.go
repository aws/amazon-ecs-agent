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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// defaultPayloadMessage returns a baseline payload message to be used in testing.
func defaultPayloadMessage() *ecsacs.PayloadMessage {
	return &ecsacs.PayloadMessage{
		Tasks:                []*ecsacs.Task{{}},
		MessageId:            aws.String(testconst.MessageID),
		ClusterArn:           aws.String(testconst.ClusterName),
		ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
	}
}

// TestValidatePayloadMessageWithNilMessage tests if a validation error
// is returned while validating an empty payload message.
func TestValidatePayloadMessageWithNilMessage(t *testing.T) {
	err := validatePayloadMessage(nil)
	assert.Error(t, err, "Expected validation error validating an empty message")
}

// TestValidateInvalidPayloadMessages performs validation on various invalid payload messages.
func TestValidateInvalidPayloadMessages(t *testing.T) {
	testCases := []struct {
		name            string
		messageMutation func(message *ecsacs.PayloadMessage)
		failureMsg      string
	}{
		{
			name: "nil message ID",
			messageMutation: func(message *ecsacs.PayloadMessage) {
				message.MessageId = nil
			},
			failureMsg: "Expected validation error validating a message with no message ID",
		},
		{
			name: "empty message ID",
			messageMutation: func(message *ecsacs.PayloadMessage) {
				message.MessageId = aws.String("")
			},
			failureMsg: "Expected validation error validating a message with empty message ID",
		},
		{
			name: "nil cluster ARN",
			messageMutation: func(message *ecsacs.PayloadMessage) {
				message.ClusterArn = nil
			},
			failureMsg: "Expected validation error validating a message with no cluster ARN",
		},
		{
			name: "empty cluster ARN",
			messageMutation: func(message *ecsacs.PayloadMessage) {
				message.ClusterArn = aws.String("")
			},
			failureMsg: "Expected validation error validating a message with empty cluster ARN",
		},
		{
			name: "nil container instance ARN",
			messageMutation: func(message *ecsacs.PayloadMessage) {
				message.ContainerInstanceArn = nil
			},
			failureMsg: "Expected validation error validating a message with no container instance ARN",
		},
		{
			name: "empty container instance ARN",
			messageMutation: func(message *ecsacs.PayloadMessage) {
				message.ContainerInstanceArn = aws.String("")
			},
			failureMsg: "Expected validation error validating a message with empty container instance ARN",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testMessage := defaultPayloadMessage()
			tc.messageMutation(testMessage)
			err := validatePayloadMessage(testMessage)
			assert.Error(t, err, tc.failureMsg)
		})
	}
}

// TestValidatePayloadMessageSuccess tests if a valid payload message
// is validated without any errors.
func TestValidatePayloadMessageSuccess(t *testing.T) {
	err := validatePayloadMessage(defaultPayloadMessage())
	assert.NoError(t, err, "Error validating payload message: %w", err)
}

// TestPayloadAckHappyPath tests the happy path for a typical PayloadMessage and confirms that the payload responder
// generates the expected ACK after processing a payload message when the payload message contains a task
// with an IAM Role. It also tests if the expected credentials ACK is generated.
func TestPayloadAckHappyPath(t *testing.T) {
	payloadAckSent := make(chan *ecsacs.AckRequest)
	credentialsAckSent := make(chan *ecsacs.IAMRoleCredentialsAckRequest)
	testResponseSender := func(response interface{}) error {
		var credentialsResp *ecsacs.IAMRoleCredentialsAckRequest
		var payloadMessageResp *ecsacs.AckRequest
		credentialsResp, ok := response.(*ecsacs.IAMRoleCredentialsAckRequest)
		if ok {
			credentialsAckSent <- credentialsResp
		} else {
			payloadMessageResp, ok = response.(*ecsacs.AckRequest)
			if ok {
				payloadAckSent <- payloadMessageResp
			} else {
				t.Fatal("response does not hold type ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest")
			}
		}
		return nil
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// The payload message in the test consists of a task, with credentials set.
	taskArn := "t1"
	credentialsExpiration := "expiration"
	credentialsRoleArn := "r1"
	credentialsAccessKey := "akid"
	credentialsSecretKey := "skid"
	credentialsSessionToken := "token"
	credentialsId := "credsid"
	testMessage := defaultPayloadMessage()
	testMessage.Tasks = []*ecsacs.Task{
		{
			Arn: aws.String(taskArn),
			RoleCredentials: &ecsacs.IAMRoleCredentials{
				AccessKeyId:     aws.String(credentialsAccessKey),
				Expiration:      aws.String(credentialsExpiration),
				RoleArn:         aws.String(credentialsRoleArn),
				SecretAccessKey: aws.String(credentialsSecretKey),
				SessionToken:    aws.String(credentialsSessionToken),
				CredentialsId:   aws.String(credentialsId),
			},
		},
	}

	mockPayloadMsgHandler := mock_session.NewMockPayloadMessageHandler(ctrl)
	testPayloadResponder := NewPayloadResponder(mockPayloadMsgHandler, testResponseSender)
	mockPayloadMsgHandler.EXPECT().
		ProcessMessage(gomock.Any(), gomock.Any()).
		Do(func(message *ecsacs.PayloadMessage,
			ackFunc func(*ecsacs.AckRequest, []*ecsacs.IAMRoleCredentialsAckRequest)) {
			assert.NotNil(t, message.Tasks)
			assert.Equal(t, 1, len(message.Tasks))
			ackFunc(&ecsacs.AckRequest{
				MessageId:         message.MessageId,
				Cluster:           message.ClusterArn,
				ContainerInstance: message.ContainerInstanceArn,
			}, []*ecsacs.IAMRoleCredentialsAckRequest{
				{
					MessageId:     message.MessageId,
					Expiration:    message.Tasks[0].RoleCredentials.Expiration,
					CredentialsId: message.Tasks[0].RoleCredentials.CredentialsId,
				},
			})
		}).
		Return(nil)

	// Send a payload message.
	handlePayloadMessage :=
		testPayloadResponder.HandlerFunc().(func(message *ecsacs.PayloadMessage))
	handlePayloadMessage(testMessage)

	// Verify that payload message ACK is sent and is as expected.
	expectedPayloadAck := &ecsacs.AckRequest{
		MessageId:         aws.String(testconst.MessageID),
		Cluster:           aws.String(testconst.ClusterName),
		ContainerInstance: aws.String(testconst.ContainerInstanceARN),
	}
	actualPayloadAck := <-payloadAckSent
	assert.Equal(t, expectedPayloadAck, actualPayloadAck)

	// Verify the credentials ACK for the payload message is sent and is as expected.
	expectedCredentialsAck := &ecsacs.IAMRoleCredentialsAckRequest{
		MessageId:     aws.String(testconst.MessageID),
		Expiration:    aws.String(credentialsExpiration),
		CredentialsId: aws.String(credentialsId),
	}
	actualCredentialsAck := <-credentialsAckSent
	assert.Equal(t, expectedCredentialsAck, actualCredentialsAck)
}
