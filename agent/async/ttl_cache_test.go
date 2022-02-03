//go:build unit

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

package async

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTTLSimple(t *testing.T) {
	ttl := NewTTLCache(time.Minute)
	ttl.Set("foo", "bar")

	bar, expired, ok := ttl.Get("foo")
	require.True(t, ok)
	require.False(t, expired)
	require.Equal(t, bar, "bar")

	baz, expired, ok := ttl.Get("fooz")
	require.False(t, ok)
	require.False(t, expired)
	require.Nil(t, baz)

	ttl.Delete("foo")
	bar, expired, ok = ttl.Get("foo")
	require.False(t, ok)
	require.False(t, expired)
	require.Nil(t, bar)
}

func TestTTLSetDelete(t *testing.T) {
	ttl := NewTTLCache(time.Minute)

	ttl.Set("foo", "bar")
	bar, expired, ok := ttl.Get("foo")
	require.True(t, ok)
	require.False(t, expired)
	require.Equal(t, bar, "bar")

	ttl.Set("foo", "bar2")
	bar, expired, ok = ttl.Get("foo")
	require.True(t, ok)
	require.False(t, expired)
	require.Equal(t, bar, "bar2")

	ttl.Delete("foo")
	bar, expired, ok = ttl.Get("foo")
	require.False(t, ok)
	require.False(t, expired)
	require.Nil(t, bar)
}

func TestTTLCache(t *testing.T) {
	ttl := NewTTLCache(50 * time.Millisecond)
	ttl.Set("foo", "bar")

	bar, expired, ok := ttl.Get("foo")
	require.False(t, expired)
	require.True(t, ok)
	require.Equal(t, bar, "bar")

	time.Sleep(100 * time.Millisecond)

	bar, expired, ok = ttl.Get("foo")
	require.True(t, ok)
	require.True(t, expired)
	require.Equal(t, bar, "bar")
}
