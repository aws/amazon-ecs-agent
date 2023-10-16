//go:build unit
// +build unit

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

package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateVolumeResource tests that validation for volume resource attachments takes place properly.
func TestValidateVolumeResource(t *testing.T) {
	volumeAttachmentProperties := make(map[string]string)
	testString := "(╯°□°)╯︵ ┻━┻"

	for _, property := range getCommonProperties() {
		volumeAttachmentProperties[property] = testString
	}
	for _, property := range getVolumeSpecificProperties() {
		volumeAttachmentProperties[property] = testString
	}

	for _, tc := range []struct {
		resourceType string
		// resourceSpecificProperties is the list of properties specific to a resource type.
		resourceSpecificProperties []string
	}{
		{
			resourceType:               EphemeralStorage,
			resourceSpecificProperties: getExtensibleEphemeralStorageProperties(),
		},
		{
			resourceType: ElasticBlockStorage,
			// ElasticBlockStorage resource type does not have any specific properties.
			resourceSpecificProperties: []string{},
		},
	} {
		t.Run(tc.resourceType, func(t *testing.T) {
			resourceProperties := make(map[string]string, len(volumeAttachmentProperties))
			for k, v := range volumeAttachmentProperties {
				resourceProperties[k] = v
			}

			resourceProperties[ResourceTypeName] = tc.resourceType
			for _, property := range tc.resourceSpecificProperties {
				resourceProperties[property] = testString
			}

			err := ValidateResourceByResourceType(resourceProperties)
			require.NoError(t, err)

			// `requiredProperties` contains all properties required for a resource.
			// When any item from requiredProperties is missing from the resource's attachmentProperties,
			// we expect resource validation to fail.
			requiredProperties := make([]string, 0)
			for key := range resourceProperties {
				requiredProperties = append(requiredProperties, key)
			}
			// Test that we are validating for all properties by removing each property one at a time, and then resetting.
			for _, property := range requiredProperties {
				// Store the current value so that we can add it back when we reset after removing it.
				val := resourceProperties[property]
				delete(resourceProperties, property)
				err := ValidateResourceByResourceType(resourceProperties)
				require.Error(t, err)
				// Make resourceProperties whole again.
				resourceProperties[property] = val
			}
		})
	}
}

func TestValidateVolumeFileSystem(t *testing.T) {
	validFileSystems := []string{"xfs", "ext2", "ext3", "ext4", "ntfs"}

	for _, fs := range validFileSystems {
		err := ValidateFileSystemType(fs)
		require.NoError(t, err)
	}

	err := ValidateFileSystemType("someFilesystem")
	require.Error(t, err)
}
