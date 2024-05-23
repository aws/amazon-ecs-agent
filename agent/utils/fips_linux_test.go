//go:build !windows && unit
// +build !windows,unit

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

package utils

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// detectFIPSModeWithPath is a helper function for testing
func detectFIPSModeWithPath(filePath string) bool {
	data, err := ioutil.ReadFile(filePath)
	if err == nil && strings.TrimSpace(string(data)) == "1" {
		return true
	}
	return false
}
func TestDetectFIPSMode(t *testing.T) {
	// Create a temporary file to mock the FIPS mode file
	tempFile, err := ioutil.TempFile("", "fips_enabled")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())
	// Test FIPS mode enabled
	_, err = tempFile.WriteString("1\n")
	assert.NoError(t, err)
	tempFile.Sync()
	// Initialize the logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	result := detectFIPSModeWithPath(tempFile.Name())
	assert.True(t, result, "FIPS mode should be detected")
	// Test FIPS mode disabled
	tempFile.Truncate(0)
	tempFile.Seek(0, 0)
	_, err = tempFile.WriteString("0\n")
	assert.NoError(t, err)
	tempFile.Sync()
	result = detectFIPSModeWithPath(tempFile.Name())
	assert.False(t, result, "FIPS mode should not be detected")
}
