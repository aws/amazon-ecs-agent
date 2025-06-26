//go:build linux
// +build linux

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cihub/seelog"
)

const (
	unsupportedOSFamily = "linux"
	OSFamilyEnvVar      = "ECS_OS_FAMILY"
	osReleaseFilePath   = "/etc/os-release"
)

var execCommand = exec.Command

func GetLinuxOSFamily() string {
	cmd := execCommand("cat", osReleaseFilePath)
	output, err := cmd.Output()
	if err != nil {
		seelog.Errorf("failed to execute 'cat %s' command: %v", osReleaseFilePath, err)
		return unsupportedOSFamily
	}

	var id, versionID string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id = parseOSReleaseValue(strings.TrimPrefix(line, "ID="))
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			versionID = parseOSReleaseValue(strings.TrimPrefix(line, "VERSION_ID="))
		}
	}

	if err := scanner.Err(); err != nil {
		seelog.Errorf("error scanning output from 'cat %s': %v", osReleaseFilePath, err)
		return unsupportedOSFamily
	}

	if id == "" {
		seelog.Error("could not determine OS ID from /etc/os-release")
		return unsupportedOSFamily
	}

	if versionID == "" {
		seelog.Error("could not determine VERSION_ID from /etc/os-release")
		return unsupportedOSFamily
	}

	osFamily := fmt.Sprintf("%s_%s", id, versionID)

	if err := os.Setenv(OSFamilyEnvVar, osFamily); err != nil {
		seelog.Errorf("failed to set environment variable %s: %v", OSFamilyEnvVar, err)
		return unsupportedOSFamily
	}

	seelog.Debugf("operating system family is: %s", osFamily)
	return osFamily
}

// parseOSReleaseValue parses a value from /etc/os-release by trimming whitespace and removing quotation marks
func parseOSReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"'`)
}
