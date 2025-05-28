//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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
package ipcompatibility

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPCompatibility(t *testing.T) {
	t.Run("IPv4 Compatibility", func(t *testing.T) {
		ic := NewIPCompatibility(true, false)
		assert.True(t, ic.IsIPv4Compatible(), "IPv4 should be compatible")

		ic = NewIPCompatibility(false, false)
		assert.False(t, ic.IsIPv4Compatible(), "IPv4 should be incompatible")
	})

	t.Run("IPv6 Compatibility", func(t *testing.T) {
		ic := NewIPCompatibility(false, true)
		assert.True(t, ic.IsIPv6Compatible(), "IPv6 should be compatible")

		ic = NewIPCompatibility(true, false)
		assert.False(t, ic.IsIPv6Compatible(), "IPv6 should be incompatible")
	})
}

func TestIPv4OnlyCompatibility(t *testing.T) {
	c := NewIPv4OnlyCompatibility()
	assert.True(t, c.IsIPv4Compatible())
	assert.False(t, c.IsIPv6Compatible())
}

func TestIPv6OnlyCompatibility(t *testing.T) {
	c := NewIPv6OnlyCompatibility()
	assert.True(t, c.IsIPv6Compatible())
	assert.False(t, c.IsIPv4Compatible())
}

func TestIsIPv6Only(t *testing.T) {
	tests := []struct {
		name     string
		ipCompat IPCompatibility
		expected bool
	}{
		{
			name:     "IPv6 only network",
			ipCompat: IPCompatibility{false, true},
			expected: true,
		},
		{
			name:     "Dual stack network",
			ipCompat: IPCompatibility{true, true},
			expected: false,
		},
		{
			name:     "IPv4 only network",
			ipCompat: IPCompatibility{true, false},
			expected: false,
		},
		{
			name:     "No IP support",
			ipCompat: IPCompatibility{false, false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.ipCompat.IsIPv6Only())
		})
	}
}

func TestNewDualStackCompatibility(t *testing.T) {
	ipcompat := NewDualStackCompatibility()
	assert.True(t, ipcompat.IsIPv4Compatible())
	assert.True(t, ipcompat.IsIPv6Compatible())
}
