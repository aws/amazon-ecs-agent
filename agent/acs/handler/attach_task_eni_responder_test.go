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
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	mock_dockerstate "github.com/aws/amazon-ecs-agent/agent/engine/dockerstate/mocks"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	acssession "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apieni "github.com/aws/amazon-ecs-agent/ecs-agent/api/eni"
)

var testAttachTaskENIMessage = &ecsacs.AttachTaskNetworkInterfacesMessage{
	MessageId:            aws.String(testconst.MessageID),
	ClusterArn:           aws.String(testconst.ClusterName),
	ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
	ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
		{
			Ec2Id:                        aws.String("1"),
			MacAddress:                   aws.String(testconst.RandomMAC),
			InterfaceAssociationProtocol: aws.String(testconst.InterfaceProtocol),
			SubnetGatewayIpv4Address:     aws.String(testconst.GatewayIPv4),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(testconst.IPv4Address),
				},
			},
		},
	},
	TaskArn:       aws.String(testconst.TaskARN),
	WaitTimeoutMs: aws.Int64(testconst.WaitTimeoutMillis),
}

// TestTaskENIAckHappyPath tests the happy path for a typical AttachTaskNetworkInterfacesMessage and confirms expected
// ACK request is made
func TestTaskENIAckHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ackSent := make(chan *ecsacs.AckRequest)

	taskEngineState := dockerstate.NewTaskEngineState()
	dataClient := data.NewNoopClient()

	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}
	testAttachTaskENIResponder := acssession.NewAttachTaskENIResponder(
		&eniHandler{
			state:      taskEngineState,
			dataClient: dataClient,
		},
		testResponseSender)

	handleAttachMessage := testAttachTaskENIResponder.HandlerFunc().(func(*ecsacs.AttachTaskNetworkInterfacesMessage))
	go handleAttachMessage(testAttachTaskENIMessage)

	attachTaskEniAckSent := <-ackSent
	assert.Equal(t, aws.StringValue(attachTaskEniAckSent.MessageId), testconst.MessageID)
}

// TestTaskENIAckSingleMessageWithDuplicateENIAttachment tests the path for an
// AttachTaskNetworkInterfacesMessage with a duplicate expired ENI and confirms:
//  1. attempt is made to start the ack timer that records the expiration of ENI attachment (i.e., ENI is not added to
//     task engine state)
//  2. expected ACK request is made
func TestTaskENIAckSingleMessageWithDuplicateENIAttachment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ackSent := make(chan *ecsacs.AckRequest)

	mockState := mock_dockerstate.NewMockTaskEngineState(ctrl)
	dataClient := data.NewNoopClient()

	// Set expiresAt to a value in the past.
	expiresAt := time.Unix(time.Now().Unix()-1, 0)

	// WaitGroup is necessary to wait for a function to be called in separate goroutine before exiting the test.
	wg := sync.WaitGroup{}
	wg.Add(1)

	gomock.InOrder(
		// Sending a duplicate expired ENI attachment still results in an attempt to start the timer. We don't really
		// care if the timer is actually started or not (i.e., whether or not the ENI attachment is expired); we just
		// care that an attempt was made. Attempting to start the timer means that the ENI attachment was not added to
		// the task engine state.
		mockState.EXPECT().
			ENIByMac(testconst.RandomMAC).
			Return(&apieni.ENIAttachment{
				AttachmentInfo: attachmentinfo.AttachmentInfo{
					ExpiresAt: expiresAt,
				},
			}, true).
			Do(func(arg0 interface{}) {
				defer wg.Done() // we can exit the test now that ENIByMac function has been called
			}),
	)

	testResponseSender := func(response interface{}) error {
		resp := response.(*ecsacs.AckRequest)
		ackSent <- resp
		return nil
	}
	testAttachTaskENIResponder := acssession.NewAttachTaskENIResponder(
		&eniHandler{
			state:      mockState,
			dataClient: dataClient,
		},
		testResponseSender)

	handleAttachMessage := testAttachTaskENIResponder.HandlerFunc().(func(*ecsacs.AttachTaskNetworkInterfacesMessage))
	go handleAttachMessage(testAttachTaskENIMessage)

	attachTaskEniAckSent := <-ackSent
	wg.Wait()
	assert.Equal(t, aws.StringValue(attachTaskEniAckSent.MessageId), testconst.MessageID)
}
