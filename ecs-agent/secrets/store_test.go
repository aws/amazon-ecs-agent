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

// Tests exercise the unexported generic store[T] directly since they are
// in the same package. Domain interface tests live in their own files.

func TestStoreString(t *testing.T) {
	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Set and Get with valid inputs",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[string]()
				err := s.Set("container-1", "my-secret-token")
				require.NoError(t, err)
				val, ok := s.Get("container-1")
				assert.True(t, ok)
				assert.Equal(t, "my-secret-token", val)
			},
		},
		{
			name: "Set with empty ID returns error",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[string]()
				err := s.Set("", "some-value")
				assert.Error(t, err)
				assert.Equal(t, "secret ID must not be empty", err.Error())
			},
		},
		{
			name: "Get with non-existent ID returns false",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[string]()
				val, ok := s.Get("does-not-exist")
				assert.False(t, ok)
				assert.Equal(t, "", val)
			},
		},
		{
			name: "Remove with valid ID",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[string]()
				require.NoError(t, s.Set("container-1", "token"))
				s.Remove("container-1")
				val, ok := s.Get("container-1")
				assert.False(t, ok)
				assert.Equal(t, "", val)
			},
		},
		{
			name: "Remove with non-existent ID is no-op",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[string]()
				// Should not panic or error.
				s.Remove("does-not-exist")
			},
		},
		{
			name: "Overwrite returns latest value",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[string]()
				require.NoError(t, s.Set("container-1", "first"))
				require.NoError(t, s.Set("container-1", "second"))
				val, ok := s.Get("container-1")
				assert.True(t, ok)
				assert.Equal(t, "second", val)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}

func TestStoreMap(t *testing.T) {
	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Set and Get map value",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[map[string]string]()
				env := map[string]string{"KEY1": "val1", "KEY2": "val2"}
				require.NoError(t, s.Set("container-1", env))
				val, ok := s.Get("container-1")
				assert.True(t, ok)
				assert.Equal(t, env, val)
			},
		},
		{
			name: "Set with empty ID returns error for map store",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[map[string]string]()
				err := s.Set("", map[string]string{"K": "V"})
				assert.Error(t, err)
				assert.Equal(t, "secret ID must not be empty", err.Error())
			},
		},
		{
			name: "Get non-existent ID returns nil map and false",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[map[string]string]()
				val, ok := s.Get("missing")
				assert.False(t, ok)
				assert.Nil(t, val)
			},
		},
		{
			name: "Overwrite map returns latest value",
			test: func(t *testing.T) {
				t.Parallel()
				s := newStore[map[string]string]()
				require.NoError(t, s.Set("c1", map[string]string{"A": "1"}))
				require.NoError(t, s.Set("c1", map[string]string{"B": "2"}))
				val, ok := s.Get("c1")
				assert.True(t, ok)
				assert.Equal(t, map[string]string{"B": "2"}, val)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}
