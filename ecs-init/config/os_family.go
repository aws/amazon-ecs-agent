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
	"io"
	"os"
	"strings"

	"github.com/cihub/seelog"
)

const (
	unsupportedOSFamily = "linux"
	osReleaseFilePath   = "/etc/os-release"
)

func GetLinuxOSFamily() string {
	file, err := os.Open(osReleaseFilePath)
	if err != nil {
		seelog.Errorf("failed to read file %s: %v", osReleaseFilePath, err)
		return unsupportedOSFamily
	}
	defer file.Close()

	return getLinuxOSFamilyFromReader(file)
}

func getLinuxOSFamilyFromReader(reader io.Reader) string {
	id, versionID, err := parseOSReleaseFromReader(reader)
	if err != nil {
		seelog.Errorf("error parsing os-release content: %v", err)
		return unsupportedOSFamily
	}

	if id == "" {
		seelog.Error("could not determine OS ID from os-release")
		return unsupportedOSFamily
	}

	if versionID == "" {
		seelog.Error("could not determine VERSION_ID from os-release")
		return unsupportedOSFamily
	}

	osFamily := fmt.Sprintf("%s_%s", id, versionID)
	seelog.Infof("Operating system family is: %s", osFamily)
	return osFamily
}

// parseOSReleaseFromReader parses os-release content from an io.Reader and extracts ID and VERSION_ID
// See https://man7.org/linux/man-pages/man5/os-release.5.html for information about the os-release file format
func parseOSReleaseFromReader(reader io.Reader) (string, string, error) {
	var id, versionID string
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id = parseOSReleaseValue(strings.TrimPrefix(line, "ID="))
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			versionID = parseOSReleaseValue(strings.TrimPrefix(line, "VERSION_ID="))
		}
	}

	return id, versionID, scanner.Err()
}

// parseOSReleaseValue parses a value from /etc/os-release by trimming whitespace and removing quotation marks
func parseOSReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"'`)
}
