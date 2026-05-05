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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseS3FilesTaskVolumeAttachment_AllProperties(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{Name: aws.String("volumeId"), Value: aws.String("fs-123")},
			{Name: aws.String("volumeName"), Value: aws.String("myVolume")},
			{Name: aws.String("s3FilesRootDirectory"), Value: aws.String("/data")},
			{Name: aws.String("s3FilesTransitEncryptionPort"), Value: aws.String("2049")},
			{Name: aws.String("s3FilesAccessPointId"), Value: aws.String("fsap-123")},
		},
	}

	cfg, err := ParseS3FilesTaskVolumeAttachment(attachment)
	require.NoError(t, err)
	assert.Equal(t, "fs-123", cfg.VolumeId)
	assert.Equal(t, "myVolume", cfg.VolumeName)
	assert.Equal(t, "/data", cfg.S3FilesRootDirectory)
	assert.Equal(t, "2049", cfg.S3FilesTransitEncryptionPort)
	assert.Equal(t, "fsap-123", cfg.S3FilesAccessPointId)
}

func TestParseS3FilesTaskVolumeAttachment_RequiredOnly(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{Name: aws.String("volumeId"), Value: aws.String("fs-456")},
			{Name: aws.String("volumeName"), Value: aws.String("vol2")},
		},
	}

	cfg, err := ParseS3FilesTaskVolumeAttachment(attachment)
	require.NoError(t, err)
	assert.Equal(t, "fs-456", cfg.VolumeId)
	assert.Equal(t, "vol2", cfg.VolumeName)
	assert.Empty(t, cfg.S3FilesRootDirectory)
	assert.Empty(t, cfg.S3FilesTransitEncryptionPort)
	assert.Empty(t, cfg.S3FilesAccessPointId)
}

func TestParseS3FilesTaskVolumeAttachment_NilProperty(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{Name: aws.String("volumeId"), Value: aws.String("fs-123")},
			nil,
			{Name: aws.String("volumeName"), Value: aws.String("myVolume")},
		},
	}

	_, err := ParseS3FilesTaskVolumeAttachment(attachment)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil property")
}

func TestParseS3FilesTaskVolumeAttachment_EmptyValue(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{Name: aws.String("volumeId"), Value: aws.String("")},
			{Name: aws.String("volumeName"), Value: aws.String("myVolume")},
		},
	}

	_, err := ParseS3FilesTaskVolumeAttachment(attachment)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty value")
}

func TestParseS3FilesTaskVolumeAttachment_UnrecognizedProperty(t *testing.T) {
	attachment := &ecsacs.Attachment{
		AttachmentProperties: []*ecsacs.AttachmentProperty{
			{Name: aws.String("volumeId"), Value: aws.String("fs-789")},
			{Name: aws.String("volumeName"), Value: aws.String("vol3")},
			{Name: aws.String("unknownProperty"), Value: aws.String("someValue")},
		},
	}

	cfg, err := ParseS3FilesTaskVolumeAttachment(attachment)
	require.NoError(t, err)
	assert.Equal(t, "fs-789", cfg.VolumeId)
	assert.Equal(t, "vol3", cfg.VolumeName)
}

func TestValidateS3FilesAttachment_MissingVolumeId(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		VolumeId:   "",
		VolumeName: "myVolume",
	}
	err := validateS3FilesAttachment(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volumeId")
}

func TestValidateS3FilesAttachment_MissingVolumeName(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		VolumeId:   "fs-123",
		VolumeName: "",
	}
	err := validateS3FilesAttachment(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "volumeName")
}

func TestS3FilesVolumeConfig_Source(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		DockerVolumeName: "test-vol",
	}
	assert.Equal(t, "test-vol", cfg.Source())
}

func TestS3FilesVolumeConfig_GetType(t *testing.T) {
	cfg := &S3FilesVolumeConfig{}
	assert.Equal(t, "s3files", cfg.GetType())
}

func TestGetVolumePluginDriverOptions_Basic(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		VolumeId:   "fs-123",
		VolumeName: "vol1",
	}
	options := cfg.GetVolumePluginDriverOptions("/creds")
	assert.Equal(t, "s3files", options["type"])
	assert.Equal(t, "fs-123", options["device"])
	assert.Contains(t, options["o"], "tls")
	assert.Contains(t, options["o"], "iam")
	assert.Contains(t, options["o"], "awscredsuri=/creds")
}

func TestGetVolumePluginDriverOptions_WithAccessPoint(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		VolumeId:             "fs-123",
		VolumeName:           "vol1",
		S3FilesAccessPointId: "fsap-123",
	}
	options := cfg.GetVolumePluginDriverOptions("/creds")
	assert.Contains(t, options["o"], "accesspoint=fsap-123")
}

func TestGetVolumePluginDriverOptions_WithRootDirectory(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		VolumeId:             "fs-123",
		VolumeName:           "vol1",
		S3FilesRootDirectory: "/data",
	}
	options := cfg.GetVolumePluginDriverOptions("/creds")
	assert.Equal(t, "fs-123:/data", options["device"])
}

func TestGetVolumePluginDriverOptions_WithTLSPort(t *testing.T) {
	cfg := &S3FilesVolumeConfig{
		VolumeId:                     "fs-123",
		VolumeName:                   "vol1",
		S3FilesTransitEncryptionPort: "2049",
	}
	options := cfg.GetVolumePluginDriverOptions("/creds")
	assert.Contains(t, options["o"], "tlsport=2049")
}
