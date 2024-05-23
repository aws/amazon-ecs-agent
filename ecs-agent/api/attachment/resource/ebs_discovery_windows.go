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

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	diskNumberOffset = 0
	volumeIdOffset   = 1
	deviceNameOffset = 2
	volumeInfoLength = 3
)

func (api *EBSDiscoveryClient) ConfirmEBSVolumeIsAttached(deviceName, volumeID string) (string, error) {
	ctxWithTimeout, cancel := context.WithTimeout(api.ctx, ebsVolumeDiscoveryTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctxWithTimeout,
		"C:\\PROGRAMDATA\\Amazon\\Tools\\ebsnvme-id.exe").CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run ebsnvme-id.exe: %s", string(output))
	}

	actualDeviceName, err := parseExecutableOutput(output, volumeID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse ebsnvme-id.exe output for volumeID: %s and deviceName: %s",
			volumeID, deviceName)
	}

	log.Info(fmt.Sprintf("found volume with volumeID: %s and deviceName: %s", volumeID, actualDeviceName))

	return actualDeviceName, nil
}

// parseExecutableOutput parses the output of `ebsnvme-id.exe` and returns the deviceName.
func parseExecutableOutput(output []byte, candidateVolumeId string) (string, error) {
	/* The output of the ebsnvme-id.exe is emitted like the following:
	Disk Number: 0
	Volume ID: vol-0a1234f340444abcd
	Device Name: sda1

	Disk Number: 1
	Volume ID: vol-abcdef1234567890a
	Device Name: /dev/sdf */

	out := string(output)
	// Replace double line with a single line and split based on single line
	volumeInfo := strings.Split(strings.Replace(string(out), "\r\n\r\n", "\r\n", -1), "\r\n")

	if len(volumeInfo) < volumeInfoLength {
		return "", errors.New("cannot find the volume ID. Encountered error message: " + out)
	}

	//Read every 3 lines of disk information
	for volumeIndex := 0; volumeIndex <= len(volumeInfo)-volumeInfoLength; volumeIndex = volumeIndex + volumeInfoLength {
		_, volumeId, deviceName, err := parseSet(volumeInfo[volumeIndex : volumeIndex+volumeInfoLength])
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse the output for volumeID: %s. "+
				"Output:%s", candidateVolumeId, out)
		}

		if volumeId == candidateVolumeId {
			return deviceName, nil
		}

	}

	return "", errors.New("cannot find the volume ID:" + candidateVolumeId)
}

// parseSet parses the single volume information that is 3 lines long
func parseSet(lines []string) (string, string, string, error) {
	if len(lines) != 3 {
		return "", "", "", errors.New("the number of entries in the volume information is insufficient to parse. Expected 3 lines")
	}

	diskNumber, err := parseValue(lines[diskNumberOffset], "Disk Number:")
	if err != nil {
		return "", "", "", err
	}
	volumeId, err := parseValue(lines[volumeIdOffset], "Volume ID:")
	if err != nil {
		return "", "", "", err
	}
	deviceName, err := parseValue(lines[deviceNameOffset], "Device Name:")
	if err != nil {
		return "", "", "", err
	}
	return diskNumber, volumeId, deviceName, nil
}

// parseValue looks for the volume information identifier and replaces it to return just the value
func parseValue(inputBuffer string, stringToTrim string) (string, error) {
	// if the input buffer doesn't have the identifier for the information, return an error
	if !strings.Contains(inputBuffer, stringToTrim) {
		return "", errors.New("output buffer was missing the string:" + stringToTrim)
	}

	return strings.TrimSpace(strings.Replace(inputBuffer, stringToTrim, "", -1)), nil
}
