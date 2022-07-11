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

type TaskEventsFlowController struct {
	flowControl      map[string]chan bool
	eventControlLock sync.RWMutex
}

func NewEventFlowController() *TaskEventsFlowController {
	return &TaskEventsFlowController{
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

func RetryWithBackoffForTaskHandler(eventFlowController *TaskEventsFlowController, taskARN string, backoff Backoff, fn func() error) error {
	return RetryWithBackoffCtxForTaskHandler(eventFlowController, taskARN, context.Background(), backoff, fn)
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

func RetryWithBackoffCtxForTaskHandler(eventFlowController *TaskEventsFlowController, taskARN string, ctx context.Context, backoff Backoff, fn func() error) error {

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

		taskChannel := eventFlowController.createChannelForTask(taskARN)

		/*
			If we switch to disconnected mode after executing the previous code block,
			that means there is no channel initialized for the current ARN.
			Hence we don't use this nil channel

			If we were in disconnected mode up to this point and switch to normal mode here,
			we call waitForDuration and not waitForDurationAndInterruptIfRequired.
			Hence, message is sent on channel but it is never read.
		*/
		if config.GetDisconnectModeEnabled() && taskChannel != nil {
			waitForDurationAndInterruptIfRequired(10*time.Minute, taskChannel)
		} else {
			waitForDuration(backoff.Duration())
		}

		/*
			Note: If we were in disconnected mode up to this point and switch to normal mode here,
			message is sent on channel but it is never read.
			Hence, channel may have messages in it when it is closed and deleted from map
		*/
		eventFlowController.deleteChannelForTask(taskARN)

	}
}

func (eventFlowController *TaskEventsFlowController) deleteChannelForTask(taskARN string) {

	logger.Debug("Acquiring lock to delete a channel")
	eventFlowController.eventControlLock.Lock()
	logger.Debug("Acquired lock to delete a channel")
	if _, ok := eventFlowController.flowControl[taskARN]; ok {
		logger.Debug("Closing channel for taskArn")
		logger.Debug(taskARN)
		close(eventFlowController.flowControl[taskARN])
		logger.Debug("Deleting channel for taskArn")
		logger.Debug(taskARN)
		delete(eventFlowController.flowControl, taskARN)
	}

	logger.Debug("Releasing lock to delete a channel")
	eventFlowController.eventControlLock.Unlock()
	logger.Debug("Released lock to delete a channel")
}

func (eventFlowController *TaskEventsFlowController) createChannelForTask(taskARN string) chan bool {

	var taskChannel chan bool
	logger.Debug("Acquiring lock to create a channel")
	eventFlowController.eventControlLock.Lock()
	logger.Debug("Acquired lock to create a channel")
	if _, ok := eventFlowController.flowControl[taskARN]; !ok {
		logger.Debug("Creating channel for")
		logger.Debug(taskARN)

		//Checking disconnectModeEnabled here ensures that we create channel only in disconnected mode
		if config.GetDisconnectModeEnabled() {
			taskChannel = make(chan bool, 1)
			eventFlowController.flowControl[taskARN] = taskChannel
		}
	}
	logger.Debug("Releasing lock to create a channel")
	eventFlowController.eventControlLock.Unlock()
	logger.Debug("Released lock to create a channel")

	return taskChannel
}

func DoResumeEventsFlow(eventFlowController *TaskEventsFlowController) {
	logger.Debug("Acquiring lock to resume events flow")
	eventFlowController.eventControlLock.Lock()
	logger.Debug("Acquired lock to resume events flow")

	for arn := range eventFlowController.flowControl {
		logger.Debug("Resuming task events flow for arn")
		logger.Debug(arn)
		if len(eventFlowController.flowControl[arn]) == 0 {
			taskChannel := eventFlowController.flowControl[arn]
			if taskChannel != nil {
				eventFlowController.flowControl[arn] <- true
			}
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
