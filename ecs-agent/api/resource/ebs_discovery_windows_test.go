//go:build windows && unit
// +build windows,unit

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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVolumeID = "vol-0a1234f340444abcd"
	deviceName   = "/dev/sdf"
)

func TestParseExecutableOutputWithHappyPath(t *testing.T) {
	output := fmt.Sprintf("Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nDisk Number: 1\r\nVolume ID: %s\r\nDevice Name: %s\r\n\r\n", testVolumeID, deviceName)
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, deviceName)
	require.NoError(t, err)
	assert.True(t, strings.Contains(parsedOutput, testVolumeID))
}

func TestParseExecutableOutputWithMissingDiskNumber(t *testing.T) {
	output := fmt.Sprintf("Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nVolume ID: %s\r\nDevice Name: %s\r\n\r\n", testVolumeID, deviceName)
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, deviceName)
	require.Error(t, err)
	assert.Equal(t, "", parsedOutput)
}

func TestParseExecutableOutputWithMissingVolumeInformation(t *testing.T) {
	output := fmt.Sprintf("Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nDisk Number: 1\r\nDevice Name: %s\r\n\r\n", deviceName)
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, deviceName)
	require.Error(t, err)
	assert.Equal(t, "", parsedOutput)
}

func TestParseExecutableOutputWithMissingDeviceName(t *testing.T) {
	output := fmt.Sprintf("Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nDisk Number: 1\r\nVolume ID: %s\r\n\r\n", testVolumeID)
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, deviceName)
	require.Error(t, err)
	assert.Equal(t, "", parsedOutput)
}

func TestParseExecutableOutputWithVolumeNameMismatch(t *testing.T) {
	output := fmt.Sprintf("Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nDisk Number: 1\r\nVolume ID: %s\r\nDevice Name: %s\r\n\r\n", testVolumeID, deviceName)
	parsedOutput, err := parseExecutableOutput([]byte(output), "MismatchedVolumeName", deviceName)
	require.Error(t, err)
	assert.Equal(t, "", parsedOutput)
}

func TestParseExecutableOutputWithDeviceNameMismatch(t *testing.T) {
	output := fmt.Sprintf("Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nDisk Number: 1\r\nVolume ID: %s\r\nDevice Name: %s\r\n\r\n", testVolumeID, deviceName)
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, "MismatchedDeviceName")
	require.Error(t, err)
	assert.Equal(t, "", parsedOutput)
}

func TestParseExecutableOutputWithTruncatedOutputBuffer(t *testing.T) {
	output := "Disk Number: 0\r\nVolume ID: vol-abcdef1234567890a\r\nDevice Name: sda1\r\n\r\nDisk Number: 1\r\nVolume ID: TruncatedBuffer..."
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, deviceName)
	require.Error(t, err)
	assert.Equal(t, "", parsedOutput)
}

func TestParseExecutableOutputWithUnexpectedOutput(t *testing.T) {
	output := "No EBS NVMe disks found."
	parsedOutput, err := parseExecutableOutput([]byte(output), testVolumeID, deviceName)
	require.Error(t, err, "cannot find the volume ID: %s", output)
	assert.Equal(t, "", parsedOutput)
}
