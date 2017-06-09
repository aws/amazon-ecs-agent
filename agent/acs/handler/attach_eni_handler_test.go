// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager/mocks"
	"github.com/aws/amazon-ecs-agent/agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	eniMessageId = "123"
	randomMAC    = "00:0a:95:9d:68:16"
	waitTimeout  = 10
)

// TestAttachENIMessageWithNoMessageId checks the validator against an
// AttachTaskNetworkInterfacesMessage without a messageId
func TestAttachENIMessageWithNoMessageId(t *testing.T) {
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		ClusterArn:               aws.String(clusterName),
		ContainerInstanceArn:     aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
		TaskArn:                  aws.String(taskArn),
		WaitTimeoutMs:            aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithNoClusterArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a ClusterArn
func TestAttachENIMessageWithNoClusterArn(t *testing.T) {
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:                aws.String(eniMessageId),
		ContainerInstanceArn:     aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
		TaskArn:                  aws.String(taskArn),
		WaitTimeoutMs:            aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithNoContainerInstanceArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a ContainerInstanceArn
func TestAttachENIMessageWithNoContainerInstanceArn(t *testing.T) {
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:                aws.String(eniMessageId),
		ClusterArn:               aws.String(clusterName),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
		TaskArn:                  aws.String(taskArn),
		WaitTimeoutMs:            aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithNoInterfaces checks the validator against an
// AttachTaskNetworkInterfacesMessage without any interface
func TestAttachENIMessageWithNoInterfaces(t *testing.T) {
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:     aws.String(eniMessageId),
		ClusterArn:    aws.String(clusterName),
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}
	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithMultipleInterfaceschecks checks the validator against an
// AttachTaskNetworkInterfacesMessage with multiple interfaces
func TestAttachENIMessageWithMultipleInterfaces(t *testing.T) {
	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		MacAddress: aws.String(randomMAC),
		Ec2Id:      aws.String("1"),
	}
	mockNetInterface2 := ecsacs.ElasticNetworkInterface{
		MacAddress: aws.String(randomMAC),
		Ec2Id:      aws.String("2"),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
			&mockNetInterface2,
		},
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithMissingNetworkDetails checks the validator against an
// AttachTaskNetworkInterfacesMessage without network details
func TestAttachENIMessageWithMissingNetworkDetails(t *testing.T) {
	mockNetInterface1 := ecsacs.ElasticNetworkInterface{}

	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithMissingMACAddress checks the validator against an
// AttachTaskNetworkInterfacesMessage without a MAC address
func TestAttachENIMessageWithMissingMACAddress(t *testing.T) {
	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id: aws.String("1"),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TODO:
// * Add TaskArn + Timeout Tests

// TestAttachENIMessageWithMissingTaskArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a MAC address
func TestAttachENIMessageWithMissingTaskArn(t *testing.T) {
	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:      aws.String("1"),
		MacAddress: aws.String(randomMAC),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestAttachENIMessageWithMissingTimeout checks the validator against an
// AttachTaskNetworkInterfacesMessage without a MAC address
func TestAttachENIMessageWithMissingTimeout(t *testing.T) {
	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id: aws.String("1"),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		TaskArn: aws.String(taskArn),
	}

	err := validateAttachTaskNetworkInterfacesMessage(message)
	assert.Error(t, err)
}

// TestENIAckSingleMessage checks the ack for a single message
func TestENIAckSingleMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	manager := mock_statemanager.NewMockStateManager(ctrl)

	ctx := context.TODO()
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	eniAttachHandler := newAttachENIHandler(ctx, clusterName, containerInstanceArn, mockWsClient, taskEngineState, manager)

	var eniAckRequested *ecsacs.AckRequest
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		eniAckRequested = ackRequest
		eniAttachHandler.stop()
	})
	manager.EXPECT().Save().Return(nil)

	go eniAttachHandler.start()

	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:         aws.String("1"),
		MacAddress:    aws.String(randomMAC),
		AttachmentArn: aws.String("attachmentarn"),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	eniAttachHandler.handleSingleMessage(message)
	assert.Len(t, taskEngineState.(*dockerstate.DockerTaskEngineState).AllENIAttachments(), 1)
	eniattachment, ok := taskEngineState.ENIByMac(randomMAC)
	assert.True(t, ok)
	assert.Equal(t, taskArn, eniattachment.TaskArn)

	select {
	case <-eniAttachHandler.ctx.Done():
	}

	assert.Equal(t, aws.StringValue(eniAckRequested.MessageId), eniMessageId)
}

// TestENIAckForMessageIdMismatch tests for mismatched messageId's in the
// AttachTaskNetworkInterfacesMessage and the ack
func TestENIAckForMessageIdMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	manager := mock_statemanager.NewMockStateManager(ctrl)

	ctx := context.TODO()
	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	eniAttachHandler := newAttachENIHandler(ctx, clusterName, containerInstanceArn, mockWsClient, taskEngineState, manager)

	var eniAckRequested *ecsacs.AckRequest
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		// Change MessageId
		ackRequest.MessageId = aws.String("42")
		eniAckRequested = ackRequest
		eniAttachHandler.stop()
	})
	manager.EXPECT().Save().Return(nil)

	go eniAttachHandler.start()

	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:      aws.String("1"),
		MacAddress: aws.String(randomMAC),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	eniAttachHandler.handleSingleMessage(message)

	select {
	case <-eniAttachHandler.ctx.Done():
	}

	assert.NotEqual(t, aws.StringValue(eniAckRequested.MessageId), eniMessageId)
}

// TestENIAckHappyPath tests the happy path for a typical AttachTaskNetworkInterfacesMessage
func TestENIAckHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	taskEngineState := dockerstate.NewTaskEngineState()
	manager := mock_statemanager.NewMockStateManager(ctrl)

	mockWsClient := mock_wsclient.NewMockClientServer(ctrl)
	eniAttachHandler := newAttachENIHandler(ctx, clusterName, containerInstanceArn, mockWsClient, taskEngineState, manager)

	var eniAckRequested *ecsacs.AckRequest
	mockWsClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		eniAckRequested = ackRequest
		eniAttachHandler.stop()
	})
	manager.EXPECT().Save().Return(nil).AnyTimes()

	go eniAttachHandler.start()

	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:      aws.String("1"),
		MacAddress: aws.String(randomMAC),
	}
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		TaskArn:       aws.String(taskArn),
		WaitTimeoutMs: aws.Int64(waitTimeout),
	}

	eniAttachHandler.messageBuffer <- message
	eniAttachHandler.handleMessages()

	select {
	case <-eniAttachHandler.ctx.Done():
	}

	assert.Equal(t, aws.StringValue(eniAckRequested.MessageId), eniMessageId)
}
