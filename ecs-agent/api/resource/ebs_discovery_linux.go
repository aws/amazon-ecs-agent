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
	"os/exec"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/pkg/errors"
)

type LsblkOutput struct {
	BlockDevies []BD `json:"blockdevices"`
}
type BD struct {
	Name   string `json:"name"`
	Serial string `json:"serial"`
	// Children []BDChild `json:"children"`
}

// type BDChild struct {
// 	Name   string `json:"name"`
// 	Serial string `json:"serial"`
// }

func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error {
	var lsblkOut LsblkOutput
	ctxWithTimeout, cancel := context.WithTimeout(api.ctx, ebsnvmeIDTimeoutDuration)
	defer cancel()
	output, err := exec.CommandContext(ctxWithTimeout, "lsblk", "-o", "NAME,SERIAL", "-J").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run lsblk: %s", string(output))
	}
	log.Infof("lsblk output: %v", string(output))
	err = json.Unmarshal(output, &lsblkOut)
	if err != nil {
		return errors.Wrapf(err, "Failed to unmarshal string: %s", string(output))
	}
	// log.Infof("Struct: %v", lsblkOut)
	// actualVolumeID, err := parseEBSNVMeIDOutput(output)
	// if err != nil {
	// 	return err
	// }

	actualVolumeID, err := parseLsblkOutput(&lsblkOut, deviceName)

	// actualVolumeID, err := parseLsblkOutput(output, deviceName)
	if err != nil {
		return err
	}
	vid := strings.ReplaceAll(volumeID, "-", "")
	if vid != actualVolumeID {
		return errors.Wrapf(ErrInvalidVolumeID, "expected EBS volume %s but found %s", volumeID, actualVolumeID)
	}

	return nil
}

func parseLsblkOutput(output *LsblkOutput, deviceName string) (string, error) {
	log.Infof("Output: %v", output)
	actualDeviceName := deviceName[strings.LastIndex(deviceName, "/")+1:]
	for _, block := range output.BlockDevies {
		if block.Name == actualDeviceName {
			return block.Serial, nil
		}
	}
	return "", errors.New("cannot find the EBS volume with device name: %v " + actualDeviceName)
}

// func parseLsblkOutput(out []byte, deviceName string) (string, error) {
// 	// The output of the "lsblk -o +SERIAL" command looks like the following:
// 	// NAME          MAJ:MIN RM SIZE RO TYPE MOUNTPOINT SERIAL
// 	// nvme0n1       259:0    0  30G  0 disk            vol123
// 	// ├─nvme0n1p1   259:1    0  30G  0 part /
// 	// └─nvme0n1p128 259:2    0   1M  0 part

// 	actualDeviceName := deviceName[strings.LastIndex(deviceName, "/")+1:]

// 	// will be looping in small indices (there is a limit of EBS voluems that can be attached to so this will be negligible)
// 	for _, line := range strings.Split(string(out), "\n") {
// 		volumeInfo := strings.Fields((line))
// 		// Example of a EBS volume [nvme0n1 259:0 0 30G 0 disk vol087768edff8511a23]
// 		// We can hard code it to be of size 7 since this
// 		// fmt.Println(volumeID)
// 		if len(volumeInfo) == 0 {
// 			continue
// 		}
// 		volumeId := volumeInfo[len(volumeInfo)-1]
// 		if volumeInfo[0] == actualDeviceName && strings.HasPrefix(volumeId, "vol") {
// 			volumeId = volumeId[:3] + "-" + volumeId[3:]
// 			return volumeId, nil
// 		}
// 	}
// 	return "", errors.New("cannot find the EBS volume with device name: %v " + deviceName)
// }

// func parseEBSNVMeIDOutput(output []byte) (string, error) {
// 	// The output of the "ebsnvme-id -v /dev/xvda" command looks like the following:
// 	// Volume ID: vol-0a5620f3403272844
// 	out := string(output)
// 	volumeInfo := strings.Fields(out)
// 	if len(volumeInfo) != 3 {
// 		return "", errors.New("cannot find the volume ID: " + out)
// 	}
// 	return volumeInfo[2], nil
// }