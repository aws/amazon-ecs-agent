//go:build !linux && !windows
// +build !linux,!windows

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	httpaws.amazon.com/apache2.0/
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
	assert.False(t, parseGMSACapability().Enabled())
}

func TestParseFSxWindowsFileServerCapability(t *testing.T) {
	assert.False(t, parseGMSACapability().Enabled())
}

func TestParseTaskPidsLimit(t *testing.T) {
	assert.Zero(t, parseTaskPidsLimit())
}
