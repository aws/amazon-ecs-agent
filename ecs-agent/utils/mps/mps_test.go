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

package mps

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func uintPtr(v uint) *uint { return &v }

func TestPinnedDeviceMemLimit(t *testing.T) {
	// Always keyed on in-container device index 0, MiB value, "M" suffix.
	assert.Equal(t, "0=1024M", PinnedDeviceMemLimit(1024))
	assert.Equal(t, "0=1M", PinnedDeviceMemLimit(1))
	assert.Equal(t, "0=22563M", PinnedDeviceMemLimit(22563))
}

func TestBuildEnvWithComputePercent(t *testing.T) {
	env := BuildEnv(4096, uintPtr(50))
	assert.Equal(t, map[string]string{
		PipeDirectoryEnvVar:          PipeDirectory,
		PinnedDeviceMemLimitEnvVar:   "0=4096M",
		ActiveThreadPercentageEnvVar: "50",
	}, env)
}

func TestBuildEnvWithoutComputePercent(t *testing.T) {
	// When compute percent is omitted, the active-thread env var must be absent
	// entirely (daemon default of 100% applies), not set to 0 or 100.
	env := BuildEnv(4096, nil)
	assert.Equal(t, map[string]string{
		PipeDirectoryEnvVar:        PipeDirectory,
		PinnedDeviceMemLimitEnvVar: "0=4096M",
	}, env)
	_, ok := env[ActiveThreadPercentageEnvVar]
	assert.False(t, ok, "compute percent env var must be omitted when nil")
}

func TestBuildEnvComputePercentBoundaries(t *testing.T) {
	env := BuildEnv(1, uintPtr(1))
	assert.Equal(t, "1", env[ActiveThreadPercentageEnvVar])

	env = BuildEnv(1, uintPtr(100))
	assert.Equal(t, "100", env[ActiveThreadPercentageEnvVar])
}
