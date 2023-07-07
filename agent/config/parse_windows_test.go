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

func TestParseFSxWindowsFileServerCapabilityDefault(t *testing.T) {
	IsWindows2016 = func() (bool, error) {
		return false, nil
	}

	assert.True(t, parseFSxWindowsFileServerCapability().Enabled())
}

func TestParseDomainlessgMSACapabilityFalseOnW2016(t *testing.T) {
	IsWindows2016 = func() (bool, error) {
		return true, nil
	}

	os.Setenv("ECS_GMSA_SUPPORTED", "True")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	fnQueryDomainlessGmsaPluginSubKeys = func() ([]string, error) {
		return []string{}, nil
	}

	assert.False(t, parseGMSADomainlessCapability().Enabled())
}

func TestParseDomainlessgMSACapabilityFalsePluginNoEnv(t *testing.T) {
	IsWindows2016 = func() (bool, error) {
		return false, nil
	}

	os.Setenv("ECS_GMSA_SUPPORTED", "False")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	fnQueryDomainlessGmsaPluginSubKeys = func() ([]string, error) {
		return []string{"{859E1386-BDB4-49E8-85C7-3070B13920E1}"}, nil
	}

	assert.False(t, parseGMSADomainlessCapability().Enabled())
}

func TestParseDomainlessgMSACapabilityFalseNoPlugin(t *testing.T) {
	IsWindows2016 = func() (bool, error) {
		return false, nil
	}

	os.Setenv("ECS_GMSA_SUPPORTED", "True")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	fnQueryDomainlessGmsaPluginSubKeys = func() ([]string, error) {
		return []string{}, nil
	}

	assert.False(t, parseGMSADomainlessCapability().Enabled())
}

func TestParseDomainlessgMSACapabilityTrue(t *testing.T) {
	IsWindows2016 = func() (bool, error) {
		return false, nil
	}

	os.Setenv("ECS_GMSA_SUPPORTED", "True")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	fnQueryDomainlessGmsaPluginSubKeys = func() ([]string, error) {
		return []string{"{859E1386-BDB4-49E8-85C7-3070B13920E1}"}, nil
	}

	assert.True(t, parseGMSADomainlessCapability().Enabled())
}

func TestParseTaskPidsLimit(t *testing.T) {
	t.Setenv("ECS_TASK_PIDS_LIMIT", "1")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "10")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "100")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "10000")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "0")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "-1")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "foobar")
	assert.Equal(t, 0, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "")
	assert.Equal(t, 0, parseTaskPidsLimit())
}

func TestParseTaskPidsLimit_Unset(t *testing.T) {
	assert.Equal(t, 0, parseTaskPidsLimit())
}
