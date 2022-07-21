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

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
)

var _time ttime.Time = &ttime.DefaultTime{}

// RetryWithBackoff takes a Backoff and a function to call that returns an error
// If the error is nil then the function will no longer be called
// If the error is Retriable then that will be used to determine if it should be
// retried
func RetryWithBackoff(backoff Backoff, fn func() error) error {
	return RetryWithBackoffCtx(context.Background(), backoff, fn)
}

func RetryWithBackoffForTaskHandler(cfg *config.Config, taskARN string, delay time.Duration, backoff Backoff, eventFlowCtx *context.Context, fn func() error) error {
	return RetryWithBackoffCtxForTaskHandler(context.Background(), eventFlowCtx, cfg, taskARN, backoff, delay, fn)
}

// RetryWithBackoffCtx takes a context, a Backoff, and a function to call that returns an error
// If the context is done, nil will be returned
// If the error is nil then the function will no longer be called
// If the error is Retriable then that will be used to determine if it should be
// retried
func RetryWithBackoffCtx(ctx context.Context, backoff Backoff, fn func() error) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		err = fn()

		retriableErr, isRetriableErr := err.(apierrors.Retriable)

		if err == nil || (isRetriableErr && !retriableErr.Retry()) {
			return err
		}

		_time.Sleep(backoff.Duration())
	}
}

func RetryWithBackoffCtxForTaskHandler(ctx context.Context, eventFlowCtx *context.Context, cfg *config.Config, taskARN string, backoff Backoff, delay time.Duration, fn func() error) error {

	var err error
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		err = fn()

		retriableErr, isRetriableErr := err.(apierrors.Retriable)

		//if unretriable error in disconnected mode, don't return
		if err == nil || (!cfg.GetDisconnectModeEnabled() && isRetriableErr && !retriableErr.Retry()) {
			return err
		}

		/*
			If we were in normal mode up to this point and switch to disconnected mode here,
			we enter the if block

			If we were in disconnected mode up to this point and switch to normal mode here,
			eventFlowCtx is cancelled, and we enter else block
		*/
		if cfg.GetDisconnectModeEnabled() && *eventFlowCtx != nil {

			/*
				If we switch to normal mode while we are in this portion before calling WaitForDurationAndInterruptIfRequired,
				there is a chance that we receive on a cancelled context- but this returns immediately
			*/
			WaitForDurationWithContext(eventFlowCtx, delay)
		} else {

			/*
				If we switch to disconnected mode at this point, eventFlowCtx is initialized, no effect
			*/

			waitForDuration(backoff.Duration())
		}

		/*
			If we were in disconnected mode up to this point and switch to normal mode here,
			eventFlowCtx context is cancelled, but this has no effect

			If we were in normal mode up to this point and switch to disconnected mode here,
			eventFlowCtx is initialzed, but this has no effect
		*/

	}
}

func waitForDuration(delay time.Duration) bool {
	backgroundCtx := context.Background()
	return WaitForDurationWithContext(&backgroundCtx, delay)
}

func WaitForDurationWithContext(eventFlowCtx *context.Context, delay time.Duration) bool {
	reconnectTimer := time.NewTimer(delay)
	logger.Debug("Started wait", logger.Fields{
		"waitDelay": delay.String(),
	})
	select {
	case <-reconnectTimer.C:
		logger.Debug("Finished wait", logger.Fields{
			"waitDelay": delay.String(),
		})
		return true

	case <-(*eventFlowCtx).Done():
		logger.Debug("Interrupt wait as connection resumed")
		if !reconnectTimer.Stop() { //prevents memory leak
			<-reconnectTimer.C
		}
		return true
	}
}

// RetryNWithBackoff takes a Backoff, a maximum number of tries 'n', and a
// function that returns an error. The function is called until either it does
// not return an error or the maximum tries have been reached.
// If the error returned is Retriable, the Retriability of it will be respected.
// If the number of tries is exhausted, the last error will be returned.
func RetryNWithBackoff(backoff Backoff, n int, fn func() error) error {
	return RetryNWithBackoffCtx(context.Background(), backoff, n, fn)
}

// RetryNWithBackoffCtx takes a context, a Backoff, a maximum number of tries 'n', and a function that returns an error.
// The function is called until it does not return an error, the context is done, or the maximum tries have been
// reached.
// If the error returned is Retriable, the Retriability of it will be respected.
// If the number of tries is exhausted, the last error will be returned.
func RetryNWithBackoffCtx(ctx context.Context, backoff Backoff, n int, fn func() error) error {
	var err error
	RetryWithBackoffCtx(ctx, backoff, func() error {
		err = fn()
		n--
		if n == 0 {
			// Break out after n tries
			return nil
		}
		return err
	})
	return err
}
