//go:build linux
// +build linux

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
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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

// ConfirmEBSVolumeIsAttached is used to scan for an EBS volume that's on the host with a specific device name and/or volume ID.
// There are two cases:
// 1. On nitro-based instance we check both device name and volume ID.
// 2. On xen-based instance we only check by the device name.
func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error {
	var lsblkOut LsblkOutput
	ctxWithTimeout, cancel := context.WithTimeout(api.ctx, ebsVolumeDiscoveryTimeout)
	defer cancel()

	// The lsblk command will output the name and volume ID of all block devices on the host in JSON format
	output, err := exec.CommandContext(ctxWithTimeout, "lsblk", "-o", "NAME,SERIAL", "-J").CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%w; failed to run lsblk %v", err, string(output))
		return err
	}
	err = json.Unmarshal(output, &lsblkOut)
	if err != nil {
		err = fmt.Errorf("%w; failed to unmarshal string: %v", err, string(output))
		return err
	}

	actualVolumeId, err := parseLsblkOutput(&lsblkOut, deviceName)
	if err != nil {
		return err
	}
	expectedVolumeId := strings.ReplaceAll(volumeID, "-", "")

	// On Xen-based instances, the volume ID can't be obtained and so we don't need to check by volume ID.
	if actualVolumeId != "" && expectedVolumeId != actualVolumeId {
		err = fmt.Errorf("%w; expected EBS volume %v but found %v", ErrInvalidVolumeID, volumeID, actualVolumeId)
		return err
	}

	return nil
}

// parseLsblkOutput will parse the `lsblk` output and search for a EBS volume with a specific device name.
// Once found we return the volume ID, otherwise we return an empty string along with an error
// The output of the "lsblk -o NAME,SERIAL -J" command looks like the following:
//
//	{
//		"blockdevices": [
//		   {"name": "nvme0n1", "serial": "vol087768edff8511a23",
//			  "children": [
//				 {"name": "nvme0n1p1", "serial": null},
//				 {"name": "nvme0n1p128", "serial": null}
//			  ]
//		   }
//		]
//	 }
func parseLsblkOutput(output *LsblkOutput, deviceName string) (string, error) {
	actualDeviceName := deviceName[strings.LastIndex(deviceName, "/")+1:]
	for _, block := range output.BlockDevices {
		if block.Name == actualDeviceName {
			return block.Serial, nil
		}
	}
	return "", fmt.Errorf("cannot find EBS volume with device name: %v", actualDeviceName)
}
