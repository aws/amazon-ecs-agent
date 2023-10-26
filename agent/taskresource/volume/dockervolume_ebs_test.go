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
package volume

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	apiresource "github.com/aws/amazon-ecs-agent/ecs-agent/api/resource"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

const (
	testAttachmentArn = "att-arn-1"
)

func TestParseEBSTaskVolumeAttachmentHappyCase(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentArn:  aws.String(testAttachmentArn),
		AttachmentType: aws.String(apiresource.EBSTaskAttach),
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{
				Name:  aws.String(apiresource.VolumeIdKey),
				Value: aws.String(TestVolumeId),
			},
			{
				Name:  aws.String(apiresource.VolumeSizeGibKey),
				Value: aws.String(TestVolumeSizeGib),
			},
			{
				Name:  aws.String(apiresource.SourceVolumeHostPathKey),
				Value: aws.String(TestSourceVolumeHostPath),
			},
			{
				Name:  aws.String(apiresource.VolumeNameKey),
				Value: aws.String(TestVolumeName),
			},
			{
				Name:  aws.String(apiresource.FileSystemKey),
				Value: aws.String(TestFileSystem),
			},
			{
				Name:  aws.String(apiresource.DeviceNameKey),
				Value: aws.String(TestDeviceName),
			},
		},
	}

	expectedEBSCfg := &EBSTaskVolumeConfig{
		VolumeId:             "vol-12345",
		VolumeName:           "test-volume",
		VolumeSizeGib:        "10",
		SourceVolumeHostPath: "taskarn_vol-12345",
		DeviceName:           "/dev/nvme1n1",
		FileSystem:           "ext4",
	}

	parsedEBSCfg, err := ParseEBSTaskVolumeAttachment(attachment)
	assert.NoError(t, err, "unable to parse attachment")
	assert.Equal(t, expectedEBSCfg, parsedEBSCfg)
}

func TestParseEBSTaskVolumeAttachmentNilProperty(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentArn:  aws.String(testAttachmentArn),
		AttachmentType: aws.String(apiresource.EBSTaskAttach),
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			nil,
		},
	}

	_, err := ParseEBSTaskVolumeAttachment(attachment)
	assert.Error(t, err)
}

func TestParseEBSTaskVolumeAttachmentNilPropertyValue(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentArn:  aws.String(testAttachmentArn),
		AttachmentType: aws.String(apiresource.EBSTaskAttach),
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{
				Name:  aws.String(apiresource.VolumeIdKey),
				Value: nil,
			},
			{
				Name:  aws.String(apiresource.VolumeSizeGibKey),
				Value: aws.String(TestVolumeSizeGib),
			},
			{
				Name:  aws.String(apiresource.SourceVolumeHostPathKey),
				Value: aws.String(TestSourceVolumeHostPath),
			},
			{
				Name:  aws.String(apiresource.VolumeNameKey),
				Value: aws.String(TestVolumeName),
			},
			{
				Name:  aws.String(apiresource.FileSystemKey),
				Value: aws.String(TestFileSystem),
			},
			{
				Name:  aws.String(apiresource.DeviceNameKey),
				Value: aws.String(TestDeviceName),
			},
		},
	}

	_, err := ParseEBSTaskVolumeAttachment(attachment)
	assert.Error(t, err)
}

func TestParseEBSTaskVolumeAttachmentEmptyPropertyValue(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentArn:  aws.String(testAttachmentArn),
		AttachmentType: aws.String(apiresource.EBSTaskAttach),
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{
				Name:  aws.String(apiresource.VolumeIdKey),
				Value: aws.String(""),
			},
			{
				Name:  aws.String(apiresource.VolumeSizeGibKey),
				Value: aws.String(TestVolumeSizeGib),
			},
			{
				Name:  aws.String(apiresource.SourceVolumeHostPathKey),
				Value: aws.String(TestSourceVolumeHostPath),
			},
			{
				Name:  aws.String(apiresource.VolumeNameKey),
				Value: aws.String(TestVolumeName),
			},
			{
				Name:  aws.String(apiresource.FileSystemKey),
				Value: aws.String(TestFileSystem),
			},
			{
				Name:  aws.String(apiresource.DeviceNameKey),
				Value: aws.String(TestDeviceName),
			},
		},
	}

	_, err := ParseEBSTaskVolumeAttachment(attachment)
	assert.Error(t, err)
}

func TestParseEBSTaskVolumeAttachmentUnknownProperty(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentArn:  aws.String(testAttachmentArn),
		AttachmentType: aws.String(apiresource.EBSTaskAttach),
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{
				Name:  aws.String(apiresource.VolumeIdKey),
				Value: aws.String(TestVolumeId),
			},
			{
				Name:  aws.String(apiresource.VolumeSizeGibKey),
				Value: aws.String(TestVolumeSizeGib),
			},
			{
				Name:  aws.String(apiresource.SourceVolumeHostPathKey),
				Value: aws.String(TestSourceVolumeHostPath),
			},
			{
				Name:  aws.String(apiresource.VolumeNameKey),
				Value: aws.String(TestVolumeName),
			},
			{
				Name:  aws.String(apiresource.FileSystemKey),
				Value: aws.String(TestFileSystem),
			},
			{
				Name:  aws.String(apiresource.DeviceNameKey),
				Value: aws.String(TestDeviceName),
			},
			{
				Name:  aws.String("someProperty"),
				Value: aws.String("foo"),
			},
		},
	}

	expectedEBSCfg := &EBSTaskVolumeConfig{
		VolumeId:             "vol-12345",
		VolumeName:           "test-volume",
		VolumeSizeGib:        "10",
		SourceVolumeHostPath: "taskarn_vol-12345",
		DeviceName:           "/dev/nvme1n1",
		FileSystem:           "ext4",
	}

	parsedEBSCfg, err := ParseEBSTaskVolumeAttachment(attachment)
	assert.NoError(t, err, "unable to parse attachment")
	assert.Equal(t, expectedEBSCfg, parsedEBSCfg)
}

func TestParseEBSTaskVolumeAttachmentMissingProperty(t *testing.T) {
	// The following attachment will be missing the SourceVolumeHostPath property
	attachment := &ecsacs.Attachment{
		AttachmentArn:  aws.String(testAttachmentArn),
		AttachmentType: aws.String(apiresource.EBSTaskAttach),
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{
				Name:  aws.String(apiresource.VolumeIdKey),
				Value: aws.String(TestVolumeId),
			},
			{
				Name:  aws.String(apiresource.VolumeSizeGibKey),
				Value: aws.String(TestVolumeSizeGib),
			},
			{
				Name:  aws.String(apiresource.VolumeNameKey),
				Value: aws.String(TestVolumeName),
			},
			{
				Name:  aws.String(apiresource.FileSystemKey),
				Value: aws.String(TestFileSystem),
			},
			{
				Name:  aws.String(apiresource.DeviceNameKey),
				Value: aws.String(TestDeviceName),
			},
		},
	}
	_, err := ParseEBSTaskVolumeAttachment(attachment)
	assert.Error(t, err)
}
