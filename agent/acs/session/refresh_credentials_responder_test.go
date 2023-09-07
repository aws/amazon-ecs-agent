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
	"fmt"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	acssession "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	expiration   = "soon"
	roleArn      = "taskrole1"
	accessKey    = "akid"
	secretKey    = "secret"
	sessionToken = "token"
	roleType     = "TaskExecution"
)

var expectedCredentialsAck = &ecsacs.IAMRoleCredentialsAckRequest{
	Expiration:    aws.String(expiration),
	MessageId:     aws.String(testconst.MessageID),
	CredentialsId: aws.String(testconst.CredentialsID),
}

var expectedCredentials = credentials.TaskIAMRoleCredentials{
	ARN: testconst.TaskARN,
	IAMRoleCredentials: credentials.IAMRoleCredentials{
		RoleArn:         roleArn,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		SessionToken:    sessionToken,
		Expiration:      expiration,
		CredentialsID:   testconst.CredentialsID,
		RoleType:        roleType,
	},
}

var testRefreshCredentialsMessage = &ecsacs.IAMRoleCredentialsMessage{
	MessageId: aws.String(testconst.MessageID),
	TaskArn:   aws.String(testconst.TaskARN),
	RoleType:  aws.String(roleType),
	RoleCredentials: &ecsacs.IAMRoleCredentials{
		RoleArn:         aws.String(roleArn),
		Expiration:      aws.String(expiration),
		AccessKeyId:     aws.String(accessKey),
		SecretAccessKey: aws.String(secretKey),
		SessionToken:    aws.String(sessionToken),
		CredentialsId:   aws.String(testconst.CredentialsID),
	},
}

// TestInvalidCredentialsMessageNotAcked tests that invalid credential message
// is not ACKed.
func TestInvalidCredentialsMessageNotAcked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ackSent := false
	testResponseSender := func(response interface{}) error {
		ackSent = true
		return nil
	}
	testRefreshCredentialsResponder := acssession.NewRefreshCredentialsResponder(credentials.NewManager(),
		NewCredentialsMetadataSetter(nil),
		metrics.NewNopEntryFactory(),
		testResponseSender)

	handleCredentialsMessage := testRefreshCredentialsResponder.HandlerFunc().(func(*ecsacs.IAMRoleCredentialsMessage))

	// Test handling a credentials message without any fields set.
	message := &ecsacs.IAMRoleCredentialsMessage{}
	handleCredentialsMessage(message)
	assert.False(t, ackSent,
		"Expected no ACK of invalid refresh credentials message when it is invalid")
}

// TestCredentialsMessageNotAckedWhenTaskNotFound tests if credential messages
// are not ACKed when the task ARN in the message is not found in the task
// engine.
func TestCredentialsMessageNotAckedWhenTaskNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ackSent := false
	mockTaskEngine := mock_engine.NewMockTaskEngine(ctrl)
	testResponseSender := func(response interface{}) error {
		ackSent = true
		return nil
	}
	testRefreshCredentialsResponder := acssession.NewRefreshCredentialsResponder(credentials.NewManager(),
		NewCredentialsMetadataSetter(mockTaskEngine),
		metrics.NewNopEntryFactory(),
		testResponseSender)

	handleCredentialsMessage := testRefreshCredentialsResponder.HandlerFunc().(func(*ecsacs.IAMRoleCredentialsMessage))

	// Test handling a credentials message with a task ARN that is not in the task engine.
	mockTaskEngine.EXPECT().GetTaskByArn(testconst.TaskARN).Return(nil, false)
	handleCredentialsMessage(testRefreshCredentialsMessage)
	assert.False(t, ackSent,
		"Expected no ACK of invalid refresh credentials message when its task ARN is not in task engine")
}

// TestHandleRefreshMessageAckedWhenCredentialsUpdated tests that a credential message
// is ACKed when the credentials are updated successfully and the domainless gMSA plugin credentials
// are updated successfully.
func TestHandleRefreshMessageAckedWhenCredentialsUpdated(t *testing.T) {
	testCases := []struct {
		name       string
		taskArn    string
		containers []*apicontainer.Container
	}{
		{
			name:       "EmptyTaskSucceeds",
			taskArn:    testconst.TaskARN,
			containers: []*apicontainer.Container{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ackSent := make(chan *ecsacs.IAMRoleCredentialsAckRequest)
			credentialsManager := credentials.NewManager()
			mockTaskEngine := mock_engine.NewMockTaskEngine(ctrl)

			testResponseSender := func(response interface{}) error {
				resp := response.(*ecsacs.IAMRoleCredentialsAckRequest)
				ackSent <- resp
				return nil
			}
			testRefreshCredentialsResponder := acssession.NewRefreshCredentialsResponder(credentialsManager,
				NewCredentialsMetadataSetter(mockTaskEngine),
				metrics.NewNopEntryFactory(),
				testResponseSender)

			handleCredentialsMessage :=
				testRefreshCredentialsResponder.HandlerFunc().(func(*ecsacs.IAMRoleCredentialsMessage))

			checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = func(
				iamRoleCredentials credentials.IAMRoleCredentials, task *apitask.Task) error {
				if tc.taskArn != task.Arn {
					return errors.New(fmt.Sprintf("Expected taskArnInput to be %s, instead got %s", tc.taskArn,
						task.Arn))
				}

				return nil
			}

			defer func() {
				checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl =
					checkAndSetDomainlessGMSATaskExecutionRoleCredentials
			}()

			// Return a task from the engine for GetTaskByArn.
			mockTaskEngine.EXPECT().GetTaskByArn(tc.taskArn).Return(
				&apitask.Task{Arn: tc.taskArn, Containers: tc.containers}, true)

			go handleCredentialsMessage(testRefreshCredentialsMessage)

			refreshCredentialsAckSent := <-ackSent
			assert.Equal(t, expectedCredentialsAck, refreshCredentialsAckSent)

			creds, exist := credentialsManager.GetTaskCredentials(testconst.CredentialsID)
			assert.True(t, exist, "Expected credentials to exist for the task")
			assert.Equal(t, expectedCredentials, creds)
		})
	}
}

// TestCredentialsMessageNotAckedWhenDomainlessGMSACredentialsNotSet tests that credential messages
// are not ACKed when setting the domainless GMSA Credentials fails.
func TestCredentialsMessageNotAckedWhenDomainlessGMSACredentialsError(t *testing.T) {
	testCases := []struct {
		name                                                   string
		taskArn                                                string
		containers                                             []*apicontainer.Container
		setDomainlessGMSATaskExecutionRoleCredentialsImplError error
	}{
		{
			name:    "ErrDomainlessTask",
			taskArn: testconst.TaskARN,
			containers: []*apicontainer.Container{
				{
					CredentialSpecs: []string{"credentialspecdomainless:file://gmsa_gmsa-acct.json"},
				},
			},
			setDomainlessGMSATaskExecutionRoleCredentialsImplError: errors.New(
				"mock setDomainlessGMSATaskExecutionRoleCredentialsImplError"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var ackSent, errorOnSetDomainlessGMSACreds bool
			credentialsManager := credentials.NewManager()
			mockTaskEngine := mock_engine.NewMockTaskEngine(ctrl)

			testResponseSender := func(response interface{}) error {
				ackSent = true
				return nil
			}
			testRefreshCredentialsResponder := acssession.NewRefreshCredentialsResponder(credentialsManager,
				NewCredentialsMetadataSetter(mockTaskEngine),
				metrics.NewNopEntryFactory(),
				testResponseSender)

			handleCredentialsMessage :=
				testRefreshCredentialsResponder.HandlerFunc().(func(*ecsacs.IAMRoleCredentialsMessage))

			checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = func(
				iamRoleCredentials credentials.IAMRoleCredentials, task *apitask.Task) error {
				if tc.taskArn != task.Arn {
					return errors.New(fmt.Sprintf("Expected taskArnInput to be %s, instead got %s", tc.taskArn, task.Arn))
				}
				errorOnSetDomainlessGMSACreds = true
				return tc.setDomainlessGMSATaskExecutionRoleCredentialsImplError
			}

			defer func() {
				checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl =
					checkAndSetDomainlessGMSATaskExecutionRoleCredentials
			}()

			// Return a task from the engine for GetTaskByArn.
			mockTaskEngine.EXPECT().GetTaskByArn(tc.taskArn).Return(
				&apitask.Task{Arn: tc.taskArn, Containers: tc.containers}, true)

			handleCredentialsMessage(testRefreshCredentialsMessage)
			assert.True(t, errorOnSetDomainlessGMSACreds,
				"Expected error when setting the domainless GMSA Credentials")
			assert.False(t, ackSent,
				"Expected no ACK of refresh credentials message when setting domainless GMSA Credentials fails")
		})
	}
}
