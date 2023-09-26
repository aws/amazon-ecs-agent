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
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/wsclient"

	"github.com/aws/aws-sdk-go/aws"
	awsARN "github.com/aws/aws-sdk-go/aws/arn"
	"github.com/pkg/errors"
)

const (
	AttachResourceMessageName = "ConfirmAttachmentMessage"
	// DefaultAttachmentWaitTimeoutInMs is the default timeout, 5 minutes, for handling the attachments from ACS.
	DefaultAttachmentWaitTimeoutInMs = 300000
)

type ResourceHandler interface {
	HandleResourceAttachment(Attachment *resource.ResourceAttachment)
}

// attachResourceResponder implements the wsclient.RequestResponder interface for responding
// to ecsacs.ConfirmAttachmentMessage messages sent by ACS.
type attachResourceResponder struct {
	resourceHandler ResourceHandler
	metricsFactory  metrics.EntryFactory
	respond         wsclient.RespondFunc
}

func NewAttachResourceResponder(resourceHandler ResourceHandler, metricsFactory metrics.EntryFactory,
	responseSender wsclient.RespondFunc) wsclient.RequestResponder {
	r := &attachResourceResponder{
		resourceHandler: resourceHandler,
		metricsFactory:  metricsFactory,
	}
	r.respond = responseToACSSender(r.Name(), responseSender)
	return r
}

func (*attachResourceResponder) Name() string { return "attach resource responder" }

func (r *attachResourceResponder) HandlerFunc() wsclient.RequestHandler {
	return r.handleAttachMessage
}

func (r *attachResourceResponder) handleAttachMessage(message *ecsacs.ConfirmAttachmentMessage) {
	logger.Debug(fmt.Sprintf("Handling %s", AttachResourceMessageName))
	receivedAt := time.Now()

	// Validate fields in the message.
	attachmentProperties, err := validateAttachResourceMessage(message)
	r.metricsFactory.New(metrics.ResourceValidationMetricName).Done(err)
	if err != nil {
		logger.Error(fmt.Sprintf("Error validating %s received from ECS", AttachResourceMessageName), logger.Fields{
			field.Error: err,
		})
		return
	}

	messageID := aws.StringValue(message.MessageId)
	// Set a default wait timeout (5m) for the attachment message from ACS if not provided.
	// For example, the attachment payload for the EBS attach might not have the property.
	waitTimeoutMs := aws.Int64Value(message.WaitTimeoutMs)
	if waitTimeoutMs == 0 {
		waitTimeoutMs = DefaultAttachmentWaitTimeoutInMs
	}
	logger.Debug("Waiting for the resource attachment to be ready",
		logger.Fields{
			"WaitTimeoutMs": waitTimeoutMs,
		})
	expiresAt := receivedAt.Add(time.Duration(waitTimeoutMs) * time.Millisecond)
	go r.resourceHandler.HandleResourceAttachment(&resource.ResourceAttachment{
		AttachmentInfo: attachmentinfo.AttachmentInfo{
			TaskARN:              aws.StringValue(message.TaskArn),
			TaskClusterARN:       aws.StringValue(message.TaskClusterArn),
			ClusterARN:           aws.StringValue(message.ClusterArn),
			ContainerInstanceARN: aws.StringValue(message.ContainerInstanceArn),
			ExpiresAt:            expiresAt,
			AttachmentARN:        aws.StringValue(message.Attachment.AttachmentArn),
			Status:               status.AttachmentNone,
		},
		AttachmentProperties: attachmentProperties,
		AttachmentType:       aws.StringValue(message.Attachment.AttachmentType),
	})

	// Send ACK.
	go func() {
		err := r.respond(&ecsacs.AckRequest{
			Cluster:           message.ClusterArn,
			ContainerInstance: message.ContainerInstanceArn,
			MessageId:         message.MessageId,
		})
		if err != nil {
			logger.Warn(fmt.Sprintf("Error acknowledging %s", AttachResourceMessageName), logger.Fields{
				field.MessageID: messageID,
				field.Error:     err,
			})
		}
	}()
}

// validateAttachResourceMessage performs validation checks on the ConfirmAttachmentMessage
// and returns the attachment properties received from validateAttachmentAndReturnProperties()
func validateAttachResourceMessage(message *ecsacs.ConfirmAttachmentMessage) (
	attachmentProperties map[string]string, err error) {
	if message == nil {
		return nil, errors.New("Message is empty")
	}

	messageID := aws.StringValue(message.MessageId)
	if messageID == "" {
		return nil, errors.New("Message ID is not set")
	}

	clusterArn := aws.StringValue(message.ClusterArn)
	_, err = awsARN.Parse(clusterArn)
	if err != nil {
		return nil, errors.Errorf("Invalid clusterArn specified for message ID %s", messageID)
	}

	containerInstanceArn := aws.StringValue(message.ContainerInstanceArn)
	_, err = awsARN.Parse(containerInstanceArn)
	if err != nil {
		return nil, errors.Errorf(
			"Invalid containerInstanceArn specified for message ID %s", messageID)
	}

	attachment := message.Attachment
	if attachment == nil {
		return nil, errors.Errorf(
			"No resource attachment for message ID %s", messageID)
	}

	attachmentProperties, err = validateAttachmentAndReturnProperties(message)
	if err != nil {
		return nil, errors.Wrap(err, "unable to validate resource")
	}

	return attachmentProperties, nil
}

// validateAttachment performs validation checks on the attachment contained in the ConfirmAttachmentMessage
// and returns the attachment's properties
func validateAttachmentAndReturnProperties(message *ecsacs.ConfirmAttachmentMessage) (
	attachmentProperties map[string]string, err error) {
	attachment := message.Attachment

	arn := aws.StringValue(attachment.AttachmentArn)
	_, err = awsARN.Parse(arn)
	if err != nil {
		return nil, errors.Errorf(
			"resource attachment validation: invalid arn %s specified for attachment: %s", arn, attachment.String())
	}

	attachmentProperties = make(map[string]string)
	properties := attachment.AttachmentProperties
	for _, property := range properties {
		name := aws.StringValue(property.Name)
		if name == "" {
			return nil, errors.Errorf(
				"resource attachment validation: no name specified for attachment property: %s", property.String())
		}

		value := aws.StringValue(property.Value)
		if value == "" {
			return nil, errors.Errorf(
				"resource attachment validation: no value specified for attachment property: %s", property.String())
		}

		attachmentProperties[name] = value
	}

	// For "EBSTaskAttach" used by the EBS attach, ACS is using attachmentType to indicate its attachment type.
	attachmentType := aws.StringValue(message.Attachment.AttachmentType)
	if attachmentType == resource.EBSTaskAttach {
		err = resource.ValidateRequiredProperties(
			attachmentProperties,
			resource.GetVolumeSpecificPropertiesForEBSAttach(),
		)
		if err != nil {
			return nil, errors.Wrap(err, "resource attachment validation by attachment type failed")
		}
		return attachmentProperties, nil
	}

	// For legacy EBS volumes("EphemeralStorage" and "ElasticBlockStorage"), ACS is using resourceType to indicate its attachment type.
	err = resource.ValidateResourceByResourceType(attachmentProperties)
	if err != nil {
		return nil, errors.Wrap(err, "resource attachment validation by resource type failed ")
	}

	return attachmentProperties, nil
}
