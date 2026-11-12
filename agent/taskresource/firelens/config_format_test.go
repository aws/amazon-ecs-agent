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

package firelens

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsYAMLExternalConfigValue(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "s3 arn with yaml extension",
			value:    "arn:aws:s3:::bucket/fluent-bit-config.yaml",
			expected: true,
		},
		{
			name:     "s3 arn with yml extension",
			value:    "arn:aws:s3:::bucket/fluent-bit-config.yml",
			expected: true,
		},
		{
			name:     "s3 arn with upper case extension",
			value:    "arn:aws:s3:::bucket/fluent-bit-config.YAML",
			expected: true,
		},
		{
			name:     "s3 arn with mixed case extension",
			value:    "arn:aws:s3:::bucket/fluent-bit-config.Yml",
			expected: true,
		},
		{
			name:     "file path with yaml extension",
			value:    "/fluent-bit/etc/custom.yaml",
			expected: true,
		},
		{
			name:     "s3 arn with conf extension",
			value:    "arn:aws:s3:::bucket/fluent-bit-config.conf",
			expected: false,
		},
		{
			name:     "file path with no extension",
			value:    "/fluent-bit/etc/custom",
			expected: false,
		},
		{
			name:     "empty value",
			value:    "",
			expected: false,
		},
		{
			name:     "extension-like substring not at the end",
			value:    "arn:aws:s3:::bucket/fluent-bit.yaml.bak",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, IsYAMLExternalConfigValue(tc.value))
		})
	}
}
