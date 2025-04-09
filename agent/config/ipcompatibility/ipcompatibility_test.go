//go:build unit && linux
// +build unit,linux

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
package ipcompatibility

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIpCompatibility(t *testing.T) {
	ic := NewIPCompatibility()

	t.Run("IPv4 Compatibility", func(t *testing.T) {
		ic.SetIPv4Compatible(true)
		assert.True(t, ic.IsIPv4Compatible(), "IPv4 should be compatible")

		ic.SetIPv4Compatible(false)
		assert.False(t, ic.IsIPv4Compatible(), "IPv4 should be incompatible")
	})

	t.Run("IPv6 Compatibility", func(t *testing.T) {
		ic.SetIPv6Compatible(true)
		assert.True(t, ic.IsIPv6Compatible(), "IPv6 should be compatible")

		ic.SetIPv6Compatible(false)
		assert.False(t, ic.IsIPv6Compatible(), "IPv6 should be incompatible")
	})
}
