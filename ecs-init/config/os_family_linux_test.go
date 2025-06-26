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
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	expectedDebianOSFamily = "debian_11"
	expectedExistingValue  = "existing_value"

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

func mockCommand(command string, args ...string) *exec.Cmd {
	if command == "cat" && len(args) > 0 && args[0] == osReleaseFilePath {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		return cmd
	}
	return exec.Command(command, args...)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	switch {
	case os.Getenv("MOCK_OS_RELEASE") == "1":
		os.Stdout.WriteString(debianOSRelease)
	case os.Getenv("MOCK_OS_RELEASE_MISSING_ID") == "1":
		os.Stdout.WriteString(debianOSReleaseMissingID)
	case os.Getenv("MOCK_OS_RELEASE_MISSING_VERSION_ID") == "1":
		os.Stdout.WriteString(debianOSReleaseMissingVersion)
	}
}

func setupTest(t *testing.T, mockFunc func(string, ...string) *exec.Cmd) func() {
	originalExecCommand := execCommand
	execCommand = mockFunc

	originalValue := os.Getenv(OSFamilyEnvVar)

	return func() {
		execCommand = originalExecCommand
		if originalValue == "" {
			os.Unsetenv(OSFamilyEnvVar)
		} else {
			os.Setenv(OSFamilyEnvVar, originalValue)
		}
	}
}

// TestGetLinuxOSFamily tests parsing a happy path /etc/os-release file
func TestGetLinuxOSFamily(t *testing.T) {
	cleanup := setupTest(t, func(command string, args ...string) *exec.Cmd {
		cmd := mockCommand(command, args...)
		if command == "cat" && len(args) > 0 && args[0] == osReleaseFilePath {
			cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "MOCK_OS_RELEASE=1"}
		}
		return cmd
	})
	defer cleanup()

	osFamily := GetLinuxOSFamily()

	assert.Equal(t, expectedDebianOSFamily, osFamily)

	envValue := os.Getenv(OSFamilyEnvVar)
	assert.Equal(t, expectedDebianOSFamily, envValue)
}

// TestGetLinuxOSFamilyMissingID tests when /etc/os-release file is missing the ID field
func TestGetLinuxOSFamilyMissingID(t *testing.T) {
	cleanup := setupTest(t, func(command string, args ...string) *exec.Cmd {
		cmd := mockCommand(command, args...)
		if command == "cat" && len(args) > 0 && args[0] == osReleaseFilePath {
			cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "MOCK_OS_RELEASE_MISSING_ID=1"}
		}
		return cmd
	})
	defer cleanup()

	osFamily := GetLinuxOSFamily()

	assert.Equal(t, unsupportedOSFamily, osFamily)
}

// TestGetLinuxOSFamilyMissingVersionID tests when /etc/os-release file is missing the VERSION_ID field
func TestGetLinuxOSFamilyMissingVersionID(t *testing.T) {
	cleanup := setupTest(t, func(command string, args ...string) *exec.Cmd {
		cmd := mockCommand(command, args...)
		if command == "cat" && len(args) > 0 && args[0] == osReleaseFilePath {
			cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "MOCK_OS_RELEASE_MISSING_VERSION_ID=1"}
		}
		return cmd
	})
	defer cleanup()

	osFamily := GetLinuxOSFamily()

	assert.Equal(t, unsupportedOSFamily, osFamily)
}

// TestGetLinuxOSFamilyCommandError tests when the cat command fails to execute
func TestGetLinuxOSFamilyCommandError(t *testing.T) {
	cleanup := setupTest(t, func(command string, args ...string) *exec.Cmd {
		return exec.Command("false")
	})
	defer cleanup()

	osFamily := GetLinuxOSFamily()

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

func TestGetLinuxOSFamilyEnvironmentVariableHandling(t *testing.T) {
	cleanup := setupTest(t, func(command string, args ...string) *exec.Cmd {
		cmd := mockCommand(command, args...)
		if command == "cat" && len(args) > 0 && args[0] == osReleaseFilePath {
			cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", "MOCK_OS_RELEASE=1"}
		}
		return cmd
	})
	defer cleanup()

	os.Setenv(OSFamilyEnvVar, expectedExistingValue)

	osFamily := GetLinuxOSFamily()

	assert.Equal(t, expectedDebianOSFamily, osFamily)

	envValue := os.Getenv(OSFamilyEnvVar)
	assert.Equal(t, expectedDebianOSFamily, envValue)
}
