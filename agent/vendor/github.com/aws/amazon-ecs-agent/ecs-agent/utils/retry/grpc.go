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
	"time"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsRetriableGRPCError returns true for transient gRPC errors (Unavailable,
// DeadlineExceeded, ResourceExhausted, Aborted, Internal) and false for
// terminal ones (InvalidArgument, PermissionDenied, etc.).
func IsRetriableGRPCError(err error) bool {
	if err == nil {
		return false
	}
	s, _ := status.FromError(err)
	// Retriable codes are based on the gRPC status code guidance at
	// https://grpc.io/docs/guides/status-codes/
	switch s.Code() {
	case codes.Unavailable,
		codes.DeadlineExceeded,
		codes.ResourceExhausted,
		codes.Aborted,
		codes.Internal:
		return true
	default:
		return false
	}
}

// CallWithRetry calls fn with retries on transient gRPC errors, stopping
// immediately on terminal ones. The timeout is the total budget across all
// attempts — each attempt gets an equal share of it. Returns the result,
// the number of attempts made, and any error.
func CallWithRetry[T any](ctx context.Context, backoff Backoff, attempts int, timeout time.Duration, fn func(context.Context) (T, error)) (T, int, error) {
	var zero T
	var result T
	var lastErr error
	attemptsMade := 0

	perAttemptTimeout := timeout / time.Duration(attempts)

	RetryNWithBackoffCtx(ctx, backoff, attempts, func() error {
		callCtx, cancel := context.WithTimeout(ctx, perAttemptTimeout)
		defer cancel()

		attemptsMade++
		r, err := fn(callCtx)
		lastErr = err
		if err != nil {
			if IsRetriableGRPCError(err) {
				return apierrors.NewRetriableError(apierrors.NewRetriable(true), err)
			}
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), err)
		}
		result = r
		return nil
	})

	if lastErr != nil {
		return zero, attemptsMade, lastErr
	}
	return result, attemptsMade, nil
}
