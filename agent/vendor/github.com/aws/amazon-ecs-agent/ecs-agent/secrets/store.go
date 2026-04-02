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

// Package secrets provides thread-safe in-memory stores for secrets
// keyed by secret ID. It exposes domain-specific interfaces
// (SplunkTokenStore, ContainerEnvStore) backed by a single unexported
// generic implementation.
package secrets

import (
	"errors"
	"sync"
)

// store is a thread-safe key-value store.
// For value types (e.g. string), store can be used directly.
// For reference types (e.g. maps, slices), callers must wrap store and copy
// values on Set and Get to prevent external mutation of stored data.
type store[T any] struct {
	data map[string]T
	mu   sync.RWMutex
}

func newStore[T any]() *store[T] {
	return &store[T]{
		data: make(map[string]T),
	}
}

// Set stores a value for the given id. Returns an error if id is empty.
func (s *store[T]) Set(id string, value T) error {
	if id == "" {
		return errors.New("secret ID must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = value
	return nil
}

// Get retrieves the value for the given id. Returns the value and true if
// found, or the zero value and false if not found.
func (s *store[T]) Get(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[id]
	return val, ok
}

// Remove deletes the value for the given id.
func (s *store[T]) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
}
