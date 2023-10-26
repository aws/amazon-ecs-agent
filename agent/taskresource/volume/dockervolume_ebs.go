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

package volume

import (
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	missingEbsEntryFormart = `missing %s in the EBS config`
)

type EBSTaskVolumeConfig struct {
	VolumeId             string `json:"volumeId"`
	VolumeName           string `json:"volumeName"`
	VolumeSizeGib        string `json:"volumeSizeGib"`
	SourceVolumeHostPath string `json:"sourceVolumeHostPath"`
	DeviceName           string `json:"deviceName"`
	// FileSystem is optional and will be present if customers explicitly set this in their task definition
	FileSystem string `json:"fileSystem"`
	// DockerVolumeName is internal docker name for this volume.
	DockerVolumeName string `json:"dockerVolumeName"`
}

// ParseEBSTaskVolumeAttachment parses the ebs task volume config value
// from the given attachment.
func ParseEBSTaskVolumeAttachment(ebsAttachment *ecsacs.Attachment) (*EBSTaskVolumeConfig, error) {
	ebsTaskVolumeConfig := &EBSTaskVolumeConfig{}
	for _, property := range ebsAttachment.AttachmentProperties {
		if property == nil {
			return nil, fmt.Errorf("failed to parse task ebs attachment, encountered nil property")
		}
		if aws.StringValue(property.Value) == "" {
			return nil, fmt.Errorf("failed to parse task ebs attachment, encountered empty value for property: %s", aws.StringValue(property.Name))
		}
		switch aws.StringValue(property.Name) {
		case apiresource.VolumeIdKey:
			ebsTaskVolumeConfig.VolumeId = aws.StringValue(property.Value)
		case apiresource.VolumeSizeGibKey:
			ebsTaskVolumeConfig.VolumeSizeGib = aws.StringValue(property.Value)
		case apiresource.DeviceNameKey:
			ebsTaskVolumeConfig.DeviceName = aws.StringValue(property.Value)
		case apiresource.SourceVolumeHostPathKey:
			ebsTaskVolumeConfig.SourceVolumeHostPath = aws.StringValue(property.Value)
		case apiresource.VolumeNameKey:
			ebsTaskVolumeConfig.VolumeName = aws.StringValue(property.Value)
		case apiresource.FileSystemKey:
			ebsTaskVolumeConfig.FileSystem = aws.StringValue(property.Value)
		default:
			logger.Warn("Received an unrecognized attachment property", logger.Fields{
				"attachmentProperty": property.String(),
			})
		}
	}
	err := validateEBSTaskVolumeAttachment(ebsTaskVolumeConfig)
	if err != nil {
		return nil, err
	}

	return ebsTaskVolumeConfig, nil
}

// validateEBSTaskVolumeAttachment validates that none of the following required attachment properties
// are missing from the EBS configuration after parsing.
// Required Properties: VolumeId, VolumeName, and SourceVolumeHostPath
func validateEBSTaskVolumeAttachment(cfg *EBSTaskVolumeConfig) error {
	if cfg == nil {
		return fmt.Errorf("empty EBS task volume configuration")
	}
	if cfg.VolumeId == "" {
		return fmt.Errorf(missingEbsEntryFormart, apiresource.VolumeIdKey)
	}
	if cfg.VolumeName == "" {
		return fmt.Errorf(missingEbsEntryFormart, apiresource.VolumeNameKey)
	}
	if cfg.SourceVolumeHostPath == "" {
		return fmt.Errorf(missingEbsEntryFormart, apiresource.SourceVolumeHostPathKey)
	}
	return nil
}
