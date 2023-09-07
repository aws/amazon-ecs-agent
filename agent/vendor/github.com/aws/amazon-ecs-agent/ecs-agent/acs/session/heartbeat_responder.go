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
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"

	"github.com/aws/aws-sdk-go/aws"
)

// heartbeatResponder implements the wsclient.RequestResponder interface for responding
// to ecsacs.HeartbeatMessage type.
type heartbeatResponder struct {
	doctor  *doctor.Doctor
	respond wsclient.RespondFunc
}

// NewHeartbeatResponder returns an instance of the heartbeatResponder struct.
func NewHeartbeatResponder(doctor *doctor.Doctor, responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &heartbeatResponder{
		doctor: doctor,
	}
	r.respond = responseToACSSender(r.Name(), responseSender)
	return r
}

func (*heartbeatResponder) Name() string {
	return "heartbeat message responder"
}

func (r *heartbeatResponder) HandlerFunc() wsclient.RequestHandler {
	return r.processHeartbeatMessage
}

// processHeartbeatMessage processes an ACS heartbeat message.
// This function is meant to be called from the ACS dispatcher and as such
// should not block in any way to prevent starvation of the message handler.
func (r *heartbeatResponder) processHeartbeatMessage(message *ecsacs.HeartbeatMessage) {
	// Agent will run container instance healthchecks. They are triggered by ACS heartbeat.
	// Results of healthchecks will be sent on to TACS.
	go r.doctor.RunHealthchecks()

	// Agent will send simple ack
	ack := &ecsacs.HeartbeatAckRequest{
		MessageId: message.MessageId,
	}
	go func() {
		err := r.respond(ack)
		if err != nil {
			logger.Warn("Error acknowledging server heartbeat", logger.Fields{
				field.MessageID: aws.StringValue(ack.MessageId),
				field.Error:     err,
			})
		}
	}()
}
