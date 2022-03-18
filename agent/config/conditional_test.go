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

package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBooleanDefaultTrue(t *testing.T) {
	x := BooleanDefaultTrue{Value: NotSet}
	y := BooleanDefaultTrue{Value: ExplicitlyEnabled}
	z := BooleanDefaultTrue{Value: ExplicitlyDisabled}

	assert.True(t, x.Enabled(), "NotSet is enabled")
	assert.True(t, y.Enabled(), "ExplicitlyEnabled is enabled")
	assert.False(t, z.Enabled(), "ExplicitlyDisabled is not enabled")
}

func TestBooleanDefaultTrueImplements(t *testing.T) {
	assert.Implements(t, (*json.Marshaler)(nil), BooleanDefaultTrue{Value: NotSet})
	assert.Implements(t, (*json.Unmarshaler)(nil), (*BooleanDefaultTrue)(nil))
}

// main conversion cases for Conditional marshalling
var cases = []struct {
	defaultBool BooleanDefaultTrue
	bytes       []byte
}{
	{BooleanDefaultTrue{Value: ExplicitlyEnabled}, []byte("true")},
	{BooleanDefaultTrue{Value: ExplicitlyDisabled}, []byte("false")},
	{BooleanDefaultTrue{Value: NotSet}, []byte("null")},
}

func TestBooleanDefaultTrueMarshal(t *testing.T) {
	for _, tc := range cases {
		t.Run(string(tc.bytes), func(t *testing.T) {
			m, err := json.Marshal(tc.defaultBool)
			assert.NoError(t, err)
			assert.Equal(t, tc.bytes, m)
		})
	}
}

func TestBooleanDefaultTrueUnmarshal(t *testing.T) {
	for _, tc := range cases {
		t.Run(string(tc.bytes), func(t *testing.T) {
			var target BooleanDefaultTrue
			err := json.Unmarshal(tc.bytes, &target)
			assert.NoError(t, err)
			assert.Equal(t, tc.defaultBool, target)
		})
	}
}
