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

	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apieni "github.com/aws/amazon-ecs-agent/ecs-agent/api/eni"
	mock_wsclient "github.com/aws/amazon-ecs-agent/ecs-agent/wsclient/mock"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestInvalidAttachInstanceENIMessage tests various invalid formats of AttachInstanceNetworkInterfacesMessage
func TestInvalidAttachInstanceENIMessage(t *testing.T) {
	tcs := []struct {
		message     *ecsacs.AttachInstanceNetworkInterfacesMessage
		description string
	}{
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				ClusterArn:               aws.String(clusterName),
				ContainerInstanceArn:     aws.String(containerInstanceArn),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
				WaitTimeoutMs:            aws.Int64(waitTimeoutMillis),
			},
			description: "Message without message id should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:                aws.String(eniMessageId),
				ContainerInstanceArn:     aws.String(containerInstanceArn),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
				WaitTimeoutMs:            aws.Int64(waitTimeoutMillis),
			},
			description: "Message without cluster arn should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:                aws.String(eniMessageId),
				ClusterArn:               aws.String(clusterName),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{},
				WaitTimeoutMs:            aws.Int64(waitTimeoutMillis),
			},
			description: "Message without container instance arn should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:     aws.String(eniMessageId),
				ClusterArn:    aws.String(clusterName),
				WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
			},
			description: "Message without network interfaces should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:            aws.String(eniMessageId),
				ClusterArn:           aws.String(clusterName),
				ContainerInstanceArn: aws.String(containerInstanceArn),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
					{
						MacAddress: aws.String(randomMAC),
						Ec2Id:      aws.String("1"),
					},
					{
						MacAddress: aws.String(randomMAC),
						Ec2Id:      aws.String("2"),
					},
				},
				WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
			},
			description: "Message with multiple network interfaces should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:            aws.String(eniMessageId),
				ClusterArn:           aws.String(clusterName),
				ContainerInstanceArn: aws.String(containerInstanceArn),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
					{},
				},
				WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
			},
			description: "Message without network details should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:            aws.String(eniMessageId),
				ClusterArn:           aws.String(clusterName),
				ContainerInstanceArn: aws.String(containerInstanceArn),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
					{
						Ec2Id: aws.String("1"),
					},
				},
				WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
			},
			description: "Message with a network interface without macAddress should be invalid",
		},
		{
			message: &ecsacs.AttachInstanceNetworkInterfacesMessage{
				MessageId:            aws.String(eniMessageId),
				ClusterArn:           aws.String(clusterName),
				ContainerInstanceArn: aws.String(containerInstanceArn),
				ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
					{
						MacAddress: aws.String(randomMAC),
						Ec2Id:      aws.String("1"),
					},
				},
			},
			description: "Message without wait timeout should be invalid",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			assert.Error(t, validateAttachInstanceNetworkInterfacesMessage(tc.message))
		})
	}
}

// TestInstanceENIAckSingleMessage checks the ack for a single message
func TestInstanceENIAckSingleMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	handler := newAttachInstanceENIHandler(ctx, clusterName, containerInstanceArn, mockWSClient,
		&eniHandler{
			state:      taskEngineState,
			dataClient: dataClient,
		},
	)

	var ackSent sync.WaitGroup
	ackSent.Add(1)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		assert.Equal(t, aws.StringValue(ackRequest.MessageId), eniMessageId)
		ackSent.Done()
	})

	go handler.start()

	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:         aws.String("1"),
		MacAddress:    aws.String(randomMAC),
		AttachmentArn: aws.String(attachmentArn),
	}
	message := &ecsacs.AttachInstanceNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
	}

	handler.messageBuffer <- message

	ackSent.Wait()
	handler.stop()
}

// TestInstanceENIAckSingleMessageDuplicateENIAttachmentMessageStartsTimer checks the ack for a single message
// and ensures that the ENI ack expiration timer is started
func TestInstanceENIAckSingleMessageDuplicateENIAttachmentMessageStartsTimer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	dataClient := data.NewNoopClient()

	ctx := context.TODO()
	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	handler := newAttachInstanceENIHandler(ctx, clusterName, containerInstanceArn, mockWSClient,
		&eniHandler{
			state:      mockState,
			dataClient: dataClient,
		},
	)

	// To check that the timer is started, we set the expiresAt value of the attachment to be a value in the past
	// to trigger an error in attachment.StartTimer and checks the error
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
		// enough check to ensure that the timer was started (since StartTimer would be
		// the only place to return error)
		mockState.EXPECT().ENIByMac(randomMAC).Return(&apieni.ENIAttachment{
			AttachmentInfo: attachmentinfo.AttachmentInfo{
				ExpiresAt: expiresAt,
			},
		}, true),
	)

	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:         aws.String("1"),
		MacAddress:    aws.String(randomMAC),
		AttachmentArn: aws.String("attachmentarn"),
	}
	message := &ecsacs.AttachInstanceNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
	}

	// Expect an error starting the timer because of <=0 duration
	err := handler.handleSingleMessage(message)
	assert.Error(t, err)
	ackSent.Wait()
}

// TestInstanceENIAckHappyPath tests the happy path for a typical AttachInstanceNetworkInterfacesMessage
func TestInstanceENIAckHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()

	mockWSClient := mock_wsclient.NewMockClientServer(ctrl)
	handler := newAttachInstanceENIHandler(ctx, clusterName, containerInstanceArn, mockWSClient,
		&eniHandler{
			state:      taskEngineState,
			dataClient: dataClient,
		},
	)

	var ackSent sync.WaitGroup
	ackSent.Add(1)
	mockWSClient.EXPECT().MakeRequest(gomock.Any()).Do(func(ackRequest *ecsacs.AckRequest) {
		assert.Equal(t, aws.StringValue(ackRequest.MessageId), eniMessageId)
		ackSent.Done()
		handler.stop()
	})

	go handler.start()

	mockNetInterface1 := ecsacs.ElasticNetworkInterface{
		Ec2Id:      aws.String("1"),
		MacAddress: aws.String(randomMAC),
	}
	message := &ecsacs.AttachInstanceNetworkInterfacesMessage{
		MessageId:            aws.String(eniMessageId),
		ClusterArn:           aws.String(clusterName),
		ContainerInstanceArn: aws.String(containerInstanceArn),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
			&mockNetInterface1,
		},
		WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
	}

	handler.messageBuffer <- message

	ackSent.Wait()
	select {
	case <-handler.ctx.Done():
	}
}
