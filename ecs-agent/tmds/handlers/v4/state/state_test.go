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
package state

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that errors.As() function works with ErrorLookupFailure errors
func TestAsErrorLookupFailure(t *testing.T) {
	t.Run("as works no wrap", func(t *testing.T) {
		var target *ErrorLookupFailure
		var err = NewErrorLookupFailure("reason")

		require.True(t, errors.As(err, &target))
		assert.Equal(t, err, target)
	})
	t.Run("as works wrapped", func(t *testing.T) {
		var target *ErrorLookupFailure
		var err = NewErrorLookupFailure("reason")

		require.True(t, errors.As(errors.Wrap(err, "outer"), &target))
		assert.Equal(t, err, target)
	})
	t.Run("as should fail when no match", func(t *testing.T) {
		var target *ErrorLookupFailure
		require.False(t, errors.As(errors.New("other error"), &target))
	})
}

// Tests that errors.As() function works with ErrorMetadataFetchFailure errors
func TestAsErrorMetadataFetchFailure(t *testing.T) {
	t.Run("as works no wrap", func(t *testing.T) {
		var target *ErrorMetadataFetchFailure
		var err = NewErrorMetadataFetchFailure("containerID")

		require.True(t, errors.As(err, &target))
		assert.Equal(t, err, target)
	})
	t.Run("as works wrapped", func(t *testing.T) {
		var target *ErrorMetadataFetchFailure
		var err = NewErrorMetadataFetchFailure("containerID")

		require.True(t, errors.As(errors.Wrap(err, "outer"), &target))
		assert.Equal(t, err, target)
	})
	t.Run("as should fail when no match", func(t *testing.T) {
		var target *ErrorMetadataFetchFailure
		require.False(t, errors.As(errors.New("other error"), &target))
	})
}
