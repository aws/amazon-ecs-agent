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

	assert.True(t, parseGMSACapability())
}

func TestParseGMSACapabilityNonDomainJoined(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ECS_DOMAIN_JOINED_LINUX_INSTANCE", "False")

	assert.False(t, parseGMSACapability())
}

func TestParseGMSACapabilityUnsupported(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "False")

	assert.False(t, parseGMSACapability())
}

func TestSkipDomainJoinCheckParseGMSACapability(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	assert.True(t, parseGMSACapability())
}
