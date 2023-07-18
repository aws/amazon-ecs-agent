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

	"github.com/aws/amazon-ecs-agent/agent/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestEFSVolCfg() *EFSVolumeConfig {
	return &EFSVolumeConfig{
		AuthConfig: EFSAuthConfig{
			AccessPointId: "test-ap",
			Iam:           "ENABLED",
		},
		FileSystemID:          "fs-123",
		RootDirectory:         "/test",
		TransitEncryption:     "ENABLED",
		TransitEncryptionPort: int64(123),
		DockerVolumeName:      "test-vol",
	}
}

func TestGetEFSDriverOptions_UseLocalVolumeDriver(t *testing.T) {
	cfg := &config.Config{
		AWSRegion: "us-west-1",
	}
	efsVolCfg := getTestEFSVolCfg()
	credsURI := "/v2/creds-id"
	options := GetDriverOptions(cfg, efsVolCfg, credsURI)
	assert.Equal(t, "nfs", options["type"])
	assert.Equal(t, "addr=fs-123.efs.us-west-1.amazonaws.com,nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport", options["o"])
	assert.Equal(t, ":/test", options["device"])
}

func TestGetEFSDriverOptions_UseECSVolumePlugin(t *testing.T) {
	cfg := &config.Config{
		VolumePluginCapabilities: []string{"efsAuth"},
	}
	efsVolCfg := getTestEFSVolCfg()
	credsURI := "/v2/creds-id"
	options := GetDriverOptions(cfg, efsVolCfg, credsURI)
	assert.Equal(t, "efs", options["type"])
	assert.Equal(t, "tls,tlsport=123,iam,awscredsuri=/v2/creds-id,accesspoint=test-ap", options["o"])
	assert.Equal(t, "fs-123:/test", options["device"])
}

func TestUseVolumePlugin(t *testing.T) {
	cfg := &config.Config{
		VolumePluginCapabilities: []string{"efsAuth"},
	}
	assert.True(t, UseECSVolumePlugin(cfg))
	assert.False(t, UseECSVolumePlugin(&config.Config{}))
}

func TestGetDockerLocalDriverOptions(t *testing.T) {
	efsVolCfg := getTestEFSVolCfg()
	cfg := &config.Config{
		AWSRegion: "us-west-2",
	}
	options := efsVolCfg.getDockerLocalDriverOptions(cfg)
	require.Contains(t, options, "type")
	require.Contains(t, options, "device")
	require.Contains(t, options, "o")
	assert.Equal(t, "nfs", options["type"])
	assert.Equal(t, ":/test", options["device"])
	assert.Equal(t, "addr=fs-123.efs.us-west-2.amazonaws.com,nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport",
		options["o"])
}

func TestGetVolumePluginDriverOptions(t *testing.T) {
	efsVolCfg := getTestEFSVolCfg()
	options := efsVolCfg.getVolumePluginDriverOptions("/v2/abc")
	require.Contains(t, options, "type")
	require.Contains(t, options, "device")
	require.Contains(t, options, "o")
	assert.Equal(t, "efs", options["type"])
	assert.Equal(t, "fs-123:/test", options["device"])
	assert.Equal(t, "tls,tlsport=123,iam,awscredsuri=/v2/abc,accesspoint=test-ap", options["o"])
}
