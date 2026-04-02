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

// ContainerEnvStore is a store for container environment variable maps
// keyed by secret ID.
type ContainerEnvStore interface {
	Set(secretID string, env map[string]string) error
	Get(secretID string) (map[string]string, bool)
	Remove(secretID string)
}

// containerEnvStore wraps the generic store and copies maps on Set and Get
// to ensure thread safety for map values.
type containerEnvStore struct {
	inner *store[map[string]string]
}

// NewContainerEnvStore returns a new ContainerEnvStore.
func NewContainerEnvStore() ContainerEnvStore {
	return &containerEnvStore{inner: newStore[map[string]string]()}
}

// Set stores a copy of the environment map for the given secret ID.
func (s *containerEnvStore) Set(secretID string, env map[string]string) error {
	cp := make(map[string]string, len(env))
	for k, v := range env {
		cp[k] = v
	}
	return s.inner.Set(secretID, cp)
}

// Get retrieves a copy of the environment map for the given secret ID.
func (s *containerEnvStore) Get(secretID string) (map[string]string, bool) {
	val, ok := s.inner.Get(secretID)
	if !ok {
		return nil, false
	}
	cp := make(map[string]string, len(val))
	for k, v := range val {
		cp[k] = v
	}
	return cp, true
}

// Remove deletes the environment map for the given secret ID.
func (s *containerEnvStore) Remove(secretID string) {
	s.inner.Remove(secretID)
}
