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
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that errors.As() function works with ErrorLookupFailure, ErrorMetadataFetchFailure, ErrorStatsFetchFailure, and ErrorStatsLookupFailure errors
func TestAsErrorFailures(t *testing.T) {
	tests := []struct {
		name   string
		target interface{}
		err    error
	}{
		{
			name:   "Lookup Error with no wrap",
			target: new(*ErrorLookupFailure),
			err:    NewErrorLookupFailure("reason"),
		},
		{
			name:   "Lookup Error with wrap",
			target: new(*ErrorLookupFailure),
			err:    errors.Wrap(NewErrorLookupFailure("reason"), "outer"),
		},
		{
			name:   "Metadata Fetch Error with no wrap",
			target: new(*ErrorMetadataFetchFailure),
			err:    NewErrorMetadataFetchFailure("containerID"),
		},
		{
			name:   "Metadata Fetch Error with wrap",
			target: new(*ErrorMetadataFetchFailure),
			err:    errors.Wrap(NewErrorMetadataFetchFailure("containerID"), "outer"),
		},
		{
			name:   "Stats Fetch Error with no wrap",
			target: new(*ErrorStatsFetchFailure),
			err:    NewErrorStatsFetchFailure("containerID", errors.New("cause")),
		},
		{
			name:   "Stats Fetch Error with wrap",
			target: new(*ErrorStatsFetchFailure),
			err:    errors.Wrap(NewErrorStatsFetchFailure("reason", nil), "outer"),
		},
		{
			name:   "Stats Lookup Error with no wrap",
			target: new(*ErrorStatsLookupFailure),
			err:    NewErrorStatsLookupFailure("containerID"),
		},
		{
			name:   "Stats Lookup Error with wrap",
			target: new(*ErrorStatsLookupFailure),
			err:    errors.Wrap(NewErrorStatsLookupFailure("reason"), "outer"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, errors.As(tt.err, tt.target))
		})
	}
}

func TestAsErrorNoMatch(t *testing.T) {
	tests := []struct {
		name   string
		target interface{}
	}{
		{
			name:   "Lookup failure: as should fail when no match",
			target: new(*ErrorLookupFailure),
		},
		{
			name:   "Metadata fetch failure: as should fail when no match",
			target: new(*ErrorMetadataFetchFailure),
		},
		{
			name:   "Stats fetch failure: as should fail when no match",
			target: new(*ErrorStatsFetchFailure),
		},
		{
			name:   "Stats lookup failure: as should fail when no match",
			target: new(*ErrorStatsLookupFailure),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, errors.As(errors.New("other error"), tt.target))
		})
	}
}

func TestUnwrapErrorStatsFetchFailure(t *testing.T) {
	t.Run("unwrap works", func(t *testing.T) {
		cause := errors.New("cause")
		var err error = NewErrorStatsFetchFailure("external reason", cause)
		assert.Equal(t, cause, errors.Unwrap(err))
	})
	t.Run("unwrap with no cause", func(t *testing.T) {
		var err error = NewErrorStatsFetchFailure("external reason", nil)
		assert.Nil(t, errors.Unwrap(err))
	})
}

// Tests Error() method of ErrorStatsFetchFailure type
func TestErrorStatsFetchFailureMessage(t *testing.T) {
	var err error = NewErrorStatsFetchFailure("external reason", errors.New("cause"))
	assert.Equal(t, "failed to get stats: external reason: cause", err.Error())
}

func TestErrorDefaultNetworkInterface_Is(t *testing.T) {
	baseErr := errors.New("base error")
	networkErr := NewErrorDefaultNetworkInterface(baseErr)
	wrappedErr := fmt.Errorf("outer error: %w", networkErr)

	tests := []struct {
		name     string
		err      error
		target   error
		expected bool
	}{
		{
			name:     "can detect wrapped ErrorDefaultNetworkInterface",
			err:      wrappedErr,
			target:   &ErrorDefaultNetworkInterface{},
			expected: true,
		},
		{
			name:     "can detect base error through wrapping",
			err:      wrappedErr,
			target:   baseErr,
			expected: true,
		},
		{
			name:     "different error type",
			err:      errors.New("some other error"),
			target:   &ErrorDefaultNetworkInterface{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errors.Is(tt.err, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorDefaultNetworkInterface_As(t *testing.T) {
	baseErr := errors.New("base error")
	networkErr := NewErrorDefaultNetworkInterface(baseErr)
	wrappedErr := fmt.Errorf("outer error: %w", networkErr)

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "can extract wrapped ErrorDefaultNetworkInterface",
			err:      wrappedErr,
			expected: true,
		},
		{
			name:     "can extract ErrorDefaultNetworkInterface directly",
			err:      networkErr,
			expected: true,
		},
		{
			name:     "different error type",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *ErrorDefaultNetworkInterface
			result := errors.As(tt.err, &target)
			assert.Equal(t, tt.expected, result)
		})
	}
}
