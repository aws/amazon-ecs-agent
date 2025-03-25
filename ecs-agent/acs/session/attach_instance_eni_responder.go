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

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acs"
	acstypes "github.com/aws/aws-sdk-go-v2/service/acs/types"
	"github.com/pkg/errors"
)

const (
	AttachInstanceENIMessageName = "AttachInstanceNetworkInterfacesInput"
)

// attachInstanceENIResponder implements the wsclient.RequestResponder interface for responding
// to acs.AttachInstanceNetworkInterfacesInput messages sent by ACS.
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
	r.respond = responseToACSSender(r.Name(), responseSender)
	return r
}

func (*attachInstanceENIResponder) Name() string { return "attach instance ENI responder" }

func (r *attachInstanceENIResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleAttachMessage
}

func (r *attachInstanceENIResponder) handleAttachMessage(message *acs.AttachInstanceNetworkInterfacesInput) {
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
	messageID := aws.ToString(message.MessageId)
	clusterARN := aws.ToString(message.ClusterArn)
	containerInstanceARN := aws.ToString(message.ContainerInstanceArn)
	waitTimeoutMs := aws.ToInt64(message.WaitTimeoutMs)
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
// AttachInstanceNetworkInterfacesInput.
func (r *attachInstanceENIResponder) handleInstanceENIFromMessage(eni acstypes.ElasticNetworkInterface,
	messageID, clusterARN, containerInstanceARN string, receivedAt time.Time, waitTimeoutMs int64) {
	expiresAt := receivedAt.Add(time.Duration(waitTimeoutMs) * time.Millisecond)
	err := r.eniHandler.HandleENIAttachment(&ni.ENIAttachment{
		AttachmentInfo: attachment.AttachmentInfo{
			AttachmentARN:        aws.ToString(eni.AttachmentArn),
			Status:               attachment.AttachmentNone,
			ExpiresAt:            expiresAt,
			AttachStatusSent:     false,
			ClusterARN:           clusterARN,
			ContainerInstanceARN: containerInstanceARN,
		},
		AttachmentType: ni.ENIAttachmentTypeInstanceENI,
		MACAddress:     aws.ToString(eni.MacAddress),
	})
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to handle %s", AttachInstanceENIMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
	}
}

// validateAttachInstanceNetworkInterfacesMessage performs validation checks on the
// AttachInstanceNetworkInterfacesInput.
func validateAttachInstanceNetworkInterfacesMessage(message *acs.AttachInstanceNetworkInterfacesInput) error {
	if message == nil {
		return errors.Errorf("Message is empty")
	}

	messageID := aws.ToString(message.MessageId)
	if messageID == "" {
		return errors.Errorf("Message ID is not set")
	}

	clusterArn := aws.ToString(message.ClusterArn)
	if clusterArn == "" {
		return errors.Errorf("clusterArn is not set for message ID %s", messageID)
	}

	containerInstanceArn := aws.ToString(message.ContainerInstanceArn)
	if containerInstanceArn == "" {
		return errors.Errorf("containerInstanceArn is not set for message ID %s", messageID)
	}

	timeout := aws.ToInt64(message.WaitTimeoutMs)
	if timeout <= 0 {
		return errors.Errorf("Invalid timeout set for message ID %s", messageID)
	}

	enis := message.ElasticNetworkInterfaces
	if len(enis) < 1 {
		return errors.Errorf("No ENIs for message ID %s", messageID)
	}

	for _, eni := range enis {
		err := ni.ValidateENI(&eni)
		if err != nil {
			return err
		}
	}

	return nil
}
