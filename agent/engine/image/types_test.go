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

//go:build unit

package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseManagedEnvKeys(t *testing.T) {
	tests := []struct {
		name     string
		imageEnv []string
		wantKeys map[string]bool
	}{
		{
			name:     "no managed keys in image env",
			imageEnv: []string{"PATH=/usr/local/bin", "HOME=/root"},
			wantKeys: map[string]bool{},
		},
		{
			name:     "AWS_REGION present with value",
			imageEnv: []string{"AWS_REGION=us-east-1"},
			wantKeys: map[string]bool{"AWS_REGION": true},
		},
		{
			name:     "AWS_DEFAULT_REGION present with value",
			imageEnv: []string{"PATH=/usr/local/bin", "AWS_DEFAULT_REGION=eu-west-1"},
			wantKeys: map[string]bool{"AWS_DEFAULT_REGION": true},
		},
		{
			name:     "both region vars present",
			imageEnv: []string{"AWS_REGION=us-east-1", "AWS_DEFAULT_REGION=eu-west-1"},
			wantKeys: map[string]bool{"AWS_REGION": true, "AWS_DEFAULT_REGION": true},
		},
		{
			name:     "AWS_REGION set to empty (KEY=) — still recorded",
			imageEnv: []string{"AWS_REGION="},
			wantKeys: map[string]bool{"AWS_REGION": true},
		},
		{
			name:     "AWS_REGION bare with no equals (KEY) — still recorded",
			imageEnv: []string{"AWS_REGION"},
			wantKeys: map[string]bool{"AWS_REGION": true},
		},
		{
			name:     "nil image env",
			imageEnv: nil,
			wantKeys: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseManagedEnvKeys(tt.imageEnv)
			assert.Equal(t, tt.wantKeys, result)
		})
	}
}
