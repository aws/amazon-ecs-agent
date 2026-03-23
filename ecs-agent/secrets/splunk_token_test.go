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

func TestSplunkTokenStore(t *testing.T) {
	testCases := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Set and Get token",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewSplunkTokenStore()
				require.NoError(t, s.Set("cid-1", "tok-abc"))
				val, ok := s.Get("cid-1")
				assert.True(t, ok)
				assert.Equal(t, "tok-abc", val)
			},
		},
		{
			name: "Set with empty ID returns error",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewSplunkTokenStore()
				err := s.Set("", "tok")
				assert.Error(t, err)
				assert.Equal(t, "secret ID must not be empty", err.Error())
			},
		},
		{
			name: "Get non-existent returns false",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewSplunkTokenStore()
				val, ok := s.Get("missing")
				assert.False(t, ok)
				assert.Equal(t, "", val)
			},
		},
		{
			name: "Remove then Get returns false",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewSplunkTokenStore()
				require.NoError(t, s.Set("cid-1", "tok"))
				s.Remove("cid-1")
				_, ok := s.Get("cid-1")
				assert.False(t, ok)
			},
		},
		{
			name: "Overwrite returns latest token",
			test: func(t *testing.T) {
				t.Parallel()
				s := NewSplunkTokenStore()
				require.NoError(t, s.Set("cid-1", "first"))
				require.NoError(t, s.Set("cid-1", "second"))
				val, ok := s.Get("cid-1")
				assert.True(t, ok)
				assert.Equal(t, "second", val)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.test)
	}
}
