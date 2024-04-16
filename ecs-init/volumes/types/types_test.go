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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddMount(t *testing.T) {
	t.Run("new map is created when Mounts is nil", func(t *testing.T) {
		v := &Volume{}
		v.AddMount("id")
		assert.Equal(t, map[string]*string{"id": nil}, v.Mounts)
	})
	t.Run("second mount", func(t *testing.T) {
		v := &Volume{}
		v.AddMount("id")
		v.AddMount("id2")
		assert.Equal(t, map[string]*string{"id": nil, "id2": nil}, v.Mounts)
	})
	t.Run("mount already exists", func(t *testing.T) {
		v := &Volume{}
		v.AddMount("id")
		assert.Equal(t, map[string]*string{"id": nil}, v.Mounts)
	})
}

func TestRemoveMount(t *testing.T) {
	t.Run("no-op when mount not found", func(t *testing.T) {
		v := &Volume{}
		assert.False(t, v.RemoveMount("id"))
		assert.Empty(t, v.Mounts)
	})
	t.Run("mount should be removed if it exists", func(t *testing.T) {
		v := &Volume{}
		v.AddMount("id")
		assert.True(t, v.RemoveMount("id"))
		assert.Empty(t, v.Mounts)
	})
}
