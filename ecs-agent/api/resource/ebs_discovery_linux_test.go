//go:build linux && unit
// +build linux,unit

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVolumeID   = "vol-1234"
	testDeviceName = "nvme0n1"
)

func TestParseLsblkOutput(t *testing.T) {
	blockDevice := BlockDevice{
		Name:     testDeviceName,
		Serial:   testVolumeID,
		Children: make([]*BlockDevice, 0),
	}

	lsblkOutput := &LsblkOutput{
		BlockDevices: []BlockDevice{
			blockDevice,
		},
	}

	actualVolumeId, err := parseLsblkOutput(lsblkOutput, "/dev/"+testDeviceName)
	require.NoError(t, err)
	assert.Equal(t, testVolumeID, actualVolumeId)
}

func TestParseLsblkOutputError(t *testing.T) {
	blockDevice := BlockDevice{
		Name:     "nvme1n1",
		Serial:   testVolumeID,
		Children: make([]*BlockDevice, 0),
	}

	lsblkOutput := &LsblkOutput{
		BlockDevices: []BlockDevice{
			blockDevice,
		},
	}
	actualVolumeId, err := parseLsblkOutput(lsblkOutput, testDeviceName)
	require.Error(t, err, "cannot find the device name: %v", "/dev/"+testDeviceName)
	assert.Equal(t, "", actualVolumeId)
}
