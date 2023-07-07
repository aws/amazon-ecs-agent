//go:build linux && unit
// +build linux,unit

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGMSACapabilitySupported(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE", "True")
	t.Setenv("CREDENTIALS_FETCHER_HOST_DIR", "/var/run")

	assert.True(t, parseGMSACapability().Enabled())
}

func TestParseGMSACapabilityNonDomainJoined(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE", "False")

	assert.False(t, parseGMSACapability().Enabled())
}

func TestParseGMSACapabilityUnsupported(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "False")

	assert.False(t, parseGMSACapability().Enabled())
}

func TestParseFSxWindowsFileServerCapabilityUsingEnv(t *testing.T) {
	t.Setenv("ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED", "True")

	assert.False(t, parseFSxWindowsFileServerCapability().Enabled())
}

func TestParseFSxWindowsFileServerCapabilityDefault(t *testing.T) {
	assert.False(t, parseFSxWindowsFileServerCapability().Enabled())
}

func TestSkipDomainJoinCheckParseGMSACapability(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	assert.True(t, parseGMSACapability().Enabled())
}

func TestParseGMSADomainLessCapabilitySupported(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("CREDENTIALS_FETCHER_HOST_DIR", "/var/run")

	assert.True(t, parseGMSADomainlessCapability().Enabled())
}

func TestParseGMSADomainLessCapabilityUnSupported(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("CREDENTIALS_FETCHER_HOST_DIR", "")

	assert.False(t, parseGMSADomainlessCapability().Enabled())
}

func TestSkipDomainLessCheckParseGMSACapability(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_LESS_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	assert.True(t, parseGMSADomainlessCapability().Enabled())
}

func TestParseTaskPidsLimit(t *testing.T) {
	t.Setenv("ECS_TASK_PIDS_LIMIT", "1")
	assert.Equal(t, 1, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "10")
	assert.Equal(t, 10, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "100")
	assert.Equal(t, 100, parseTaskPidsLimit())
	t.Setenv("ECS_TASK_PIDS_LIMIT", "10000")
	assert.Equal(t, 10000, parseTaskPidsLimit())
	// test the upper limit minus 1
	t.Setenv("ECS_TASK_PIDS_LIMIT", "4194304")
	assert.Equal(t, 4194304, parseTaskPidsLimit())
	// test the upper limit
	t.Setenv("ECS_TASK_PIDS_LIMIT", "4194305")
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
