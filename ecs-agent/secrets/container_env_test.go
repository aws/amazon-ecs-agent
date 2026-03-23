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

package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerEnvStore(t *testing.T) {
	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Set and Get env map",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				env := map[string]string{"KEY1": "val1", "KEY2": "val2"}
				require.NoError(t, s.Set("cid-1", env))
				val, ok := s.Get("cid-1")
				assert.True(t, ok)
				assert.Equal(t, env, val)
			},
		},
		{
			name: "Set with empty ID returns error",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				err := s.Set("", map[string]string{"K": "V"})
				assert.Error(t, err)
				assert.Equal(t, "secret ID must not be empty", err.Error())
			},
		},
		{
			name: "Get non-existent returns nil and false",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				val, ok := s.Get("missing")
				assert.False(t, ok)
				assert.Nil(t, val)
			},
		},
		{
			name: "Remove then Get returns false",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				require.NoError(t, s.Set("cid-1", map[string]string{"A": "1"}))
				s.Remove("cid-1")
				_, ok := s.Get("cid-1")
				assert.False(t, ok)
			},
		},
		{
			name: "Overwrite returns latest env map",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				require.NoError(t, s.Set("cid-1", map[string]string{"A": "1"}))
				require.NoError(t, s.Set("cid-1", map[string]string{"B": "2"}))
				val, ok := s.Get("cid-1")
				assert.True(t, ok)
				assert.Equal(t, map[string]string{"B": "2"}, val)
			},
		},
		{
			name: "Mutating original map after Set does not affect stored value",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				env := map[string]string{"K": "V"}
				require.NoError(t, s.Set("cid-1", env))
				env["INJECTED"] = "bad"
				stored, ok := s.Get("cid-1")
				assert.True(t, ok)
				assert.NotContains(t, stored, "INJECTED")
				assert.Equal(t, map[string]string{"K": "V"}, stored)
			},
		},
		{
			name: "Mutating returned map does not affect stored value",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewContainerEnvStore()
				require.NoError(t, s.Set("cid-1", map[string]string{"K": "V"}))
				val, _ := s.Get("cid-1")
				val["INJECTED"] = "bad"
				original, ok := s.Get("cid-1")
				assert.True(t, ok)
				assert.NotContains(t, original, "INJECTED")
				assert.Equal(t, map[string]string{"K": "V"}, original)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}
