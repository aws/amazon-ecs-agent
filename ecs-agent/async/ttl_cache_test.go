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

package async

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTTLSimple(t *testing.T) {
	ttl := NewTTLCache(&TTL{Duration: time.Minute})
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
	ttl := NewTTLCache(&TTL{Duration: time.Minute})

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
	ttl := NewTTLCache(&TTL{Duration: 50 * time.Millisecond})
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

func TestTTLCacheGetTTLAndSetTTL(t *testing.T) {
	entryKey := "foo"
	entryVal := "bar"

	// Initialize cache with a nil TTL (i.e., infinite amount of time).
	cache := NewTTLCache(nil)
	require.Nil(t, cache.GetTTL())

	// Add entry to the cache.
	cache.Set(entryKey, entryVal)
	time.Sleep(100 * time.Millisecond)

	// We should be able to retrieve the entry - it should not be expired since the cache's current TTL has not elapsed.
	actualVal, expired, ok := cache.Get(entryKey)
	require.False(t, expired)
	require.True(t, ok)
	require.Equal(t, entryVal, actualVal)

	// Set TTL of cache to now be a low amount of time.
	newTTLDuration := 1 * time.Millisecond
	cache.SetTTL(&TTL{Duration: newTTLDuration})
	require.NotNil(t, cache.GetTTL())
	require.Equal(t, newTTLDuration, cache.GetTTL().Duration)
	time.Sleep(100 * time.Millisecond)

	// We should be able to retrieve the entry - it should be expired since the cache's current TTL has elapsed.
	actualVal, expired, ok = cache.Get(entryKey)
	require.True(t, ok)
	require.True(t, expired)
	require.Equal(t, entryVal, actualVal)
}

func TestTTLSetNilTTLWhenCacheTTLNotNil(t *testing.T) {
	entryKey := "foo"
	entryVal := "bar"

	// Initialize cache with a 1 second TTL (i.e., not nil TTL).
	initialTTLDuration := 1 * time.Nanosecond
	cache := NewTTLCache(&TTL{Duration: initialTTLDuration})
	require.Equal(t, initialTTLDuration, cache.GetTTL().Duration)

	// Add entry to the cache.
	cache.Set(entryKey, entryVal)
	time.Sleep(2 * initialTTLDuration)

	// We should be able to retrieve the entry - it should be expired.
	actualVal, expired, ok := cache.Get(entryKey)
	require.True(t, ok)
	require.True(t, expired)
	require.Equal(t, entryVal, actualVal)

	// Set TTL of cache to now be nil.
	cache.SetTTL(nil)
	require.Nil(t, cache.GetTTL())

	// We should be able to retrieve the entry - it should not be expired.
	actualVal, expired, ok = cache.Get(entryKey)
	require.True(t, ok)
	require.False(t, expired)
	require.Equal(t, entryVal, actualVal)
}

func TestTTLSetNilTTLWhenCacheTTLNil(t *testing.T) {
	entryKey := "foo"
	entryVal := "bar"

	// Initialize cache with a nil TTL.
	cache := NewTTLCache(nil)
	require.Nil(t, cache.GetTTL())

	// Add entry to the cache.
	cache.Set(entryKey, entryVal)

	// We should be able to retrieve the entry - it should not be expired.
	actualVal, expired, ok := cache.Get(entryKey)
	require.True(t, ok)
	require.False(t, expired)
	require.Equal(t, entryVal, actualVal)

	// Set TTL of cache to now be nil.
	cache.SetTTL(nil)
	require.Nil(t, cache.GetTTL())

	// We should be able to retrieve the entry - it should still not be expired.
	actualVal, expired, ok = cache.Get(entryKey)
	require.True(t, ok)
	require.False(t, expired)
	require.Equal(t, entryVal, actualVal)
}
