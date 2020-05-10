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
	"strconv"

	"github.com/aws/amazon-ecs-agent/agent/config"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/cihub/seelog"
)

const (
	// ECSVolumePlugin is the driver name of the ECS volume plugin.
	ECSVolumePlugin = "amazon-ecs-volume-plugin"
	// DockerLocalDriverName is the name of Docker's local volume driver.
	DockerLocalDriverName = "local"

	efsVolumePluginDriverType = "efs"
	efsLocalDriverType        = "nfs"

	// Capability injected by the ecs volume plugin via ECS_VOLUME_PLUGIN_CAPABILITIES.
	efsVolumePluginCapability = "efsAuth"

	// Enums used by acs's api-2.json model.
	efsIAMAuthEnabled           = "ENABLED"
	efsTransitEncryptionEnabled = "ENABLED"
)

// EFSVolumeConfig represents efs volume configuration.
type EFSVolumeConfig struct {
	AuthConfig            EFSAuthConfig `json:"authorizationConfig,omitempty"`
	FileSystemID          string        `json:"fileSystemId,omitempty"`
	RootDirectory         string        `json:"rootDirectory,omitempty"`
	TransitEncryption     string        `json:"transitEncryption,omitempty"`
	TransitEncryptionPort int64         `json:"transitEncryptionPort,omitempty"`
	// DockerVolumeName is internal docker name for this volume.
	DockerVolumeName string `json:"dockerVolumeName"`
}

// EFSAuthConfig contains auth config for an efs volume.
type EFSAuthConfig struct {
	AccessPointId string `json:"accessPointId,omitempty"`
	Iam           string `json:"iam,omitempty"`
}

// GetDriverOptions returns the driver options for creating an EFS volume.
func GetDriverOptions(cfg *config.Config, efsVolCfg *EFSVolumeConfig, credsRelativeURI string) map[string]string {
	if UseECSVolumePlugin(cfg) {
		return efsVolCfg.getVolumePluginDriverOptions(credsRelativeURI)
	}
	return efsVolCfg.getDockerLocalDriverOptions(cfg)
}

// UseECSVolumePlugin checks whether we should use the ECS volume plugin.
func UseECSVolumePlugin(cfg *config.Config) bool {
	for _, cap := range cfg.VolumePluginCapabilities {
		if cap == efsVolumePluginCapability {
			return true
		}
	}
	return false
}

// GetDockerLocalDriverOptions returns the options for creating an EFS volume using Docker's local volume driver.
func (efsVolCfg *EFSVolumeConfig) getDockerLocalDriverOptions(cfg *config.Config) map[string]string {
	domain := getDomainForPartition(cfg.AWSRegion)
	// These are the NFS options recommended by EFS, see:
	// https://docs.aws.amazon.com/efs/latest/ug/mounting-fs-mount-cmd-general.html
	ostr := fmt.Sprintf("addr=%s.efs.%s.%s,nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport", efsVolCfg.FileSystemID, cfg.AWSRegion, domain)
	devstr := fmt.Sprintf(":%s", efsVolCfg.RootDirectory)
	return map[string]string{
		"type":   efsLocalDriverType,
		"device": devstr,
		"o":      ostr,
	}
}

// GetEFSDriverOptions returns the options for creating an EFS volume using the ECS volume plugin.
func (efsVolCfg *EFSVolumeConfig) getVolumePluginDriverOptions(credsRelativeURI string) map[string]string {
	device := efsVolCfg.FileSystemID
	if efsVolCfg.RootDirectory != "" {
		device = fmt.Sprintf("%s:%s", device, efsVolCfg.RootDirectory)
	}

	mntOpt := NewVolumeMountOptions()
	if efsVolCfg.TransitEncryption == efsTransitEncryptionEnabled {
		mntOpt.AddOption("tls", "")
	}
	if efsVolCfg.TransitEncryptionPort != 0 {
		mntOpt.AddOption("tlsport", strconv.Itoa(int(efsVolCfg.TransitEncryptionPort)))
	}
	if efsVolCfg.AuthConfig.Iam == efsIAMAuthEnabled {
		mntOpt.AddOption("iam", "")
		mntOpt.AddOption("awscredsuri", credsRelativeURI)
	}
	if efsVolCfg.AuthConfig.AccessPointId != "" {
		mntOpt.AddOption("accesspoint", efsVolCfg.AuthConfig.AccessPointId)
	}
	options := map[string]string{
		"type":   efsVolumePluginDriverType,
		"device": device,
		"o":      mntOpt.String(),
	}
	return options
}

func getDomainForPartition(region string) string {
	partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region)
	if !ok {
		seelog.Warnf("No partition resolved for region (%s). Using AWS default (%s)", region, endpoints.AwsPartition().DNSSuffix())
		return endpoints.AwsPartition().DNSSuffix()
	}
	return partition.DNSSuffix()
}
