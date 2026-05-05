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

package task

import (
	"fmt"

	"github.com/aws/amazon-ecs-agent/agent/api/serviceconnect"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// AttachmentHandler defines an interface to handle attachment received from ACS.
type AttachmentHandler interface {
	parseAttachment(acsAttachment *ecsacs.Attachment) error
	validateAttachment(acsTask *ecsacs.Task, task *Task) error
}

// ServiceConnectAttachmentHandler defines a service connect type attachment handler.
type ServiceConnectAttachmentHandler struct {
	scConfig *serviceconnect.Config
}

// NewAttachmentHandlers returns all type of handlers to handle different types of attachment.
func NewAttachmentHandlers() map[string]AttachmentHandler {
	attachmentHandlers := make(map[string]AttachmentHandler)
	attachmentHandlers[serviceConnectAttachmentType] = &ServiceConnectAttachmentHandler{}
	return attachmentHandlers
}

// getHandlerByType returns the attachment handler based on the given type, and returns error if no matching hander can be found.
func getHandlerByType(handlerType string, handlers map[string]AttachmentHandler) (AttachmentHandler, error) {
	if handler, ok := handlers[handlerType]; ok {
		return handler, nil
	}
	return nil, fmt.Errorf("error to find an attachment handler for %s attachment type", handlerType)
}

// attachment parser of service connect attachment handler.
func (scAttachment *ServiceConnectAttachmentHandler) parseAttachment(acsAttachment *ecsacs.Attachment) error {
	config, err := serviceconnect.ParseServiceConnectAttachment(acsAttachment)
	scAttachment.scConfig = config
	return err
}

// attachment validator of service connect attachment handler.
func (scAttachment *ServiceConnectAttachmentHandler) validateAttachment(acsTask *ecsacs.Task, task *Task) error {
	config := scAttachment.scConfig
	taskContainers := acsTask.Containers
	ipv6Enabled := false
	networkMode := task.NetworkMode
	if acsTask.ElasticNetworkInterfaces != nil {
		for _, eni := range acsTask.ElasticNetworkInterfaces {
			if len(eni.Ipv6Addresses) != 0 {
				ipv6Enabled = true
				break
			}
		}
	}
	return serviceconnect.ValidateServiceConnectConfig(config, taskContainers, networkMode, ipv6Enabled)
}

// handleTaskAttachments parses and validates attachments based on attachment type.
func handleTaskAttachments(acsTask *ecsacs.Task, task *Task) error {
	if acsTask.Attachments != nil {
		var serviceConnectAttachment *ecsacs.Attachment
		var ebsVolumeAttachments []*ecsacs.Attachment
		var s3filesVolumeAttachments []*ecsacs.Attachment
		for _, attachment := range acsTask.Attachments {
			switch aws.ToString(attachment.AttachmentType) {
			case serviceConnectAttachmentType:
				serviceConnectAttachment = attachment
			case apiresource.EBSTaskAttach:
				ebsVolumeAttachments = append(ebsVolumeAttachments, attachment)
			case apiresource.S3FilesTaskAttach:
				s3filesVolumeAttachments = append(s3filesVolumeAttachments, attachment)
			default:
				logger.Debug("Received an attachment type", logger.Fields{
					"attachmentType": attachment.AttachmentType,
				})
			}
		}

		handlers := NewAttachmentHandlers()
		if serviceConnectAttachment != nil {
			scHandler, err := getHandlerByType(serviceConnectAttachmentType, handlers)
			if err != nil {
				return err
			}

			if err := scHandler.(*ServiceConnectAttachmentHandler).parseAttachment(serviceConnectAttachment); err != nil {
				return fmt.Errorf("error parsing service connect config value from the service connect attachment: %w", err)
			}

			// validate the service connect config parsed from the service connect attachment
			if err := scHandler.(*ServiceConnectAttachmentHandler).validateAttachment(acsTask, task); err != nil {
				return fmt.Errorf("service connect config validation failed: %w", err)
			}
			task.ServiceConnectConfig = scHandler.(*ServiceConnectAttachmentHandler).scConfig
		}
		if len(ebsVolumeAttachments) > 0 {
			ebsVolumes := make(map[string]bool)
			for _, attachment := range ebsVolumeAttachments {
				ebs, err := taskresourcevolume.ParseEBSTaskVolumeAttachment(attachment)
				if err != nil {
					return fmt.Errorf("unable to parse and validate EBS volume: %w", err)
				}
				taskVolume := TaskVolume{
					Name:   ebs.VolumeName,
					Type:   apiresource.EBSTaskAttach,
					Volume: ebs,
				}
				ebsVolumes[ebs.VolumeName] = true
				task.Volumes = append(task.Volumes, taskVolume)
			}
			// Remove all incorrect volume configurations that were initially passed over from ACS.
			// We rebuild the slice to avoid index-shifting bugs when removing multiple entries.
			filtered := make([]TaskVolume, 0, len(task.Volumes))
			for _, tv := range task.Volumes {
				if ebsVolumes[tv.Name] && tv.Type != apiresource.EBSTaskAttach {
					continue
				}
				filtered = append(filtered, tv)
			}
			task.Volumes = filtered
		}
		if len(s3filesVolumeAttachments) > 0 {
			s3filesVolumes := make(map[string]bool)
			for _, attachment := range s3filesVolumeAttachments {
				s3cfg, err := taskresourcevolume.ParseS3FilesTaskVolumeAttachment(attachment)
				if err != nil {
					return fmt.Errorf("unable to parse and validate S3 Files volume: %w", err)
				}
				taskVolume := TaskVolume{
					Name:   s3cfg.VolumeName,
					Type:   apiresource.S3FilesTaskAttach,
					Volume: s3cfg,
				}
				s3filesVolumes[s3cfg.VolumeName] = true
				task.Volumes = append(task.Volumes, taskVolume)
			}
			// Remove incomplete volumes with same name from ACS.
			// We rebuild the slice to avoid index-shifting bugs when removing multiple entries.
			filtered := make([]TaskVolume, 0, len(task.Volumes))
			for _, tv := range task.Volumes {
				if s3filesVolumes[tv.Name] && tv.Type != apiresource.S3FilesTaskAttach {
					continue
				}
				filtered = append(filtered, tv)
			}
			task.Volumes = filtered
		}
	}
	return nil
}
