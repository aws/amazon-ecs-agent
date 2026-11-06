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
	"testing"
	"time"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime"
	mock_ttime "github.com/aws/amazon-ecs-agent/ecs-agent/utils/ttime/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRetryWithBackoff(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mocktime := mock_ttime.NewMockTime(ctrl)
	_time = mocktime
	defer func() { _time = &ttime.DefaultTime{} }()

	t.Run("retries", func(t *testing.T) {
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(3)
		counter := 3
		RetryWithBackoff(NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), func() error {
			if counter == 0 {
				return nil
			}
			counter--
			return errors.New("err")
		})
		assert.Equal(t, 0, counter, "Counter didn't go to 0; didn't get retried enough")
	})

	t.Run("no retries", func(t *testing.T) {
		// no sleeps
		RetryWithBackoff(NewExponentialBackoff(10*time.Second, 20*time.Second, 0, 2), func() error {
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), errors.New("can't retry"))
		})
	})
}

func TestRetryWithBackoffCtx(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mocktime := mock_ttime.NewMockTime(ctrl)
	_time = mocktime
	defer func() { _time = &ttime.DefaultTime{} }()

	t.Run("retries", func(t *testing.T) {
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(3)
		counter := 3
		RetryWithBackoffCtx(context.TODO(), NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), func() error {
			if counter == 0 {
				return nil
			}
			counter--
			return errors.New("err")
		})
		assert.Equal(t, 0, counter, "Counter didn't go to 0; didn't get retried enough")
	})

	t.Run("no retries", func(t *testing.T) {
		// no sleeps
		RetryWithBackoffCtx(context.TODO(), NewExponentialBackoff(10*time.Second, 20*time.Second, 0, 2), func() error {
			return apierrors.NewRetriableError(apierrors.NewRetriable(false), errors.New("can't retry"))
		})
	})

	t.Run("cancel context", func(t *testing.T) {
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(2)
		counter := 2
		ctx, cancel := context.WithCancel(context.TODO())
		RetryWithBackoffCtx(ctx, NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), func() error {
			counter--
			if counter == 0 {
				cancel()
			}
			return errors.New("err")
		})
		assert.Equal(t, 0, counter, "Counter not 0; went the wrong number of times")
	})

}

func TestRetryWithBackoffCtxContextDone(t *testing.T) {
	attemptErr := errors.New("attempt failed")
	testCases := []struct {
		name          string
		cancelBefore  bool
		attemptErr    error
		expectedCalls int
		expectedErrs  []error
	}{
		{
			name:          "cancelled before first attempt",
			cancelBefore:  true,
			expectedCalls: 0,
			expectedErrs:  []error{context.Canceled},
		},
		{
			name:          "cancelled after failed attempt",
			attemptErr:    attemptErr,
			expectedCalls: 1,
			expectedErrs:  []error{context.Canceled, attemptErr},
		},
		{
			name:          "cancelled after retriable failed attempt",
			attemptErr:    apierrors.NewRetriableError(apierrors.NewRetriable(true), attemptErr),
			expectedCalls: 1,
			expectedErrs:  []error{context.Canceled, attemptErr},
		},
		{
			name:          "cancelled after successful attempt",
			attemptErr:    nil,
			expectedCalls: 1,
			expectedErrs:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mocktime := mock_ttime.NewMockTime(ctrl)
			_time = mocktime
			defer func() { _time = &ttime.DefaultTime{} }()
			mocktime.EXPECT().Sleep(gomock.Any()).AnyTimes()

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			if tc.cancelBefore {
				cancel()
			}
			calls := 0
			err := RetryWithBackoffCtx(ctx, NewExponentialBackoff(time.Millisecond, time.Millisecond, 0, 1), func() error {
				calls++
				cancel()
				return tc.attemptErr
			})

			assert.Equal(t, tc.expectedCalls, calls)
			if tc.expectedErrs == nil {
				assert.NoError(t, err)
			}
			for _, expected := range tc.expectedErrs {
				assert.ErrorIs(t, err, expected)
			}
		})
	}
}

func TestRetryNWithBackoff(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mocktime := mock_ttime.NewMockTime(ctrl)
	_time = mocktime
	defer func() { _time = &ttime.DefaultTime{} }()

	t.Run("count exceeded", func(t *testing.T) {
		// 2 tries, 1 sleep
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(1)
		counter := 3
		err := RetryNWithBackoff(NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 2, func() error {
			counter--
			return errors.New("err")
		})
		assert.Equal(t, 1, counter, "Should have stopped after two tries")
		assert.Error(t, err)
	})

	t.Run("retry succeeded", func(t *testing.T) {
		// 3 tries, 2 sleeps
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(2)
		counter := 3
		err := RetryNWithBackoff(NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 5, func() error {
			counter--
			if counter == 0 {
				return nil
			}
			return errors.New("err")
		})
		assert.Equal(t, 0, counter)
		assert.NoError(t, err)
	})
}

func TestRetryNWithBackoffCtx(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mocktime := mock_ttime.NewMockTime(ctrl)
	_time = mocktime
	defer func() { _time = &ttime.DefaultTime{} }()

	t.Run("count exceeded", func(t *testing.T) {
		// 2 tries, 1 sleep
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(1)
		counter := 3
		err := RetryNWithBackoffCtx(context.TODO(), NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 2, func() error {
			counter--
			return errors.New("err")
		})
		assert.Equal(t, 1, counter, "Should have stopped after two tries")
		assert.Error(t, err)
	})

	t.Run("retry succeeded", func(t *testing.T) {
		// 3 tries, 2 sleeps
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(2)
		counter := 3
		err := RetryNWithBackoffCtx(context.TODO(), NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 5, func() error {
			counter--
			if counter == 0 {
				return nil
			}
			return errors.New("err")
		})
		assert.Equal(t, 0, counter)
		assert.NoError(t, err)
	})

	t.Run("cancel context", func(t *testing.T) {
		// 2 tries, 2 sleeps
		mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(2)
		counter := 3
		ctx, cancel := context.WithCancel(context.TODO())
		err := RetryNWithBackoffCtx(ctx, NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 5, func() error {
			counter--
			if counter == 1 {
				cancel()
			}
			return errors.New("err")
		})
		assert.Equal(t, 1, counter, "Should have stopped after two tries")
		assert.Error(t, err)
	})
}

func TestRetryNWithBackoffCtxContextDone(t *testing.T) {
	attemptErr := errors.New("attempt failed")
	testCases := []struct {
		name          string
		cancelBefore  bool
		cancelAfter   int
		tries         int
		expectedCalls int
		expectedErrs  []error
		notExpected   []error
	}{
		{
			name:          "cancelled before first attempt",
			cancelBefore:  true,
			tries:         3,
			expectedCalls: 0,
			expectedErrs:  []error{context.Canceled},
			notExpected:   []error{attemptErr},
		},
		{
			name:          "cancelled after second attempt",
			cancelAfter:   2,
			tries:         5,
			expectedCalls: 2,
			expectedErrs:  []error{context.Canceled, attemptErr},
		},
		{
			name:          "tries exhausted before cancellation",
			tries:         2,
			expectedCalls: 2,
			expectedErrs:  []error{attemptErr},
			notExpected:   []error{context.Canceled},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mocktime := mock_ttime.NewMockTime(ctrl)
			_time = mocktime
			defer func() { _time = &ttime.DefaultTime{} }()
			mocktime.EXPECT().Sleep(gomock.Any()).AnyTimes()

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			if tc.cancelBefore {
				cancel()
			}
			calls := 0
			err := RetryNWithBackoffCtx(ctx, NewExponentialBackoff(time.Millisecond, time.Millisecond, 0, 1), tc.tries, func() error {
				calls++
				if calls == tc.cancelAfter {
					cancel()
				}
				return attemptErr
			})

			assert.Equal(t, tc.expectedCalls, calls)
			for _, expected := range tc.expectedErrs {
				assert.ErrorIs(t, err, expected)
			}
			for _, notExpected := range tc.notExpected {
				assert.NotErrorIs(t, err, notExpected)
			}
		})
	}
}
