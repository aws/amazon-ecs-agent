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

	"github.com/pkg/errors"
)

type LsblkOutput struct {
	BlockDevies []BD `json:"blockdevices"`
}
type BD struct {
	Name     string    `json:"name"`
	Serial   string    `json:"serial"`
	Children []BDChild `json:"children"`
}
type BDChild struct {
	Name   string `json:"name"`
	Serial string `json:"serial"`
}

func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error {
	var lsblkOut LsblkOutput
	ctxWithTimeout, cancel := context.WithTimeout(api.ctx, ebsnvmeIDTimeoutDuration)
	defer cancel()
	output, err := exec.CommandContext(ctxWithTimeout, "lsblk", "-o", "NAME,SERIAL", "-J").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run lsblk: %s", string(output))
	}
	err = json.Unmarshal(output, &lsblkOut)
	if err != nil {
		return errors.Wrapf(err, "Failed to unmarshal string: %s", string(output))
	}

	actualVolumeId, err := parseLsblkOutput(&lsblkOut, deviceName)
	if err != nil {
		return err
	}
	expectedVolumeId := strings.ReplaceAll(volumeID, "-", "")
	if expectedVolumeId != actualVolumeId {
		return errors.Wrapf(ErrInvalidVolumeID, "expected EBS volume %s but found %s", volumeID, actualVolumeId)
	}

	return nil
}

func parseLsblkOutput(output *LsblkOutput, deviceName string) (string, error) {
	actualDeviceName := deviceName[strings.LastIndex(deviceName, "/")+1:]
	for _, block := range output.BlockDevies {
		if block.Name == actualDeviceName {
			return block.Serial, nil
		}
	}
	return "", errors.New("cannot find the device name: " + actualDeviceName)
}
