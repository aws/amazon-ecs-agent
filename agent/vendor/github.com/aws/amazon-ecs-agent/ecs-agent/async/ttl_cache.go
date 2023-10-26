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
	"sync"
	"time"
)

type TTLCache interface {
	// Get fetches a value from cache, returns nil, false on miss.
	Get(key string) (value interface{}, expired bool, ok bool)
	// Set sets a value in cache. This overwrites any existing value.
	Set(key string, value interface{})
	// Delete deletes the value from the cache.
	Delete(key string)
	// GetTTL gets the time-to-live of the cache.
	GetTTL() *TTL
	// SetTTL sets the time-to-live of the cache.
	SetTTL(ttl *TTL)
}

// NewTTLCache creates a TTL cache with optional TTL for items.
func NewTTLCache(ttl *TTL) TTLCache {
	ttlCache := &ttlCache{
		cache: make(map[string]*ttlCacheEntry),
	}
	// Only set TTL if it is not nil.
	if ttl != nil {
		ttlCache.ttl = ttl
	}
	return ttlCache
}

type TTL struct {
	Duration time.Duration
}

type ttlCacheEntry struct {
	value  interface{}
	expiry time.Time
}

type ttlCache struct {
	mu    sync.RWMutex
	cache map[string]*ttlCacheEntry
	ttl   *TTL
}

// Get returns the value associated with the key.
// returns if the item is expired (true if key is expired).
// ok result indicates whether value was found in the map.
// Note that items are not automatically deleted from the map when they expire. They will continue to be
// returned with expired=true.
func (t *ttlCache) Get(key string) (value interface{}, expired bool, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if _, iok := t.cache[key]; !iok {
		return nil, false, false
	}
	entry := t.cache[key]
	// Entries can only be expired if the cache has a TTL set.
	if t.ttl != nil {
		expired = time.Now().After(entry.expiry)
	}
	return entry.value, expired, true
}

// Set sets the key-value pair in the cache.
func (t *ttlCache) Set(key string, value interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cache[key] = &ttlCacheEntry{
		value: value,
	}
	// Entries can only have expiry set if the cache has a TTL set.
	if t.ttl != nil {
		t.cache[key].expiry = time.Now().Add(t.ttl.Duration)
	}
}

// Delete removes the entry associated with the key from cache.
func (t *ttlCache) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cache, key)
}

// GetTTL gets the time-to-live of the cache.
func (t *ttlCache) GetTTL() *TTL {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ttl == nil {
		return nil
	}
	return &TTL{Duration: t.ttl.Duration}
}

// SetTTL sets the time-to-live of the cache. Existing entries in the cache are updated account for the new TTL.
func (t *ttlCache) SetTTL(newTTL *TTL) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// New TTL is nil, reset expiry of all entries in the cache and set cache TTL to nil if necessary.
	if newTTL == nil {
		if t.ttl != nil {
			for _, val := range t.cache {
				val.expiry = time.Time{}
			}
			t.ttl = nil
		}
		return
	}

	// New TTL is not nil, update expiry of all entries in the cache and set cache TTL accordingly.
	if t.ttl != nil {
		oldTTLDuration := t.ttl.Duration
		for _, val := range t.cache {
			val.expiry = val.expiry.Add(newTTL.Duration - oldTTLDuration)
		}
	} else {
		now := time.Now()
		for _, val := range t.cache {
			val.expiry = now.Add(newTTL.Duration)
		}
	}
	t.ttl = &TTL{Duration: newTTL.Duration}
}
