//go:build unit
// +build unit

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

func TestGetSubnetGatewayIPv6Address(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty address",
			input:    "",
			expected: "",
		},
		{
			name:     "ipv6 address with prefix",
			input:    "2001:db8:85a3::8a2e:370:7334/64",
			expected: "2001:db8:85a3::8a2e:370:7334",
		},
		{
			name:     "ipv6 address without prefix",
			input:    "2001:db8:85a3::8a2e:370:7334",
			expected: "2001:db8:85a3::8a2e:370:7334",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ni := &NetworkInterface{SubnetGatewayIPV6Address: tt.input}
			assert.Equal(t, tt.expected, ni.GetSubnetGatewayIPv6Address())
		})
	}
}
