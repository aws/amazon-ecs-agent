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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"golang.org/x/net/context"
)

// attachENIHandler represents the ENI attach operation for the ACS client
type attachENIHandler struct {
	messageBuffer     chan *ecsacs.AttachTaskNetworkInterfacesMessage
	ctx               context.Context
	cancel            context.CancelFunc
	saver             statemanager.Saver
	cluster           *string
	containerInstance *string
	acsClient         wsclient.ClientServer
	state             dockerstate.TaskEngineState
}

// newAttachENIHandler returns an instance of the attachENIHandler struct
func newAttachENIHandler(ctx context.Context,
	cluster string,
	containerInstanceArn string,
	acsClient wsclient.ClientServer,
	taskEngineState dockerstate.TaskEngineState,
	saver statemanager.Saver) attachENIHandler {

	// Create a cancelable context from the parent context
	derivedContext, cancel := context.WithCancel(ctx)
	return attachENIHandler{
		messageBuffer:     make(chan *ecsacs.AttachTaskNetworkInterfacesMessage),
		ctx:               derivedContext,
		cancel:            cancel,
		cluster:           aws.String(cluster),
		containerInstance: aws.String(containerInstanceArn),
		acsClient:         acsClient,
		state:             taskEngineState,
		saver:             saver,
	}
}

// handlerFunc returns a function to enqueue requests onto attachENIHandler buffer
func (attachENIHandler *attachENIHandler) handlerFunc() func(message *ecsacs.AttachTaskNetworkInterfacesMessage) {
	return func(message *ecsacs.AttachTaskNetworkInterfacesMessage) {
		attachENIHandler.messageBuffer <- message
	}
}

// start invokes handleMessages to ack each enqueued request
func (attachENIHandler *attachENIHandler) start() {
	go attachENIHandler.handleMessages()
}

// stop is used to invoke a cancellation function
func (attachENIHandler *attachENIHandler) stop() {
	attachENIHandler.cancel()
}

// handleMessages handles each message one at a time
func (attachENIHandler *attachENIHandler) handleMessages() {
	for {
		select {
		case message := <-attachENIHandler.messageBuffer:
			attachENIHandler.handleSingleMessage(message)
		case <-attachENIHandler.ctx.Done():
			return
		}
	}
}

// handleSingleMessage acks the message received
// TODO: Send response to validate ENI attachment
func (attachENIHandler *attachENIHandler) handleSingleMessage(message *ecsacs.AttachTaskNetworkInterfacesMessage) error {
	// Validate fields in the message
	err := validateAttachTaskNetworkInterfacesMessage(message)
	if err != nil {
		return errors.Wrapf(err, "attach eni message handler: error validating AttachTaskNetworkInterfac message received from ECS")
	}

	err = attachENIHandler.acsClient.MakeRequest(&ecsacs.AckRequest{
		Cluster:           message.ClusterArn,
		ContainerInstance: message.ContainerInstanceArn,
		MessageId:         message.MessageId,
	})
	if err != nil {
		seelog.Warnf("Failed to ack request with messageId: %s, error: %v", aws.StringValue(message.MessageId), err)
	}

	// Check if this is a duplicate message
	if _, ok := attachENIHandler.state.ENIByMac(aws.StringValue(message.ElasticNetworkInterfaces[0].MacAddress)); ok {
		seelog.Info("Duplicate ENIAttachment message, ENIAttachment is already managed by agent, mac: %s", aws.StringValue(message.ElasticNetworkInterfaces[0].MacAddress))
		return nil
	}

	attachENIHandler.addENIAttachmentToState(message)
	err = attachENIHandler.saver.Save()
	if err != nil {
		return errors.Wrapf(err, "attach eni message handler: error save agent state")
	}
	return nil
}

// addENIAttachmentToState adds the eni info to the state
func (handler *attachENIHandler) addENIAttachmentToState(message *ecsacs.AttachTaskNetworkInterfacesMessage) {
	attachmentArn := aws.StringValue(message.ElasticNetworkInterfaces[0].AttachmentArn)
	seelog.Info("Adding eni info to state, eni: %s", attachmentArn)
	eniAttachment := &api.ENIAttachment{
		TaskArn:          aws.StringValue(message.TaskArn),
		AttachmentArn:    attachmentArn,
		AttachStatusSent: false,
		MacAddress:       aws.StringValue(message.ElasticNetworkInterfaces[0].MacAddress),
	}
	handler.state.AddENIAttachment(eniAttachment)

	// Stop tracking the eni attachment after timeout
	eniAckTimeout := time.Duration(aws.Int64Value(message.WaitTimeoutMs)) * time.Millisecond
	timeoutHandler := func() {
		if !eniAttachment.GetSentStatus() {
			handler.state.RemoveENIAttachment(aws.StringValue(message.ElasticNetworkInterfaces[0].MacAddress))
		}
	}
	go func() {
		time.AfterFunc(eniAckTimeout, timeoutHandler)
	}()
}

// validateAttachTaskNetworkInterfacesMessage performs validation checks on the
// AttachTaskNetworkInterfacesMessage
func validateAttachTaskNetworkInterfacesMessage(message *ecsacs.AttachTaskNetworkInterfacesMessage) error {
	if message == nil {
		return errors.Errorf("attach eni handler validation: empty AttachTaskNetworkInterface message received from ECS")
	}

	messageId := aws.StringValue(message.MessageId)
	if messageId == "" {
		return errors.Errorf("attach eni handler validation: message id not set in AttachTaskNetworkInterface message received from ECS")
	}

	clusterArn := aws.StringValue(message.ClusterArn)
	if clusterArn == "" {
		return errors.Errorf("attach eni handler validation: clusterArn not set in AttachTaskNetworkInterface message received from ECS")
	}

	containerInstanceArn := aws.StringValue(message.ContainerInstanceArn)
	if containerInstanceArn == "" {
		return errors.Errorf("attach eni handler validation: containerInstanceArn not set in AttachTaskNetworkInterface message received from ECS")
	}

	enis := message.ElasticNetworkInterfaces
	if len(enis) != 1 {
		return errors.Errorf("attach eni handler validation: incorrect number of ENIs in AttachTaskNetworkInterface message received from ECS. Obtained %d", len(enis))
	}

	eni := enis[0]
	if aws.StringValue(eni.MacAddress) == "" {
		return errors.Errorf("attach eni handler validation: MACAddress not listed in AttachTaskNetworkInterface message received from ECS")
	}

	taskArn := aws.StringValue(message.TaskArn)
	if taskArn == "" {
		return errors.Errorf("attach eni handler validation: taskArn not set in AttachTaskNetworkInterface message received from ECS")
	}

	timeout := aws.Int64Value(message.WaitTimeoutMs)
	if timeout <= 0 {
		return errors.Errorf("attach eni handler validation: invalid timeout listed in AttachTaskNetworkInterface message received from ECS")

	}

	return nil
}
