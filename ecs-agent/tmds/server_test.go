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
package tmds

import (
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerErrors(t *testing.T) {
	t.Run("handler is required", func(t *testing.T) {
		_, err := NewServer(nil, WithListenAddress(AddressIPv4()))
		assert.EqualError(t, err, "handler cannot be nil")
	})
}

// Asserts that server-level settings passed to NewServer() function make their way to
// the initialized server.
func TestServerSettings(t *testing.T) {
	router := mux.NewRouter()
	writeTimeout := 5 * time.Second
	readTimeout := 10 * time.Second

	server, err := NewServer(nil,
		WithListenAddress(AddressIPv4()),
		WithHandler(router),
		WithWriteTimeout(writeTimeout),
		WithReadTimeout(readTimeout))

	require.NoError(t, err)
	assert.Equal(t, AddressIPv4(), server.Addr)
	assert.Equal(t, writeTimeout, server.WriteTimeout)
	assert.Equal(t, readTimeout, server.ReadTimeout)
}

func TestAddressIPv4(t *testing.T) {
	assert.Equal(t, "127.0.0.1:51679", AddressIPv4())
}
