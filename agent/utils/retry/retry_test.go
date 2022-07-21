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
	"fmt"
	"strconv"
	"testing"
	"time"

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	mock_ttime "github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
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

func TestRetryWithBackoffCtxForTaskHandler(t *testing.T) {

	for _, tc := range []struct {
		disconnectModeEnabled bool
	}{
		{
			disconnectModeEnabled: true,
		},
		{
			disconnectModeEnabled: false,
		},
	} {

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mocktime := mock_ttime.NewMockTime(ctrl)
		_time = mocktime
		defer func() { _time = &ttime.DefaultTime{} }()

		cfg := &config.Config{}
		cfg.SetDisconnectModeEnabled(tc.disconnectModeEnabled)

		t.Run(fmt.Sprintf("retries, disconnected %s", strconv.FormatBool(tc.disconnectModeEnabled)), func(t *testing.T) {
			counter := 3
			eventFlwCtx := context.TODO()
			RetryWithBackoffCtxForTaskHandler(context.TODO(), &eventFlwCtx, cfg, "myArn", NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 200*time.Millisecond, func() error {
				if counter == 0 {
					return nil
				}
				counter--
				return errors.New("err")
			})
			assert.Equal(t, 0, counter, "Counter didn't go to 0; didn't get retried enough")
		})

		t.Run(fmt.Sprintf("no retries, disconnected %s", strconv.FormatBool(tc.disconnectModeEnabled)), func(t *testing.T) {
			counter := 3
			eventFlwCtx := context.TODO()
			RetryWithBackoffCtxForTaskHandler(context.TODO(), &eventFlwCtx, cfg, "myArn", NewExponentialBackoff(10*time.Second, 20*time.Second, 0, 2), 200*time.Millisecond, func() error {
				if counter == 0 {
					return nil
				}
				counter--
				return apierrors.NewRetriableError(apierrors.NewRetriable(false), errors.New("can't retry"))
			})

			if tc.disconnectModeEnabled {
				assert.Equal(t, 0, counter, "Counter not 0; went the wrong number of times")
			} else {
				assert.Equal(t, 2, counter, "Counter not 2; went the wrong number of times")
			}
		})

		t.Run(fmt.Sprintf("cancel context, disconnected %s", strconv.FormatBool(tc.disconnectModeEnabled)), func(t *testing.T) {
			counter := 2
			ctx, cancel := context.WithCancel(context.TODO())
			eventFlwCtx := context.TODO()
			RetryWithBackoffCtxForTaskHandler(ctx, &eventFlwCtx, cfg, "myArn", NewExponentialBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 200*time.Millisecond, func() error {
				counter--
				if counter == 0 {
					cancel()
				}
				return errors.New("err")
			})
			assert.Equal(t, 0, counter, "Counter not 0; went the wrong number of times")
		})
	}

}

func TestWaitForDurationWithContext(t *testing.T) {

	for _, tc := range []struct {
		interruptTimer bool
		delay          time.Duration
	}{
		{
			interruptTimer: true,
			delay:          10 * time.Minute,
		},
		{
			interruptTimer: false,
			delay:          100 * time.Millisecond,
		},
	} {

		t.Run(fmt.Sprintf("interrupt timer %s", strconv.FormatBool(tc.interruptTimer)), func(t *testing.T) {

			eventFlowCtx, eventFlowCtxCancel := context.WithCancel(context.Background())

			if tc.interruptTimer {
				interruptAfter := time.NewTimer(200 * time.Millisecond)
				go func() {
					<-interruptAfter.C
					eventFlowCtxCancel()
				}()
			}
			interrupt := WaitForDurationWithContext(&eventFlowCtx, tc.delay)
			assert.True(t, interrupt, "Timer not interrupted")
		})

	}

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
