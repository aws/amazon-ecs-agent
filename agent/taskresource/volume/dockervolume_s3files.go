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
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	s3filesVolumePluginDriverType = "s3files"
)

// S3FilesVolumeConfig represents S3 Files volume configuration.
type S3FilesVolumeConfig struct {
	VolumeId                     string `json:"volumeId"`
	VolumeName                   string `json:"volumeName"`
	S3FilesRootDirectory         string `json:"s3FilesRootDirectory,omitempty"`
	S3FilesTransitEncryptionPort string `json:"s3FilesTransitEncryptionPort,omitempty"`
	S3FilesAccessPointId         string `json:"s3FilesAccessPointId,omitempty"`
	// DockerVolumeName is internal docker name for this volume.
	DockerVolumeName string `json:"dockerVolumeName"`
}

// Source returns the name of the volume resource which is used as the source of the volume mount.
func (cfg *S3FilesVolumeConfig) Source() string {
	return cfg.DockerVolumeName
}

// GetType returns the S3 Files volume type.
func (cfg *S3FilesVolumeConfig) GetType() string {
	return S3FilesVolumeType
}

// GetVolumeId returns the volume ID.
func (cfg *S3FilesVolumeConfig) GetVolumeId() string {
	return cfg.VolumeId
}

// GetVolumeName returns the volume name.
func (cfg *S3FilesVolumeConfig) GetVolumeName() string {
	return cfg.VolumeName
}

// ParseS3FilesTaskVolumeAttachment parses the S3 Files task volume config
// from the given attachment.
func ParseS3FilesTaskVolumeAttachment(attachment *ecsacs.Attachment) (*S3FilesVolumeConfig, error) {
	cfg := &S3FilesVolumeConfig{}
	for _, property := range attachment.AttachmentProperties {
		if property == nil {
			return nil, fmt.Errorf("failed to parse s3files attachment, encountered nil property")
		}
		if aws.ToString(property.Value) == "" {
			return nil, fmt.Errorf("failed to parse s3files attachment, encountered empty value for property: %s",
				aws.ToString(property.Name))
		}
		switch aws.ToString(property.Name) {
		case apiresource.S3FilesVolumeIdKey:
			cfg.VolumeId = aws.ToString(property.Value)
		case apiresource.S3FilesVolumeNameKey:
			cfg.VolumeName = aws.ToString(property.Value)
		case apiresource.S3FilesRootDirectoryKey:
			cfg.S3FilesRootDirectory = aws.ToString(property.Value)
		case apiresource.S3FilesTransitEncryptionPortKey:
			cfg.S3FilesTransitEncryptionPort = aws.ToString(property.Value)
		case apiresource.S3FilesAccessPointIdKey:
			cfg.S3FilesAccessPointId = aws.ToString(property.Value)
		default:
			logger.Warn("Received an unrecognized s3files attachment property", logger.Fields{
				"attachmentProperty": property.String(),
			})
		}
	}
	if err := validateS3FilesAttachment(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validateS3FilesAttachment validates that required fields are present in the S3 Files config.
func validateS3FilesAttachment(cfg *S3FilesVolumeConfig) error {
	if cfg.VolumeId == "" {
		return fmt.Errorf("missing %s in the S3 Files config", apiresource.S3FilesVolumeIdKey)
	}
	if cfg.VolumeName == "" {
		return fmt.Errorf("missing %s in the S3 Files config", apiresource.S3FilesVolumeNameKey)
	}
	return nil
}

// GetVolumePluginDriverOptions returns the options for creating an S3 Files volume
// using the ECS volume plugin.
func (cfg *S3FilesVolumeConfig) GetVolumePluginDriverOptions(credsRelativeURI string) map[string]string {
	device := cfg.VolumeId
	if cfg.S3FilesRootDirectory != "" {
		device = fmt.Sprintf("%s:%s", device, cfg.S3FilesRootDirectory)
	}

	mntOpt := NewVolumeMountOptions()
	// S3 Files always uses TLS.
	mntOpt.AddOption("tls", "")
	if cfg.S3FilesTransitEncryptionPort != "" {
		mntOpt.AddOption("tlsport", cfg.S3FilesTransitEncryptionPort)
	}
	// S3 Files always uses IAM auth.
	mntOpt.AddOption("iam", "")
	mntOpt.AddOption("awscredsuri", credsRelativeURI)
	if cfg.S3FilesAccessPointId != "" {
		mntOpt.AddOption("accesspoint", cfg.S3FilesAccessPointId)
	}

	return map[string]string{
		"type":   s3filesVolumePluginDriverType,
		"device": device,
		"o":      mntOpt.String(),
	}
}
