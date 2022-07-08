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
	"sync"
	"time"

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
)

var _time ttime.Time = &ttime.DefaultTime{}

type EventFlowController struct {
	eventControlLock sync.RWMutex
	flowControl      map[string]chan bool
}

func NewEventFlowController() *EventFlowController {
	return &EventFlowController{
		flowControl: make(map[string]chan bool),
	}
}

// RetryWithBackoff takes a Backoff and a function to call that returns an error
// If the error is nil then the function will no longer be called
// If the error is Retriable then that will be used to determine if it should be
// retried
func RetryWithBackoff(backoff Backoff, fn func() error) error {
	return RetryWithBackoffCtx(context.Background(), backoff, fn)
}

func RetryWithBackoffNew(eventFlowController *EventFlowController, taskARN string, backoff Backoff, fn func() error) error {
	return RetryWithBackoffCtxNew(eventFlowController, taskARN, context.Background(), backoff, fn)
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

func RetryWithBackoffCtxNew(eventFlowController *EventFlowController, taskARN string, ctx context.Context, backoff Backoff, fn func() error) error {

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

		if !config.GetDisconnectModeEnabled() {
			waitForDuration(backoff.Duration())
		} else {
			logger.Debug("acquire lock to create a channel")
			eventFlowController.eventControlLock.Lock()
			logger.Debug("acquired lock to create a channel")
			if _, ok := eventFlowController.flowControl[taskARN]; !ok {
				logger.Debug("creating channel for")
				logger.Debug(taskARN)
				eventFlowController.flowControl[taskARN] = make(chan bool, 1)
			}
			logger.Debug("releasing lock to create a channel")
			eventFlowController.eventControlLock.Unlock()
			logger.Debug("released lock to create a channel")

			eventFlowController.eventControlLock.RLock()
			taskChannel := eventFlowController.flowControl[taskARN]
			eventFlowController.eventControlLock.RUnlock()

			waitForDurationAndInterruptIfRequired(10*time.Minute, taskChannel)
			logger.Debug("acquire lock to delete a channel")
			eventFlowController.eventControlLock.Lock()
			logger.Debug("acquired lock to delete a channel")
			if _, ok := eventFlowController.flowControl[taskARN]; ok {
				logger.Debug("closing channel for taskArn")
				logger.Debug(taskARN)
				close(eventFlowController.flowControl[taskARN])
			}
			logger.Debug("deleting channel for taskArn")
			logger.Debug(taskARN)
			delete(eventFlowController.flowControl, taskARN)
			logger.Debug("releasing lock to delete a channel")
			eventFlowController.eventControlLock.Unlock()
			logger.Debug("released lock to delete a channel")
		}
	}
}

func DoResumeEventsFlow(eventFlowController *EventFlowController) {
	logger.Debug("acquire lock to resume events flow")
	eventFlowController.eventControlLock.Lock()
	logger.Debug("acquired lock to resume events flow")

	for arn := range eventFlowController.flowControl {
		logger.Debug("resuming flow for arn")
		logger.Debug(arn)
		if len(eventFlowController.flowControl[arn]) == 0 {
			eventFlowController.flowControl[arn] <- true
		}
	}
	logger.Debug("releasing lock to resume events flow")
	eventFlowController.eventControlLock.Unlock()
	logger.Debug("released lock to resume events flow")
}

func waitForDuration(delay time.Duration) bool {
	reconnectTimer := time.NewTimer(delay)
	logger.Debug("Waiting for")
	logger.Debug(delay.String())
	select {
	case <-reconnectTimer.C:
		logger.Debug("Finished waiting for")
		logger.Debug(delay.String())
		return true
	}
}

func waitForDurationAndInterruptIfRequired(delay time.Duration, resumeEventsFlow <-chan bool) bool {
	reconnectTimer := time.NewTimer(delay)
	logger.Debug("Waiting for")
	logger.Debug(delay.String())
	select {
	case <-reconnectTimer.C:
		logger.Debug("Finished waiting for")
		logger.Debug(delay.String())
		return true
	case <-resumeEventsFlow:
		logger.Debug("Interrupt wait as connection resumed")
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
