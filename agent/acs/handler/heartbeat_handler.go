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

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/agent/doctor"
	"github.com/aws/amazon-ecs-agent/agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
)

// heartbeatHandler handles heartbeat messages from ACS
type heartbeatHandler struct {
	heartbeatMessageBuffer    chan *ecsacs.HeartbeatMessage
	heartbeatAckMessageBuffer chan *ecsacs.HeartbeatAckRequest
	ctx                       context.Context
	cancel                    context.CancelFunc
	acsClient                 wsclient.ClientServer
	doctor                    *doctor.Doctor
}

// newHeartbeatHandler returns an instance of the heartbeatHandler struct
func newHeartbeatHandler(ctx context.Context, acsClient wsclient.ClientServer, heartbeatDoctor *doctor.Doctor) heartbeatHandler {
	// Create a cancelable context from the parent context
	derivedContext, cancel := context.WithCancel(ctx)
	return heartbeatHandler{
		heartbeatMessageBuffer:    make(chan *ecsacs.HeartbeatMessage),
		heartbeatAckMessageBuffer: make(chan *ecsacs.HeartbeatAckRequest),
		ctx:                       derivedContext,
		cancel:                    cancel,
		acsClient:                 acsClient,
		doctor:                    heartbeatDoctor,
	}
}

// handlerFunc returns a function to enqueue requests onto the buffer
func (heartbeatHandler *heartbeatHandler) handlerFunc() func(message *ecsacs.HeartbeatMessage) {
	return func(message *ecsacs.HeartbeatMessage) {
		heartbeatHandler.heartbeatMessageBuffer <- message
	}
}

// start() invokes go routines to handle receive and respond to heartbeats
func (heartbeatHandler *heartbeatHandler) start() {
	go heartbeatHandler.handleHeartbeatMessage()
	go heartbeatHandler.sendHeartbeatAck()
}

func (heartbeatHandler *heartbeatHandler) handleHeartbeatMessage() {
	for {
		select {
		case message := <-heartbeatHandler.heartbeatMessageBuffer:
			if err := heartbeatHandler.handleSingleHeartbeatMessage(message); err != nil {
				seelog.Warnf("Unable to handle heartbeat message [%s]: %s", message.String(), err)
			}
		case <-heartbeatHandler.ctx.Done():
			return
		}
	}
}

func (heartbeatHandler *heartbeatHandler) handleSingleHeartbeatMessage(message *ecsacs.HeartbeatMessage) error {
	// TestHandlerDoesntLeakGoroutines unit test is failing because of this section

	// Agent will run healthchecks triggered by ACS heartbeat
	// healthcheck results will be sent on to TACS, but for now just to debug logs.
	go func() {
		heartbeatHandler.doctor.RunHealthchecks()
	}()

	// Agent will send simple ack to the heartbeatAckMessageBuffer
	go func() {
		response := &ecsacs.HeartbeatAckRequest{
			MessageId: message.MessageId,
		}
		heartbeatHandler.heartbeatAckMessageBuffer <- response
	}()
	return nil
}

func (heartbeatHandler *heartbeatHandler) sendHeartbeatAck() {
	for {
		select {
		case ack := <-heartbeatHandler.heartbeatAckMessageBuffer:
			heartbeatHandler.sendSingleHeartbeatAck(ack)
		case <-heartbeatHandler.ctx.Done():
			return
		}
	}
}

// sendPendingHeartbeatAck sends all pending heartbeat acks to ACS before closing the connection
func (heartbeatHandler *heartbeatHandler) sendPendingHeartbeatAck() {
	for {
		select {
		case ack := <-heartbeatHandler.heartbeatAckMessageBuffer:
			heartbeatHandler.sendSingleHeartbeatAck(ack)
		default:
			return
		}
	}
}

func (heartbeatHandler *heartbeatHandler) sendSingleHeartbeatAck(ack *ecsacs.HeartbeatAckRequest) {
	err := heartbeatHandler.acsClient.MakeRequest(ack)
	if err != nil {
		seelog.Warnf("Error acknowledging server heartbeat, message id: %s, error: %s", aws.StringValue(ack.MessageId), err)
	}
}

// stop() cancels the context being used by this handler, which stops the go routines started by 'start()'
func (heartbeatHandler *heartbeatHandler) stop() {
	heartbeatHandler.cancel()
}

// clearAcks drains the ack request channel
func (heartbeatHandler *heartbeatHandler) clearAcks() {
	for {
		select {
		case <-heartbeatHandler.heartbeatAckMessageBuffer:
		default:
			return
		}
	}
}
