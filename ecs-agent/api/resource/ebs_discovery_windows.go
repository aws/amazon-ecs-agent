//go:build windows
// +build windows

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
	"fmt"
	"os/exec"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	volumeDiscoveryTimeoutDuration = 5 * time.Second
)

func (api EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) error {
	ctxWithTimeout, cancel := context.WithTimeout(api.ctx, volumeDiscoveryTimeoutDuration)
	defer cancel()
	output, err := exec.CommandContext(ctxWithTimeout,
		"C:\\PROGRAMDATA\\Amazon\\Tools\\ebsnvme-id.exe").CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to run ebsnvme-id.exe: %s", string(output))
	}

	_, err = parseExecutableOutput(output, volumeID, deviceName)
	if err != nil {
		return errors.Wrapf(err, "failed to parse ebsnvme-id.exe output for volumeID: %s and deviceName: %s",
			volumeID, deviceName)
	}

	log.Info(fmt.Sprintf("found volume with volumeID: %s and deviceName: %s", volumeID, deviceName))

	return nil
}

// parseExecutableOutput parses the output of `ebsnvme-id.exe` and returns the volumeId.
func parseExecutableOutput(output []byte, candidateVolumeId string, candidateDeviceName string) (string, error) {
	/* The output of the ebsnvme-id.exe is emitted like the following:
	Disk Number: 0
	Volume ID: vol-0a1234f340444abcd
	Device Name: sda1

	Disk Number: 1
	Volume ID: vol-abcdef1234567890a
	Device Name: /dev/sdf */

	out := string(output)
	volumeInfo := strings.Fields(out)

	volumeInfoLength := 9
	volumeIdOffset := 5
	deviceNameOffset := 8

	if len(volumeInfo) < volumeInfoLength {
		return "", errors.New("cannot find the volume ID. Encountered error message: " + out)
	}

	for volumeIndex := 0; volumeIndex <= len(volumeInfo)-volumeInfoLength; volumeIndex = volumeIndex + volumeInfoLength {
		volumeId := volumeInfo[volumeIndex+volumeIdOffset]
		deviceName := volumeInfo[volumeIndex+deviceNameOffset]
		if volumeId == candidateVolumeId && deviceName == candidateDeviceName {
			return volumeId, nil
		}
	}

	return "", errors.New("cannot find the volume ID:" + candidateVolumeId)
}
