//go:build unit
// +build unit

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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	expectedDebianOSFamily = "debian_11"

	// Base OS release content without ID/VERSION_ID lines
	debianOSReleaseBase = `PRETTY_NAME="Debian GNU/Linux 11 (bullseye)"
NAME="Debian GNU/Linux"
VERSION="11 (bullseye)"
VERSION_CODENAME=bullseye
HOME_URL="https://www.debian.org/"
SUPPORT_URL="https://www.debian.org/support"
BUG_REPORT_URL="https://bugs.debian.org/"
`

	// Complete OS release with both ID and VERSION_ID
	debianOSRelease = debianOSReleaseBase + `VERSION_ID="11"
ID=debian
`

	// OS release missing ID line
	debianOSReleaseMissingID = debianOSReleaseBase + `VERSION_ID="11"
`

	// OS release missing VERSION_ID line
	debianOSReleaseMissingVersion = debianOSReleaseBase + `ID=debian
`
)

// TestGetLinuxOSFamilyFromReader tests the complete workflow with valid content
func TestGetLinuxOSFamilyFromReader(t *testing.T) {
	reader := strings.NewReader(debianOSRelease)
	osFamily := getLinuxOSFamilyFromReader(reader)

	assert.Equal(t, expectedDebianOSFamily, osFamily)
}

// TestGetLinuxOSFamilyMissingID tests when /etc/os-release content is missing ID
func TestGetLinuxOSFamilyMissingID(t *testing.T) {
	reader := strings.NewReader(debianOSReleaseMissingID)
	osFamily := getLinuxOSFamilyFromReader(reader)

	assert.Equal(t, unsupportedOSFamily, osFamily)
}

// TestGetLinuxOSFamilyMissingVersionID tests when /etc/os-release content is missing VERSION_ID
func TestGetLinuxOSFamilyMissingVersionID(t *testing.T) {
	reader := strings.NewReader(debianOSReleaseMissingVersion)
	osFamily := getLinuxOSFamilyFromReader(reader)

	assert.Equal(t, unsupportedOSFamily, osFamily)
}

// TestGetLinuxOSFamilyEmptyContent tests when os-release content is empty
func TestGetLinuxOSFamilyEmptyContent(t *testing.T) {
	reader := strings.NewReader("")
	osFamily := getLinuxOSFamilyFromReader(reader)

	assert.Equal(t, unsupportedOSFamily, osFamily)
}

// TestParseOSReleaseValue tests the function that parses values from /etc/os-release ensuring it correctly handles whitespace and quotes
func TestParseOSReleaseValue(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"debian", "debian"},
		{" debian ", "debian"},
		{`"debian"`, "debian"},
		{"'debian'", "debian"},
		{` "debian" `, "debian"},
		{" 'debian' ", "debian"},
		{"11", "11"},
		{`"11"`, "11"},
		{"", ""},
		{"  ", ""},
		{`""`, ""},
		{"''", ""},
		{`"ubuntu 20.04"`, "ubuntu 20.04"},
		{"'centos 8'", "centos 8"},
	}

	for _, tc := range testCases {
		result := parseOSReleaseValue(tc.input)
		assert.Equal(t, tc.expected, result, "parseOSReleaseValue(%q) should return %q", tc.input, tc.expected)
	}
}

// TestParseOSReleaseFromReader tests the core parsing logic directly
func TestParseOSReleaseFromReader(t *testing.T) {
	testCases := []struct {
		name              string
		content           string
		expectedID        string
		expectedVersionID string
	}{
		{"Valid content", debianOSRelease, "debian", "11"},
		{"Missing ID", debianOSReleaseMissingID, "", "11"},
		{"Missing VERSION_ID", debianOSReleaseMissingVersion, "debian", ""},
		{"Empty content", "", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(tc.content)
			id, versionID, err := parseOSReleaseFromReader(reader)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedID, id)
			assert.Equal(t, tc.expectedVersionID, versionID)
		})
	}
}
