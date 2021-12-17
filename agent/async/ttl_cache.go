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
	// Get fetches a value from cache, returns nil, false on miss
	Get(key string) (value interface{}, expired bool, ok bool)
	// Set sets a value in cache. overrites any existing value
	Set(key string, value interface{})
	// Delete deletes the value from the cache
	Delete(key string)
}

// Creates a TTL cache with ttl for items.
func NewTTLCache(ttl time.Duration) TTLCache {
	return &ttlCache{
		ttl:   ttl,
		cache: make(map[string]*ttlCacheEntry),
	}
}

type ttlCacheEntry struct {
	value  interface{}
	expiry time.Time
}

type ttlCache struct {
	mu    sync.RWMutex
	cache map[string]*ttlCacheEntry
	ttl   time.Duration
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
	expired = time.Now().After(entry.expiry)
	return entry.value, expired, true
}

// Set sets the key-value pair in the cache
func (t *ttlCache) Set(key string, value interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cache[key] = &ttlCacheEntry{
		value:  value,
		expiry: time.Now().Add(t.ttl),
	}
}

// Delete removes the entry associated with the key from cache
func (t *ttlCache) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cache, key)
}
