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
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	acssession "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	apieni "github.com/aws/amazon-ecs-agent/ecs-agent/api/eni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"context"
)

// attachInstanceENIHandler handles instance ENI attach operation for the ACS client
type attachInstanceENIHandler struct {
	messageBuffer     chan *ecsacs.AttachInstanceNetworkInterfacesMessage
	ctx               context.Context
	cancel            context.CancelFunc
	cluster           *string
	containerInstance *string
	acsClient         wsclient.ClientServer
	eniHandler        acssession.ENIHandler
}

// newAttachInstanceENIHandler returns an instance of the attachInstanceENIHandler struct
func newAttachInstanceENIHandler(ctx context.Context,
	cluster string,
	containerInstanceArn string,
	acsClient wsclient.ClientServer,
	eniHandler acssession.ENIHandler) attachInstanceENIHandler {
	// Create a cancelable context from the parent context
	derivedContext, cancel := context.WithCancel(ctx)
	return attachInstanceENIHandler{
		messageBuffer:     make(chan *ecsacs.AttachInstanceNetworkInterfacesMessage),
		ctx:               derivedContext,
		cancel:            cancel,
		cluster:           aws.String(cluster),
		containerInstance: aws.String(containerInstanceArn),
		acsClient:         acsClient,
		eniHandler:        eniHandler,
	}
}

// handlerFunc returns a function to enqueue requests onto attachENIHandler buffer
func (handler *attachInstanceENIHandler) handlerFunc() func(message *ecsacs.AttachInstanceNetworkInterfacesMessage) {
	return func(message *ecsacs.AttachInstanceNetworkInterfacesMessage) {
		handler.messageBuffer <- message
	}
}

// start invokes handleMessages to ack each enqueued request
func (handler *attachInstanceENIHandler) start() {
	go handler.handleMessages()
}

// stop is used to invoke a cancellation function
func (handler *attachInstanceENIHandler) stop() {
	handler.cancel()
}

// handleMessages handles each message one at a time
func (handler *attachInstanceENIHandler) handleMessages() {
	for {
		select {
		case <-handler.ctx.Done():
			return
		case message := <-handler.messageBuffer:
			if err := handler.handleSingleMessage(message); err != nil {
				seelog.Warnf("Unable to handle instance ENI Attachment message [%s]: %v", message.String(), err)
			}
		}
	}
}

// handleSingleMessage acks the message received
func (handler *attachInstanceENIHandler) handleSingleMessage(message *ecsacs.AttachInstanceNetworkInterfacesMessage) error {
	receivedAt := time.Now()
	// Validate fields in the message
	if err := validateAttachInstanceNetworkInterfacesMessage(message); err != nil {
		return errors.Wrapf(err,
			"attach instance eni message handler: error validating AttachInstanceNetworkInterfacesMessage")
	}

	// Send ACK
	go sendAck(handler.acsClient, message.ClusterArn, message.ContainerInstanceArn, message.MessageId)

	expiresAt := receivedAt.Add(time.Duration(aws.Int64Value(message.WaitTimeoutMs)) * time.Millisecond)
	eniAttachment := &apieni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              "",
			AttachmentARN:        aws.StringValue(message.ElasticNetworkInterfaces[0].AttachmentArn),
			Status:               status.AttachmentNone,
			ExpiresAt:            expiresAt,
			AttachStatusSent:     false,
			ClusterARN:           aws.StringValue(message.ClusterArn),
			ContainerInstanceARN: aws.StringValue(message.ContainerInstanceArn),
		},
		AttachmentType: apieni.ENIAttachmentTypeInstanceENI,
		MACAddress:     aws.StringValue(message.ElasticNetworkInterfaces[0].MacAddress),
	}

	// Handle the attachment
	return handler.eniHandler.HandleENIAttachment(eniAttachment)
}

// validateAttachInstanceNetworkInterfacesMessage performs validation checks on the
// AttachInstanceNetworkInterfacesMessage
func validateAttachInstanceNetworkInterfacesMessage(message *ecsacs.AttachInstanceNetworkInterfacesMessage) error {
	if message == nil {
		return errors.Errorf("message is empty")
	}

	messageId := aws.StringValue(message.MessageId)
	if messageId == "" {
		return errors.Errorf("message id not set")
	}

	clusterArn := aws.StringValue(message.ClusterArn)
	if clusterArn == "" {
		return errors.Errorf("clusterArn not set")
	}

	containerInstanceArn := aws.StringValue(message.ContainerInstanceArn)
	if containerInstanceArn == "" {
		return errors.Errorf("containerInstanceArn not set")
	}

	enis := message.ElasticNetworkInterfaces
	if len(enis) != 1 {
		return errors.Errorf("incorrect number of instance ENIs in message: %d", len(enis))
	}

	eni := enis[0]
	if aws.StringValue(eni.MacAddress) == "" {
		return errors.Errorf("MACAddress not provided")
	}

	timeout := aws.Int64Value(message.WaitTimeoutMs)
	if timeout <= 0 {
		return errors.Errorf("invalid timeout specified: %d", timeout)

	}
	return nil
}
