//go:build windows && unit
// +build windows,unit

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

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGMSACapability(t *testing.T) {
	os.Setenv("ECS_GMSA_SUPPORTED", "False")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	assert.False(t, parseGMSACapability().Enabled())
}

func TestParseBooleanEnvVar(t *testing.T) {
	os.Setenv("EXAMPLE_SETTING", "True")
	defer os.Unsetenv("EXAMPLE_SETTING")

	assert.True(t, parseBooleanDefaultFalseConfig("EXAMPLE_SETTING").Enabled())
	assert.True(t, parseBooleanDefaultTrueConfig("EXAMPLE_SETTING").Enabled())

	os.Setenv("EXAMPLE_SETTING", "False")
	assert.False(t, parseBooleanDefaultFalseConfig("EXAMPLE_SETTING").Enabled())
	assert.False(t, parseBooleanDefaultTrueConfig("EXAMPLE_SETTING").Enabled())
}

func TestParseFSxWindowsFileServerCapability(t *testing.T) {
	IsWindows2016 = func() (bool, error) {
		return false, nil
	}
	os.Setenv("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED", "False")
	defer os.Unsetenv("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED")

	assert.False(t, parseFSxWindowsFileServerCapability().Enabled())
}
