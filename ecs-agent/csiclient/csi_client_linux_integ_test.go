//go:build linux && integration
// +build linux,integration

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
package csiclient

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/api/attachment/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timeoutDuration = 1 * time.Second

// Tests that CSIClient can fetch EBS volume metrics from EBS CSI Driver.
//
// This test assumes that there is only one EBS Volume attached to the host that's mounted
// on "/" path. It also assumes that an EBS CSI Driver is running and listening on
// /tmp/ebs-csi-driver.sock socket file.
//
// A typical environment for this test is an EC2 instance with a single EBS volume as its
// root device volume.
func TestNodeVolumeStats(t *testing.T) {
	skipForUnsupportedLsblk(t)

	// Find the root volume using lsblk
	lsblkCtx, lsblkCtxCancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer lsblkCtxCancel()
	output, err := exec.CommandContext(lsblkCtx, "lsblk", "-o", "NAME,SERIAL", "-J").CombinedOutput()
	require.NoError(t, err)
	var lsblkOut resource.LsblkOutput
	err = json.Unmarshal(output, &lsblkOut)
	require.NoError(t, err)
	require.True(t, len(lsblkOut.BlockDevices) > 0)

	volumeID := lsblkOut.BlockDevices[0].Serial
	if volumeID == "" {
		// Driver doesn't actually care about the volume ID as long as it's not empty
		volumeID = "unknown"
	}

	// Get metrics for the root volume from EBS CSI Driver.
	csiClient := NewCSIClient("/tmp/ebs-csi-driver.sock")
	getVolumeCtx, getVolumeCtxCancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer getVolumeCtxCancel()
	metrics, err := csiClient.GetVolumeMetrics(getVolumeCtx, volumeID, "/")
	require.NoError(t, err)
	assert.True(t, metrics.Capacity > 0)
	assert.True(t, metrics.Used > 0)
}

// Skips the test if lsblk version is older than 2.27 as those versions
// don't support JSON output format. Agent itself depends on lsblk >= 2.27
// for the same reason.
func skipForUnsupportedLsblk(t *testing.T) {
	const (
		requiredMajorVersion = 2
		requiredMinorVersion = 27
	)

	lsblkVerCtx, lsblkVerCtxCancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer lsblkVerCtxCancel()
	output, err := exec.CommandContext(lsblkVerCtx, "bash", "-c", "lsblk --version | awk '{print $NF}'").CombinedOutput()
	require.NoError(t, err)

	lsblkVersion := strings.Split(string(output), ".")
	require.Len(t, lsblkVersion, 3, "Failed to parse lsblk version from %s", string(output))
	majorVer, err := strconv.Atoi(lsblkVersion[0])
	require.NoError(t, err)
	minorVer, err := strconv.Atoi(lsblkVersion[1])
	require.NoError(t, err)

	if majorVer < requiredMajorVersion || majorVer == requiredMajorVersion && minorVer < requiredMinorVersion {
		t.Skipf("Need lsblk version >= %d.%d but found %s",
			requiredMajorVersion, requiredMinorVersion, string(output))
	}
}
