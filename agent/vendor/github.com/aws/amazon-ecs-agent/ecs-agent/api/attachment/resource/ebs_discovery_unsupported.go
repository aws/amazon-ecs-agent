//go:build !linux && !windows
// +build !linux,!windows

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
	"errors"
)

const (
	sdPrefix  = ""
	xvdPrefix = ""
)

// LsblkOutput is used to manage and track the output of `lsblk`
type LsblkOutput struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}
type BlockDevice struct {
	Name     string         `json:"name"`
	Serial   string         `json:"serial"`
	Children []*BlockDevice `json:"children,omitempty"`
}

func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) (string, error) {
	return "", errors.New("unsupported platform")
}

func parseLsblkOutput(
	output *LsblkOutput,
	deviceName, volumeId string,
	hasXenSupport bool,
) (string, error) {
	return "", errors.New("unsupported platform")
}

// Matches a block device against a device name by matching the block device's name and
// the device name after stripping "sd" or "xvd" prefixes (whichever is present) from both names
// if it is present in both names.
func matchXenBlockDevice(block BlockDevice, deviceName string) (string, bool) {
	return "", false
}

// Checks if the device name has sd or xvd prefix.
func hasXenPrefix(name string) bool {
	return false
}

// Trims "sd" or "xvd" prefix (whichever is present) from the given name.
func trimXenPrefix(name string) string {
	return ""
}
