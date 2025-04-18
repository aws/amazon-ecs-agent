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

package networkinterface

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIPv6Only(t *testing.T) {
	t.Run("no IP addresses", func(t *testing.T) {
		assert.False(t, (&NetworkInterface{}).IsIPv6Only())
	})
	t.Run("IPv4 addresses only", func(t *testing.T) {
		assert.False(
			t,
			(&NetworkInterface{
				IPV4Addresses: []*IPV4Address{{Primary: true, Address: "1.2.3.4"}},
			}).IsIPv6Only())
	})
	t.Run("dual stack", func(t *testing.T) {
		assert.False(
			t,
			(&NetworkInterface{
				IPV4Addresses: []*IPV4Address{{Primary: true, Address: "1.2.3.4"}},
				IPV6Addresses: []*IPV6Address{{Address: "1:2:3:4::"}},
			}).IsIPv6Only())
	})
	t.Run("IPv6 addresses only", func(t *testing.T) {
		assert.True(
			t,
			(&NetworkInterface{
				IPV6Addresses: []*IPV6Address{{Address: "1:2:3:4::"}},
			}).IsIPv6Only())
	})
}
