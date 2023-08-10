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

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

const (
	PayloadMessageName = "PayloadMessage"
)

type PayloadMessageHandler interface {
	ProcessMessage(message *ecsacs.PayloadMessage,
		ackFunc func(*ecsacs.AckRequest, []*ecsacs.IAMRoleCredentialsAckRequest)) error
}

// payloadResponder implements the wsclient.RequestResponder interface for responding
// to ecsacs.PayloadMessage messages sent by ACS.
type payloadResponder struct {
	payloadMessageHandler PayloadMessageHandler
	respond               wsclient.RespondFunc
}

// NewPayloadResponder returns an instance of the payloadResponder struct.
func NewPayloadResponder(payloadMessageHandler PayloadMessageHandler,
	responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &payloadResponder{
		payloadMessageHandler: payloadMessageHandler,
	}
	r.respond = ResponseToACSSender(r.Name(), responseSender)
	return r
}

func (*payloadResponder) Name() string { return "payload responder" }

func (r *payloadResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handlePayloadMessage
}

func (r *payloadResponder) handlePayloadMessage(message *ecsacs.PayloadMessage) {
	logger.Debug(fmt.Sprintf("Handling %s", PayloadMessageName))

	// Validate fields in the message.
	if err := validatePayloadMessage(message); err != nil {
		logger.Error(fmt.Sprintf("Error validating %s received from ECS", PayloadMessageName), logger.Fields{
			field.Error: err,
		})
		return
	}

	// Handle payload message.
	err := r.payloadMessageHandler.ProcessMessage(message, r.ackFunc)
	if err != nil {
		logger.Critical(fmt.Sprintf("Unable to handle %s", PayloadMessageName), logger.Fields{
			field.MessageID: aws.StringValue(message.MessageId),
			field.Error:     err,
		})
	}
}

// ackFunc sends ACKs of the payload message and of the credentials associated with the tasks contained in the payload
// message.
func (r *payloadResponder) ackFunc(payloadAck *ecsacs.AckRequest, credsAcks []*ecsacs.IAMRoleCredentialsAckRequest) {
	go r.sendAck(payloadAck)

	for _, credsAck := range credsAcks {
		go r.sendAck(credsAck)
	}
}

// sendAck handles the sending of an individual specific ACK, assuming it is of type
// ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest.
//
// NOTE: These above two ACK types are different from each other. payloadResponder needs to be able to send both types
// of ACKs because while processing payload message we may wish to ACK:
//  1. any credentials associated with task(s) contained in a payload message that were handled
//     (via ecsacs.IAMRoleCredentialsAckRequest)
//  2. the payload message itself
//     (via ecsacs.AckRequest)
func (r *payloadResponder) sendAck(ackRequest interface{}) {
	var credentialsAck *ecsacs.IAMRoleCredentialsAckRequest
	var payloadMessageAck *ecsacs.AckRequest
	credentialsAck, ok := ackRequest.(*ecsacs.IAMRoleCredentialsAckRequest)
	if ok {
		logger.Debug(fmt.Sprintf("ACKing credentials associated with %s", PayloadMessageName), logger.Fields{
			field.CredentialsID: aws.StringValue(credentialsAck.CredentialsId),
			field.MessageID:     aws.StringValue(credentialsAck.MessageId),
		})
	} else {
		payloadMessageAck, ok = ackRequest.(*ecsacs.AckRequest)
		if ok {
			logger.Debug(fmt.Sprintf("ACKing %s", PayloadMessageName), logger.Fields{
				field.MessageID: aws.StringValue(payloadMessageAck.MessageId),
			})
		} else {
			logger.Error(fmt.Sprintf("Error sending acknowledgement: %s",
				"ackRequest does not hold type ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest"))
			return
		}
	}

	err := r.respond(ackRequest)

	if err != nil {
		if credentialsAck != nil {
			logger.Warn(fmt.Sprintf("Error acknowledging credentials associated with %s",
				PayloadMessageName), logger.Fields{
				field.CredentialsID: aws.StringValue(credentialsAck.CredentialsId),
				field.MessageID:     aws.StringValue(credentialsAck.MessageId),
				field.Error:         err,
			})
		} else if payloadMessageAck != nil {
			logger.Warn(fmt.Sprintf("Error acknowledging %s", PayloadMessageName), logger.Fields{
				field.MessageID: aws.StringValue(payloadMessageAck.MessageId),
				field.Error:     err,
			})
		} else {
			// We don't expect this condition to ever be reached, but log an error just in case it is.
			logger.Error(fmt.Sprintf("Error sending acknowledgement for %s",
				"ackRequest that does not hold type ecsacs.IAMRoleCredentialsAckRequest or ecsacs.AckRequest"),
				logger.Fields{
					field.Error: err,
				})
		}
	}
}

// validatePayloadMessage performs validation checks on the PayloadMessage.
func validatePayloadMessage(message *ecsacs.PayloadMessage) error {
	if message == nil {
		return errors.Errorf("Message is empty")
	}

	messageID := aws.StringValue(message.MessageId)
	if messageID == "" {
		return errors.Errorf("Message ID is not set")
	}

	clusterArn := aws.StringValue(message.ClusterArn)
	if clusterArn == "" {
		return errors.Errorf("clusterArn is not set for message ID %s", messageID)
	}

	containerInstanceArn := aws.StringValue(message.ContainerInstanceArn)
	if containerInstanceArn == "" {
		return errors.Errorf("containerInstanceArn is not set for message ID %s", messageID)
	}

	return nil
}
