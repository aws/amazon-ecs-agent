// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package resource

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime"
)

type ResourceAttachment struct {
	attachmentinfo.AttachmentInfo
	// AttachmentProperties is a map storing (name, value) representation of attachment properties.
	// Each pair is a set of property of one resource attachment.
	// The "FargateResourceId" is a property name that will be present for all resources.
	// Other properties can vary based on the resource.
	// For example, if the attachment is used for an EBS volume resource, the additional properties will be
	// the customer specified volume size, and the image cache size.
	AttachmentProperties map[string]string `json:"AttachmentProperties,omitempty"`
	// ackTimer is used to register the expiration timeout callback for unsuccessful
	// Resource attachments
	ackTimer ttime.Timer
	// guard protects access to fields of this struct
	guard sync.RWMutex
	err   error
}

// Agent Communication Service (ACS) can send messages of type ConfirmAttachmentMessage. These messages include
// an attachment, and map of associated properties. The below list contains attachment properties which Agent can use
// to validate various types of attachments.
const (
	// Common properties.
	ResourceTypeName = "resourceType"

	// Properties specific to volumes.
	VolumeIdName = "volumeID"
	DeviceName   = "deviceName" // name of the block device on the instance where the volume is attached

	// Properties specific to resources provisioned by Fargate Control Plane.
	FargateResourceIdName = "resourceID"

	// Properties specific to Extensible Ephemeral Storage (EES).
	VolumeSizeInGiBName = "volumeSizeInGiB"    // the total size of the EES (requested size + image cache size)
	RequestedSizeName   = "requestedSizeInGiB" // the customer requested size of extensible ephemeral storage

	// Properties specific to Elastic Block Service Volumes
	FileSystemTypeName = "filesystemType"
)

// getCommonProperties returns the common properties as used for validating a resource.
func getCommonProperties() (commonProperties []string) {
	commonProperties = []string{
		ResourceTypeName,
	}
	return commonProperties
}

// getVolumeSpecificProperties returns the properties specific to volume resources.
func getVolumeSpecificProperties() (volumeSpecificProperties []string) {
	volumeSpecificProperties = []string{
		VolumeIdName,
		DeviceName,
	}
	return volumeSpecificProperties
}

// getFargateControlPlaneProperties returns the properties specific to resources provisioned by Fargate control plane.
func getFargateControlPlaneProperties() (fargateCpProperties []string) {
	fargateCpProperties = []string{
		FargateResourceIdName,
	}
	return fargateCpProperties
}

// getExtensibleEphemeralStorageProperties returns the properties specific to extensible ephemeral storage resources.
func getExtensibleEphemeralStorageProperties() (ephemeralStorageProperties []string) {
	ephemeralStorageProperties = []string{
		VolumeSizeInGiBName,
		RequestedSizeName,
	}
	ephemeralStorageProperties = append(ephemeralStorageProperties, getFargateControlPlaneProperties()...)
	return ephemeralStorageProperties
}

func getResourceAttachmentLogFields(ra *ResourceAttachment, duration time.Duration) logger.Fields {
	fields := logger.Fields{
		"duration":          duration.String(),
		"attachmentARN":     ra.AttachmentARN,
		"attachmentType":    ra.AttachmentProperties[ResourceTypeName],
		"attachmentSent":    ra.AttachStatusSent,
		"volumeSizeInGiB":   ra.AttachmentProperties[VolumeSizeInGiBName],
		"requestedSizeName": ra.AttachmentProperties[RequestedSizeName],
		"volumeId":          ra.AttachmentProperties[VolumeIdName],
		"deviceName":        ra.AttachmentProperties[DeviceName],
		"filesystemType":    ra.AttachmentProperties[FileSystemTypeName],
		"status":            ra.Status.String(),
		"expiresAt":         ra.ExpiresAt.Format(time.RFC3339),
	}

	return fields
}

// StartTimer starts the ack timer to record the expiration of resource attachment
func (ra *ResourceAttachment) StartTimer(timeoutFunc func()) error {
	ra.guard.Lock()
	defer ra.guard.Unlock()

	if ra.ackTimer != nil {
		// The timer has already been initialized, do nothing
		return nil
	}
	now := time.Now()
	duration := ra.ExpiresAt.Sub(now)
	if duration <= 0 {
		return fmt.Errorf("resource attachment: timer expiration is in the past; expiration [%s] < now [%s]",
			ra.ExpiresAt.String(), now.String())
	}
	logger.Info("Starting resource attachment ack timer", getResourceAttachmentLogFields(ra, duration))
	ra.ackTimer = time.AfterFunc(duration, timeoutFunc)
	return nil
}

// Initialize initializes the fields that can't be populated from loading state file.
// Notably, this initializes the ack timer so that if we time out waiting for the resource to be attached, the attachment
// can be removed from state.
func (ra *ResourceAttachment) Initialize(timeoutFunc func()) error {
	ra.guard.Lock()
	defer ra.guard.Unlock()

	if ra.AttachStatusSent { // resource attachment status has been sent, no need to start ack timer.
		return nil
	}

	now := time.Now()
	duration := ra.ExpiresAt.Sub(now)
	if duration <= 0 {
		return errors.New("resource attachment has already expired")
	}

	logger.Info("Starting Resource attachment ack timer", getResourceAttachmentLogFields(ra, duration))
	ra.ackTimer = time.AfterFunc(duration, timeoutFunc)

	return nil
}

// IsSent checks if the resource attachment attached status has been sent
func (ra *ResourceAttachment) IsSent() bool {
	ra.guard.RLock()
	defer ra.guard.RUnlock()

	return ra.AttachStatusSent
}

// SetSentStatus marks the resource attachment attached status has been sent
func (ra *ResourceAttachment) SetSentStatus() {
	ra.guard.Lock()
	defer ra.guard.Unlock()

	ra.AttachStatusSent = true
}

// IsAttached checks if the resource attachment has been found attached on the host
func (ra *ResourceAttachment) IsAttached() bool {
	ra.guard.RLock()
	defer ra.guard.RUnlock()

	return ra.Status == status.AttachmentAttached
}

// SetAttachedStatus marks the resouce attachment as attached once it's been found on the host
func (ra *ResourceAttachment) SetAttachedStatus() {
	ra.guard.Lock()
	defer ra.guard.Unlock()

	ra.Status = status.AttachmentAttached
}

// StopAckTimer stops the ack timer set on the resource attachment
func (ra *ResourceAttachment) StopAckTimer() {
	ra.guard.Lock()
	defer ra.guard.Unlock()

	ra.ackTimer.Stop()
}

// HasExpired returns true if the resource attachment object has exceeded the
// threshold for notifying the backend of the attachment
func (ra *ResourceAttachment) HasExpired() bool {
	ra.guard.RLock()
	defer ra.guard.RUnlock()

	return time.Now().After(ra.ExpiresAt)
}

// SetError sets the error for a resource attachment if it can't be found.
func (ra *ResourceAttachment) SetError(err error) {
	ra.guard.Lock()
	defer ra.guard.Unlock()
	ra.err = err
}

// GetError returns the error field for a resource attachment.
func (ra *ResourceAttachment) GetError() error {
	ra.guard.RLock()
	defer ra.guard.RUnlock()

	return ra.err
}

// EBSToString returns a string representation of an EBS volume resource attachment.
func (ra *ResourceAttachment) EBSToString() string {
	ra.guard.RLock()
	defer ra.guard.RUnlock()

	return ra.ebsToStringUnsafe()
}

func (ra *ResourceAttachment) ebsToStringUnsafe() string {
	return fmt.Sprintf(
		"Resource Attachment: attachment=%s attachmentType=%s attachmentSent=%t volumeSizeInGiB=%s requestedSizeName=%s volumeId=%s deviceName=%s filesystemType=%s status=%s expiresAt=%s error=%v",
		ra.AttachmentARN, ra.AttachmentProperties[ResourceTypeName], ra.AttachStatusSent, ra.AttachmentProperties[VolumeSizeInGiBName], ra.AttachmentProperties[RequestedSizeName], ra.AttachmentProperties[VolumeIdName],
		ra.AttachmentProperties[DeviceName], ra.AttachmentProperties[FileSystemTypeName], ra.Status.String(), ra.ExpiresAt.Format(time.RFC3339), ra.err)
}

// GetAttachmentProperties returns the specific attachment property of the resource attachment object
func (ra *ResourceAttachment) GetAttachmentProperties(key string) string {
	ra.guard.RLock()
	defer ra.guard.RUnlock()
	val, ok := ra.AttachmentProperties[key]
	if ok {
		return val
	}
	return ""
}
