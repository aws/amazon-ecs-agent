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
	"github.com/pkg/errors"
)

// ValidateResourceByResourceType checks if the provided resource type is valid, as well as if the attachment
// properties of the specified resource are valid.
func ValidateResourceByResourceType(resourceAttachmentProperties map[string]string) error {
	resourceType, ok := resourceAttachmentProperties[ResourceTypeName]
	if !ok {
		return errors.New("resource attachment validation: no resourceType found")
	}

	err := validateCommonAttachmentProperties(resourceAttachmentProperties)
	if err != nil {
		return errors.Wrapf(err, "failed to validate resource type %s", resourceType)
	}

	switch resourceType {
	case EphemeralStorage:
		err = validateEphemeralStorageProperties(resourceAttachmentProperties)
	case ElasticBlockStorage:
		err = validateVolumeAttachmentProperties(resourceAttachmentProperties)
	default:
		return errors.Errorf("unknown resourceType provided: %s", resourceType)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to validate resource type %s", resourceType)
	}
	return nil
}

func validateEphemeralStorageProperties(properties map[string]string) error {
	err := validateVolumeAttachmentProperties(properties)
	if err != nil {
		return err
	}

	return ValidateRequiredProperties(properties, getExtensibleEphemeralStorageProperties())
}

// validateCommonAttachmentProperties checks if all required common properties exist for an attachment
func validateCommonAttachmentProperties(resourceAttachmentProperties map[string]string) error {
	return ValidateRequiredProperties(resourceAttachmentProperties, getCommonProperties())
}

// validateVolumeAttachmentProperties checks if all required properties exist for a given volume attachment.
func validateVolumeAttachmentProperties(volumeAttachmentProperties map[string]string) error {
	return ValidateRequiredProperties(volumeAttachmentProperties, getVolumeSpecificProperties())
}

func ValidateRequiredProperties(actualProperties map[string]string, requiredProperties []string) error {
	for _, property := range requiredProperties {
		if _, ok := actualProperties[property]; !ok {
			return errors.Errorf("property %s not found in attachment properties", property)
		}
	}
	return nil
}

// For EBS-backed task attachment payload, the file system type is optional. If we do receive a file system type value,
// we want to validate what we receive is one of the following types [xfs, ext2, ext3, ext4, ntfs].
func ValidateFileSystemType(filesystemType string) error {
	if filesystemType != "" && !allowedFSTypes[filesystemType] {
		return errors.Errorf("invalid file system type: %s", filesystemType)
	}
	return nil
}
