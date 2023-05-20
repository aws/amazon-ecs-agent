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
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	mock_wsclient "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/mock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	messageId         = "message1"
	taskArn           = "task1"
	cluster           = "default"
	containerInstance = "instance"
	expiration        = "soon"
	roleArn           = "taskrole1"
	accessKey         = "akid"
	secretKey         = "secret"
	sessionToken      = "token"
	credentialsId     = "credsid"
	roleType          = "TaskExecution"
)

var expectedAck = &ecsacs.IAMRoleCredentialsAckRequest{
	Expiration:    aws.String(expiration),
	MessageId:     aws.String(messageId),
	CredentialsId: aws.String(credentialsId),
}

var expectedCredentials = credentials.TaskIAMRoleCredentials{
	ARN: taskArn,
	IAMRoleCredentials: credentials.IAMRoleCredentials{
		RoleArn:         roleArn,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		SessionToken:    sessionToken,
		Expiration:      expiration,
		CredentialsID:   credentialsId,
		RoleType:        roleType,
	},
}

var message = &ecsacs.IAMRoleCredentialsMessage{
	MessageId: aws.String(messageId),
	TaskArn:   aws.String(taskArn),
	RoleType:  aws.String(roleType),
	RoleCredentials: &ecsacs.IAMRoleCredentials{
		RoleArn:         aws.String(roleArn),
		Expiration:      aws.String(expiration),
		AccessKeyId:     aws.String(accessKey),
		SecretAccessKey: aws.String(secretKey),
		SessionToken:    aws.String(sessionToken),
		CredentialsId:   aws.String(credentialsId),
	},
}

// TestValidateRefreshMessageWithNilMessage tests if a validation error
// is returned while validating an empty credentials message
func TestValidateRefreshMessageWithNilMessage(t *testing.T) {
	err := validateIAMRoleCredentialsMessage(nil)
	if err == nil {
		t.Error("Expected validation error validating an empty message")
	}
}

// TestValidateRefreshMessageWithNoMessageId tests if a validation error
// is returned while validating a credentials message with no message id
func TestValidateRefreshMessageWithNoMessageId(t *testing.T) {
	message := &ecsacs.IAMRoleCredentialsMessage{}
	err := validateIAMRoleCredentialsMessage(message)
	if err == nil {
		t.Error("Expected validation error validating a message with no message id")
	}
	message.MessageId = aws.String("")
	err = validateIAMRoleCredentialsMessage(message)
	if err == nil {
		t.Error("Expected validation error validating a message with empty  message id")
	}
}

// TestValidateRefreshMessageWithNoRoleCredentials tests if a validation error
// is returned while validating a credentials message with no role credentials
func TestValidateRefreshMessageWithNoRoleCredentials(t *testing.T) {
	message := &ecsacs.IAMRoleCredentialsMessage{
		MessageId: aws.String(messageId),
	}
	err := validateIAMRoleCredentialsMessage(message)
	if err == nil {
		t.Error("Expected validation error validating a message with no role credentials")
	}
}

// TestValidateRefreshMessageWithNoCredentialsId tests if a valid error
// is returned while validating a credentials message with no credentials id
func TestValidateRefreshMessageWithNoCredentialsId(t *testing.T) {
	message := &ecsacs.IAMRoleCredentialsMessage{
		MessageId:       aws.String(messageId),
		RoleCredentials: &ecsacs.IAMRoleCredentials{},
	}
	err := validateIAMRoleCredentialsMessage(message)
	if err == nil {
		t.Error("Expected validation error validating a message with no credentials id")
	}
	message.RoleCredentials.CredentialsId = aws.String("")
	err = validateIAMRoleCredentialsMessage(message)
	if err == nil {
		t.Error("Expected validation error validating a message with empty credentials id")
	}
}

// TestValidateRefreshMessageWithNoTaskArn tests if a validation error
// is returned while validating a credentials message with no task arn
func TestValidateRefreshMessageWithNoTaskArn(t *testing.T) {
	message := &ecsacs.IAMRoleCredentialsMessage{
		MessageId: aws.String(messageId),
		RoleCredentials: &ecsacs.IAMRoleCredentials{
			CredentialsId: aws.String("id"),
		},
	}
	err := validateIAMRoleCredentialsMessage(message)
	if err == nil {
		t.Error("Expected validation error validating a message with no task arn")
	}
}

// TestValidateRefreshMessageSuccess tests if a valid credentials message
// is validated without any errors
func TestValidateRefreshMessageSuccess(t *testing.T) {
	message := &ecsacs.IAMRoleCredentialsMessage{
		MessageId: aws.String(messageId),
		RoleCredentials: &ecsacs.IAMRoleCredentials{
			CredentialsId: aws.String("id"),
		},
		TaskArn: aws.String(taskArn),
	}
	err := validateIAMRoleCredentialsMessage(message)
	if err != nil {
		t.Errorf("Error validating credentials message: %v", err)
	}
}

// TestInvalidCredentialsMessageNotAcked tests if invalid credential messages
// are not acked
func TestInvalidCredentialsMessageNotAcked(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := credentials.NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	handler := newRefreshCredentialsHandler(ctx, cluster, containerInstance, nil, credentialsManager, nil)

	// Start a goroutine to listen for acks. Cancelling the context stops the goroutine
	go func() {
		for {
			select {
			// We never expect the message to be acked
			case <-handler.ackRequest:
				t.Fatalf("Received ack when none expected")
			case <-ctx.Done():
				return
			}
		}
	}()

	// test adding a credentials message without the MessageId field
	message := &ecsacs.IAMRoleCredentialsMessage{}
	err := handler.handleSingleMessage(message)
	if err == nil {
		t.Error("Expected error updating credentials when the message contains no message id")
	}
	cancel()
}

// TestCredentialsMessageNotAckedWhenTaskNotFound tests if credential messages
// are not acked when the task arn in the message is not found in the task
// engine
func TestCredentialsMessageNotAckedWhenTaskNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := credentials.NewManager()

	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	// Return task not found from the engine for GetTaskByArn
	taskEngine.EXPECT().GetTaskByArn(taskArn).Return(nil, false)

	ctx, cancel := context.WithCancel(context.Background())
	handler := newRefreshCredentialsHandler(ctx, cluster, containerInstance, nil, credentialsManager, taskEngine)

	// Start a goroutine to listen for acks. Cancelling the context stops the goroutine
	go func() {
		for {
			select {
			// We never expect the message to be acked
			case <-handler.ackRequest:
				t.Fatalf("Received ack when none expected")
			case <-ctx.Done():
				return
			}
		}
	}()

	// Test adding a credentials message without the MessageId field
	err := handler.handleSingleMessage(message)
	if err == nil {
		t.Error("Expected error updating credentials when the message contains unexpected task arn")
	}
	cancel()
}

// TestHandleRefreshMessageAckedWhenCredentialsUpdated tests that a credential message
// is ackd when the credentials are updated successfully and the domainless gMSA plugin credentials are updated successfully
func TestHandleRefreshMessageAckedWhenCredentialsUpdated(t *testing.T) {
	testCases := []struct {
		name                            string
		taskArn                         string
		domainlessGMSATaskExpectedInput bool
		containers                      []*apicontainer.Container
	}{
		{
			name:       "EmptyTaskSucceeds",
			taskArn:    taskArn,
			containers: []*apicontainer.Container{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			credentialsManager := credentials.NewManager()

			ctx, cancel := context.WithCancel(context.Background())
			var ackRequested *ecsacs.IAMRoleCredentialsAckRequest

			mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
			mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
				ackRequested = ackRequest
				cancel()
			}).Times(1)

			taskEngine := mock_engine.NewMockTaskEngine(ctrl)
			// Return a task from the engine for GetTaskByArn
			taskEngine.EXPECT().GetTaskByArn(tc.taskArn).Return(&apitask.Task{Arn: tc.taskArn, Containers: tc.containers}, true)

			checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = func(iamRoleCredentials credentials.IAMRoleCredentials, task *apitask.Task) error {
				if tc.taskArn != task.Arn {
					return errors.New(fmt.Sprintf("Expected taskArnInput to be %s, instead got %s", tc.taskArn, task.Arn))
				}

				return nil
			}

			defer func() {
				checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = checkAndSetDomainlessGMSATaskExecutionRoleCredentials
			}()

			handler := newRefreshCredentialsHandler(ctx, testconst.ClusterName, testconst.ContainerInstanceARN, mockWsClient, credentialsManager, taskEngine)
			go handler.sendAcks()

			// test adding a credentials message without the MessageId field
			err := handler.handleSingleMessage(message)
			if err != nil {
				t.Errorf("Error updating credentials: %v", err)
			}

			// Wait till we get an ack from the ackBuffer
			select {
			case <-ctx.Done():
			}

			if !reflect.DeepEqual(ackRequested, expectedAck) {
				t.Errorf("Message between expected and requested ack. Expected: %v, Requested: %v", expectedAck, ackRequested)
			}

			creds, exist := credentialsManager.GetTaskCredentials(credentialsId)
			if !exist {
				t.Errorf("Expected credentials to exist for the task")
			}
			if !reflect.DeepEqual(creds, expectedCredentials) {
				t.Errorf("Mismatch between expected credentials and credentials for task. Expected: %v, got: %v", expectedCredentials, creds)
			}
		})
	}
}

// TestCredentialsMessageNotAckedWhenDomainlessGMSACredentialsNotSet tests if credential messages
// are not acked when setting the domainlessGMSA Credentials fails
func TestCredentialsMessageNotAckedWhenDomainlessGMSACredentialsError(t *testing.T) {
	testCases := []struct {
		name                                                   string
		taskArn                                                string
		containers                                             []*apicontainer.Container
		domainlessGMSATaskExpectedInput                        bool
		setDomainlessGMSATaskExecutionRoleCredentialsImplError error
		expectedErrorString                                    string
	}{
		{
			name:                            "ErrDomainlessTask",
			taskArn:                         taskArn,
			containers:                      []*apicontainer.Container{{CredentialSpecs: []string{"credentialspecdomainless:file://gmsa_gmsa-acct.json"}}},
			domainlessGMSATaskExpectedInput: true,
			setDomainlessGMSATaskExecutionRoleCredentialsImplError: errors.New("mock setDomainlessGMSATaskExecutionRoleCredentialsImplError"),
			expectedErrorString: "unable to SetDomainlessGMSATaskExecutionRoleCredentials: mock setDomainlessGMSATaskExecutionRoleCredentialsImplError",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			credentialsManager := credentials.NewManager()

			taskEngine := mock_engine.NewMockTaskEngine(ctrl)
			// Return a task from the engine for GetTaskByArn
			taskEngine.EXPECT().GetTaskByArn(tc.taskArn).Return(&apitask.Task{Arn: tc.taskArn, Containers: tc.containers}, true)

			checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = func(iamRoleCredentials credentials.IAMRoleCredentials, task *apitask.Task) error {
				if tc.taskArn != task.Arn {
					return errors.New(fmt.Sprintf("Expected taskArnInput to be %s, instead got %s", tc.taskArn, task.Arn))
				}

				return tc.setDomainlessGMSATaskExecutionRoleCredentialsImplError
			}

			defer func() {
				checkAndSetDomainlessGMSATaskExecutionRoleCredentialsImpl = checkAndSetDomainlessGMSATaskExecutionRoleCredentials
			}()

			ctx, cancel := context.WithCancel(context.Background())
			handler := newRefreshCredentialsHandler(ctx, cluster, containerInstance, nil, credentialsManager, taskEngine)

			// Start a goroutine to listen for acks. Cancelling the context stops the goroutine
			go func() {
				for {
					select {
					// We never expect the message to be acked
					case <-handler.ackRequest:
						t.Fatalf("Received ack when none expected")
					case <-ctx.Done():
						return
					}
				}
			}()

			err := handler.handleSingleMessage(message)
			assert.EqualError(t, err, tc.expectedErrorString)
			cancel()
		})
	}
}

func TestRefreshCredentialsHandlerSendPendingAcks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	credentialsManager := credentials.NewManager()
	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Return(nil).Times(1)

	handler := newRefreshCredentialsHandler(ctx, testconst.ClusterName, testconst.ContainerInstanceARN, mockWSClient,
		credentialsManager, taskEngine)

	wg := sync.WaitGroup{}
	wg.Add(2)

	// write a dummy ack into the ackRequest
	go func() {
		handler.ackRequest <- expectedAck
		wg.Done()
	}()

	// sleep here to ensure that the sending go routine above executes before the receiving one below. if not, then the
	// receiving go routine will finish without receiving the ack msg since sendPendingAcks() is non-blocking.
	time.Sleep(1 * time.Second)

	go func() {
		handler.sendPendingAcks()
		wg.Done()
	}()

	// wait for both go routines above to finish before we verify that ack channel is empty and exit the test.
	// this also ensures that the mock MakeRequest call happened as expected.
	wg.Wait()

	// verify that the ackRequest channel is empty
	assert.Equal(t, 0, len(handler.ackRequest))
}

// TestRefreshCredentialsHandler tests if a credential message is acked when
// the message is sent to the messageBuffer channel
func TestRefreshCredentialsHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	credentialsManager := credentials.NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	var ackRequested *ecsacs.IAMRoleCredentialsAckRequest
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.IAMRoleCredentialsAckRequest) {
		ackRequested = ackRequest
		cancel()
	}).Times(1)

	taskEngine := mock_engine.NewMockTaskEngine(ctrl)
	// Return a task from the engine for GetTaskByArn
	taskEngine.EXPECT().GetTaskByArn(taskArn).Return(&apitask.Task{}, true)

	handler := newRefreshCredentialsHandler(ctx, testconst.ClusterName, testconst.ContainerInstanceARN, mockWsClient, credentialsManager, taskEngine)
	go handler.start()

	handler.messageBuffer <- message
	// Wait till we get an ack
	select {
	case <-ctx.Done():
	}

	if !reflect.DeepEqual(ackRequested, expectedAck) {
		t.Errorf("Message between expected and requested ack. Expected: %v, Requested: %v", expectedAck, ackRequested)
	}

	creds, exist := credentialsManager.GetTaskCredentials(credentialsId)
	if !exist {
		t.Errorf("Expected credentials to exist for the task")
	}
	if !reflect.DeepEqual(creds, expectedCredentials) {
		t.Errorf("Mismatch between expected credentials and credentials for task. Expected: %v, got: %v", expectedCredentials, creds)
	}
}
