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

package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsRetriableGRPCError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retriable bool
	}{
		{"nil error", nil, false},
		{"Unavailable", status.Errorf(codes.Unavailable, "unavailable"), true},
		{"DeadlineExceeded", status.Errorf(codes.DeadlineExceeded, "deadline"), true},
		{"ResourceExhausted", status.Errorf(codes.ResourceExhausted, "exhausted"), true},
		{"Aborted", status.Errorf(codes.Aborted, "aborted"), true},
		{"Internal", status.Errorf(codes.Internal, "internal"), true},
		{"InvalidArgument", status.Errorf(codes.InvalidArgument, "bad arg"), false},
		{"PermissionDenied", status.Errorf(codes.PermissionDenied, "denied"), false},
		{"NotFound", status.Errorf(codes.NotFound, "not found"), false},
		{"non-gRPC error", errors.New("plain error"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retriable, IsRetriableGRPCError(tt.err))
		})
	}
}

func TestCallWithRetry(t *testing.T) {
	fastBackoff := NewExponentialBackoff(time.Nanosecond, time.Nanosecond, 0, 1)

	t.Run("succeeds on first attempt", func(t *testing.T) {
		result, attempts, err := CallWithRetry(context.Background(), fastBackoff, 3, time.Minute,
			func(_ context.Context) (string, error) {
				return "ok", nil
			})
		require.NoError(t, err)
		assert.Equal(t, "ok", result)
		assert.Equal(t, 1, attempts)
	})

	t.Run("retries on transient error and succeeds", func(t *testing.T) {
		var callCount atomic.Int32
		result, attempts, err := CallWithRetry(context.Background(), fastBackoff, 3, time.Minute,
			func(_ context.Context) (string, error) {
				if callCount.Add(1) < 3 {
					return "", status.Errorf(codes.Unavailable, "transient")
				}
				return "ok", nil
			})
		require.NoError(t, err)
		assert.Equal(t, "ok", result)
		assert.Equal(t, 3, attempts)
	})

	t.Run("exhausts retries and returns last error", func(t *testing.T) {
		var callCount atomic.Int32
		_, attempts, err := CallWithRetry(context.Background(), fastBackoff, 3, time.Minute,
			func(_ context.Context) (string, error) {
				callCount.Add(1)
				return "", status.Errorf(codes.Unavailable, "always unavailable")
			})
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
		assert.Equal(t, 3, attempts)
		assert.Equal(t, int32(3), callCount.Load())
	})

	t.Run("does not retry non-gRPC error", func(t *testing.T) {
		var callCount atomic.Int32
		_, attempts, err := CallWithRetry(context.Background(), fastBackoff, 3, time.Minute,
			func(_ context.Context) (string, error) {
				callCount.Add(1)
				return "", errors.New("plain error")
			})
		require.Error(t, err)
		assert.Equal(t, "plain error", err.Error())
		assert.Equal(t, 1, attempts)
		assert.Equal(t, int32(1), callCount.Load(), "non-gRPC error should not be retried")
	})

	t.Run("does not retry terminal error", func(t *testing.T) {
		var callCount atomic.Int32
		_, attempts, err := CallWithRetry(context.Background(), fastBackoff, 3, time.Minute,
			func(_ context.Context) (string, error) {
				callCount.Add(1)
				return "", status.Errorf(codes.PermissionDenied, "denied")
			})
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Equal(t, 1, attempts)
		assert.Equal(t, int32(1), callCount.Load(), "terminal error should not be retried")
	})

	t.Run("context cancellation stops retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var callCount atomic.Int32
		_, _, err := CallWithRetry(ctx, fastBackoff, 3, time.Minute,
			func(_ context.Context) (string, error) {
				if callCount.Add(1) == 1 {
					cancel()
				}
				return "", status.Errorf(codes.Unavailable, "transient")
			})
		// lastErr is preserved even when context is cancelled
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})
}
