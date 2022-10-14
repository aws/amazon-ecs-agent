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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGMSACapabilitySupported(t *testing.T) {
	os.Setenv("ECS_GMSA_SUPPORTED", "True")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	os.Setenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE", "True")
	defer os.Unsetenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE")

	os.Setenv("CREDENTIALS_FETCHER_HOST_DIR", "/var/run")
	defer os.Unsetenv("CREDENTIALS_FETCHER_HOST_DIR")

	assert.True(t, parseGMSACapability())
}

func TestParseGMSACapabilityNonDomainJoined(t *testing.T) {
	os.Setenv("ECS_GMSA_SUPPORTED", "True")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	os.Setenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE", "False")
	defer os.Unsetenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE")

	assert.False(t, parseGMSACapability())
}

func TestParseGMSACapabilityUnsupported(t *testing.T) {
	os.Setenv("ECS_GMSA_SUPPORTED", "False")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")

	assert.False(t, parseGMSACapability())
}

func TestSkipDomainJoinCheckParseGMSACapability(t *testing.T) {
	os.Setenv("ECS_GMSA_SUPPORTED", "True")
	defer os.Unsetenv("ECS_GMSA_SUPPORTED")
	os.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")
	defer os.Unsetenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION")

	assert.True(t, parseGMSACapability())
}
