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
	AttachTaskENIMessageName = "AttachTaskNetworkInterfacesMessage"
)

// attachTaskENIResponder implements the wsclient.RequestResponder interface for responding
// to ecsacs.AttachTaskNetworkInterfacesMessage messages sent by ACS.
type attachTaskENIResponder struct {
	eniHandler ENIHandler
	respond    wsclient.RespondFunc
}

// NewAttachTaskENIResponder returns an instance of the attachTaskENIResponder struct.
func NewAttachTaskENIResponder(eniHandler ENIHandler, responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &attachTaskENIResponder{
		eniHandler: eniHandler,
	}
	r.respond = ResponseToACSSender(r.Name(), responseSender)
	return r
}

func (*attachTaskENIResponder) Name() string { return "attach task ENI responder" }

func (r *attachTaskENIResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleAttachMessage
}

func (r *attachTaskENIResponder) handleAttachMessage(message *ecsacs.AttachTaskNetworkInterfacesMessage) {
	logger.Debug(fmt.Sprintf("Handling %s", AttachTaskENIMessageName))
	receivedAt := time.Now()

	// Validate fields in the message.
	if err := validateAttachTaskNetworkInterfacesMessage(message); err != nil {
		logger.Error(fmt.Sprintf("Error validating %s received from ECS", AttachTaskENIMessageName), logger.Fields{
			field.Error: err,
		})
		return
	}

	// Handle ENIs in the message.
	messageID := aws.StringValue(message.MessageId)
	taskARN := aws.StringValue(message.TaskArn)
	clusterARN := aws.StringValue(message.ClusterArn)
	containerInstanceARN := aws.StringValue(message.ContainerInstanceArn)
	waitTimeoutMs := aws.Int64Value(message.WaitTimeoutMs)
	for _, mENI := range message.ElasticNetworkInterfaces {
		go r.handleTaskENIFromMessage(mENI, messageID, taskARN, clusterARN, containerInstanceARN, receivedAt,
			waitTimeoutMs)
	}

	// Send ACK.
	go func() {
		err := r.respond(&ecsacs.AckRequest{
			Cluster:           message.ClusterArn,
			ContainerInstance: message.ContainerInstanceArn,
			MessageId:         message.MessageId,
		})
		if err != nil {
			logger.Warn(fmt.Sprintf("Error acknowledging %s", AttachTaskENIMessageName), logger.Fields{
				field.MessageID: messageID,
				field.Error:     err,
			})
		}
	}()
}

// handleTaskENIFromMessage handles the attachment of a given task ENI from an
// AttachTaskNetworkInterfacesMessage.
func (r *attachTaskENIResponder) handleTaskENIFromMessage(eni *ecsacs.ElasticNetworkInterface,
	messageID, taskARN, clusterARN, containerInstanceARN string, receivedAt time.Time, waitTimeoutMs int64) {
	expiresAt := receivedAt.Add(time.Duration(waitTimeoutMs) * time.Millisecond)
	err := r.eniHandler.HandleENIAttachment(&ni.ENIAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              taskARN,
			AttachmentARN:        aws.StringValue(eni.AttachmentArn),
			Status:               status.AttachmentNone,
			ExpiresAt:            expiresAt,
			AttachStatusSent:     false,
			ClusterARN:           clusterARN,
			ContainerInstanceARN: containerInstanceARN,
		},
		AttachmentType: ni.ENIAttachmentTypeTaskENI,
		MACAddress:     aws.StringValue(eni.MacAddress),
	})
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to handle %s", AttachTaskENIMessageName), logger.Fields{
			field.MessageID: messageID,
			field.Error:     err,
		})
	}
}

// validateAttachTaskNetworkInterfacesMessage performs validation checks on the
// AttachTaskNetworkInterfacesMessage.
func validateAttachTaskNetworkInterfacesMessage(message *ecsacs.AttachTaskNetworkInterfacesMessage) error {
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

	taskArn := aws.StringValue(message.TaskArn)
	if taskArn == "" {
		return errors.Errorf("taskArn is not set for message ID %s", messageID)
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
