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

package v1

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that errors.As() function works with ErrorNotFound errors.
func TestAsErrorNotFound(t *testing.T) {
	t.Run("as works no wrap", func(t *testing.T) {
		var target *ErrorNotFound
		var err = NewErrorNotFound("reason")

		require.True(t, errors.As(err, &target))
		assert.Equal(t, err, target)
	})
	t.Run("as works wrapped", func(t *testing.T) {
		var target *ErrorNotFound
		var err = NewErrorNotFound("reason")

		require.True(t, errors.As(errors.Wrap(err, "outer"), &target))
		assert.Equal(t, err, target)
	})
	t.Run("as should fail when no match", func(t *testing.T) {
		var target *ErrorNotFound
		require.False(t, errors.As(errors.New("other error"), &target))
	})
}

// Tests that errors.As() function works with ErrorFetchFailure errors.
func TestAsErrorFetchFailure(t *testing.T) {
	t.Run("as works no wrap", func(t *testing.T) {
		var target *ErrorFetchFailure
		var err = NewErrorFetchFailure("containerID")

		require.True(t, errors.As(err, &target))
		assert.Equal(t, err, target)
	})
	t.Run("as works wrapped", func(t *testing.T) {
		var target *ErrorFetchFailure
		var err = NewErrorFetchFailure("containerID")

		require.True(t, errors.As(errors.Wrap(err, "outer"), &target))
		assert.Equal(t, err, target)
	})
	t.Run("as should fail when no match", func(t *testing.T) {
		var target *ErrorFetchFailure
		require.False(t, errors.As(errors.New("other error"), &target))
	})
}

// Tests that Error returns the string provided when the error is initialized.
func TestError(t *testing.T) {
	t.Run("external reason is set", func(t *testing.T) {
		var err = NewErrorFetchFailure("containerID")

		assert.Equal(t, "containerID", err.Error())
	})
	t.Run("external reason is not set", func(t *testing.T) {
		var err = NewErrorFetchFailure("")

		assert.Equal(t, "", err.Error())
	})
}

func TestMetricName(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		var err = NewErrorNotFound("")
		assert.Equal(t, metrics.IntrospectionNotFound, err.MetricName())
	})
	t.Run("fetch failure", func(t *testing.T) {
		var err = NewErrorFetchFailure("")
		assert.Equal(t, metrics.IntrospectionFetchFailure, err.MetricName())
	})
}
