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
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	mock_wsclient "github.com/aws/amazon-ecs-agent/agent/wsclient/mock"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	eniMessageId      = "123"
	randomMAC         = "00:0a:95:9d:68:16"
	waitTimeoutMillis = 1000
)

// TestAttachENIMessageWithNoMessageId checks the validator against an
// AttachTaskNetworkInterfacesMessage without a messageId
func TestAttachENIMessageWithNoMessageId(t *testing.T) {
	message := &ecsacs.AttachTaskNetworkInterfacesMessage{
		ClusterArn:               aws.String(clusterName),
		ContainerInstanceArn:     aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
		TaskArn:                  aws.String(taskArn),
		WaitTimeoutMs:            aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs:            aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs:            aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
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
	dataClient := data.NewNoopClient()

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	eniAttachHandler := newAttachTaskENIHandler(ctx, clusterName, containerInstanceArn, mockWSClient, taskEngineState, dataClient)

	var ackSent sync.WaitGroup
	ackSent.Add(1)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		assert.Equal(t, aws.StringValue(ackRequest.MessageId), eniMessageId)
		ackSent.Done()
	})

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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
	}

	eniAttachHandler.messageBuffer <- message
	ackSent.Wait()
	eniAttachHandler.stop()
}

// TestENIAckSingleMessageDuplicateENIAttachmentMessageStartsTimer checks the ack for a single message
// and ensures that the ENI ack expiration timer is started
func TestENIAckSingleMessageDuplicateENIAttachmentMessageStartsTimer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	dataClient := data.NewNoopClient()

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	eniAttachHandler := newAttachTaskENIHandler(ctx, clusterName, containerInstanceArn, mockWSClient, mockState, dataClient)

	// Set expiresAt to a value in the past
	expiresAt := time.Unix(time.Now().Unix()-1, 0)
	var ackSent sync.WaitGroup
	ackSent.Add(1)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		assert.Equal(t, aws.StringValue(ackRequest.MessageId), eniMessageId)
		ackSent.Done()
	})
	gomock.InOrder(
		// Sending an attachment with ExpiresAt set in the past results in an
		// error in starting the timer.
		// Ensuring that statemanager.Save() is not invoked should be a strong
		// enough check to ensure that the timer was started
		mockState.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{ExpiresAt: expiresAt}, true),
	)

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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
	}

	// Expect an error starting the timer because of <=0 duration
	err := eniAttachHandler.handleSingleMessage(message)
	assert.Error(t, err)
	ackSent.Wait()
}

// TestENIAckHappyPath tests the happy path for a typical AttachTaskNetworkInterfacesMessage
func TestENIAckHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()

	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	eniAttachHandler := newAttachTaskENIHandler(ctx, clusterName, containerInstanceArn, mockWSClient, taskEngineState, dataClient)

	var ackSent sync.WaitGroup
	ackSent.Add(1)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		assert.Equal(t, aws.StringValue(ackRequest.MessageId), eniMessageId)
		ackSent.Done()
		eniAttachHandler.stop()
	})

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
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
	}

	eniAttachHandler.messageBuffer <- message

	ackSent.Wait()
	select {
	case <-eniAttachHandler.ctx.Done():
	}
}
