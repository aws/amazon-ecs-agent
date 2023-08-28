//go:build unit
// +build unit

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

package ecscni

import (
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	// ECSCNIVersion, ECSCNIGitHash, VPCCNIGitHash needs to be updated every time CNI plugin is updated.
	currentECSCNIVersion = "2020.09.0"
	currentECSCNIGitHash = "53a8481891251e66e35847554d52a13fc7c4fd03"
	currentVPCCNIGitHash = "be5214353252f8315a1341f4df9ffbd8cf69000c"
)

// Asserts that CNI plugin version matches the expected version
func TestCNIPluginVersionNumber(t *testing.T) {
	versionStr := getCNIVersionString(t)
	assert.Equal(t, currentECSCNIVersion, versionStr)
}

// Asserts that CNI plugin version is upgraded when new commits are made to CNI plugin submodule
func TestCNIPluginVersionUpgrade(t *testing.T) {
	versionStr := getCNIVersionString(t)
	cmd := exec.Command("git", "submodule")
	versionInfo, err := cmd.Output()
	assert.NoError(t, err, "Error running the command: git submodule")
	versionInfoStrList := strings.Split(string(versionInfo), "\n")
	// If a new commit is added, version should be upgraded
	if currentECSCNIGitHash != strings.Split(versionInfoStrList[0], " ")[1] {
		assert.NotEqual(t, currentECSCNIVersion, versionStr)
	}
	assert.Equal(t, currentVPCCNIGitHash, strings.Split(versionInfoStrList[1], " ")[1])
}

// Returns the version in CNI plugin VERSION file as a string
func getCNIVersionString(t *testing.T) string {
	// ../../amazon-ecs-cni-plugins/VERSION
	versionFilePath := filepath.Clean(filepath.Join("..", "..", "amazon-ecs-cni-plugins", "VERSION"))
	versionStr, err := ioutil.ReadFile(versionFilePath)
	assert.NoError(t, err, "Error reading the CNI plugin version file")
	return strings.TrimSpace(string(versionStr))
}
