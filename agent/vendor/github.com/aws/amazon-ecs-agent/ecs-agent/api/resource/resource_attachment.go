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
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachmentinfo"
)

// The ResourceAttachment is a general attachment for resources created specifically for Fargate launch type
// (e.g., EBS volume, V2N).
type ResourceAttachment struct {
	attachmentinfo.AttachmentInfo
	// AttachmentProperties is a map storing (name, value) representation of attachment properties.
	// Each pair is a set of property of one resource attachment.
	// The "FargateResourceId" is a property name that will be present for all resources.
	// Other properties can vary based on the resource.
	// For example, if the attachment is used for an EBS volume resource, the additional properties will be
	// the customer specified volume size, and the image cache size.
	AttachmentProperties map[string]string `json:"AttachmentProperties,omitempty"`
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
