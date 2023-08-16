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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"
)

const (
	AttachInstanceENIMessageName = "AttachInstanceNetworkInterfacesMessage"
)

// attachInstanceENIResponder implements the wsclient.RequestResponder interface for responding
// to ecsacs.AttachInstanceNetworkInterfacesMessage messages sent by ACS.
type attachInstanceENIResponder struct {
	eniHandler ENIHandler
	respond    wsclient.RespondFunc
}

// NewAttachInstanceENIResponder returns an instance of the attachInstanceENIResponder struct.
func NewAttachInstanceENIResponder(eniHandler ENIHandler,
	responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &attachInstanceENIResponder{
		eniHandler: eniHandler,
	}
	r.respond = ResponseToACSSender(r.Name(), responseSender)
	return r
}

func (*attachInstanceENIResponder) Name() string { return "attach instance ENI responder" }

func (r *attachInstanceENIResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleAttachMessage
}

func (r *attachInstanceENIResponder) handleAttachMessage(message *ecsacs.AttachInstanceNetworkInterfacesMessage) {
	logger.Debug(fmt.Sprintf("Handling %s", AttachInstanceENIMessageName))
	receivedAt := time.Now()

	// Validate fields in the message.
	if err := validateAttachInstanceNetworkInterfacesMessage(message); err != nil {
		logger.Error(fmt.Sprintf("Error validating %s received from ECS", AttachInstanceENIMessageName), logger.Fields{
			field.Error: err,
		})
		return
	}

	// Handle ENIs in the message.
	messageID := aws.StringValue(message.MessageId)
	clusterARN := aws.StringValue(message.ClusterArn)
	containerInstanceARN := aws.StringValue(message.ContainerInstanceArn)
	waitTimeoutMs := aws.Int64Value(message.WaitTimeoutMs)
	for _, mENI := range message.ElasticNetworkInterfaces {
		go r.handleInstanceENIFromMessage(mENI, messageID, clusterARN, containerInstanceARN, receivedAt, waitTimeoutMs)
	}

	// Send ACK.
	go func() {
		err := r.respond(&ecsacs.AckRequest{
			Cluster:           message.ClusterArn,
			ContainerInstance: message.ContainerInstanceArn,
			MessageId:         message.MessageId,
		})
		if err != nil {
			logger.Warn(fmt.Sprintf("Error acknowledging %s", AttachInstanceENIMessageName), logger.Fields{
				field.MessageID: messageID,
				field.Error:     err,
			})
		}
	}()
}

// handleInstanceENIFromMessage handles the attachment of a given instance ENI from an
// AttachInstanceNetworkInterfacesMessage.
func (r *attachInstanceENIResponder) handleInstanceENIFromMessage(eni *ecsacs.ElasticNetworkInterface,
	messageID, clusterARN, containerInstanceARN string, receivedAt time.Time, waitTimeoutMs int64) {
	expiresAt := receivedAt.Add(time.Duration(waitTimeoutMs) * time.Millisecond)
	err := r.eniHandler.HandleENIAttachment(&ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			AttachmentARN:        aws.StringValue(eni.AttachmentArn),
			Status:               status.AttachmentNone,
			ExpiresAt:            expiresAt,
			AttachStatusSent:     false,
			ClusterARN:           clusterARN,
			ContainerInstanceARN: containerInstanceARN,
		},
		AttachmentType: ni.ENIAttachmentTypeInstanceENI,
		MACAddress:     aws.StringValue(eni.MacAddress),
	})
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to handle %s", AttachInstanceENIMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
	}
}

// validateAttachInstanceNetworkInterfacesMessage performs validation checks on the
// AttachInstanceNetworkInterfacesMessage.
func validateAttachInstanceNetworkInterfacesMessage(message *ecsacs.AttachInstanceNetworkInterfacesMessage) error {
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

	timeout := aws.Int64Value(message.WaitTimeoutMs)
	if timeout <= 0 {
		return errors.Errorf("Invalid timeout set for message ID %s", messageID)
	}

	enis := message.ElasticNetworkInterfaces
	if len(enis) < 1 {
		return errors.Errorf("No ENIs for message ID %s", messageID)
	}

	for _, eni := range enis {
		err := ni.ValidateENI(eni)
		if err != nil {
			return err
		}
	}

	return nil
}
