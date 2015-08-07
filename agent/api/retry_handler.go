// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	// submitStateChangeMaxDelayRetries is the initial set of retries where delay
	// between retries grows exponentially at 2^n * 30ms.  This should be roughly
	// 5 minutes of delay at the last growing retry.
	// 5 min = 2^n * 30 ms
	// 300000 ms = 2^n * 30 ms
	// 10000 ms = 2^n
	// n ~= 13.3
	submitStateChangeMaxDelayRetries = 14

	// submitStateChangeExtraRetries is the set of retries (at the max delay per
	// retry of roughly 5 minutes) which should reach roughly 24 hours of elapsed
	// time.
	// delay = 2^(14) * 30ms = 491520 ms ~= 8 minutes
	// baseTryTime = \sum_{i=0}^14{2^i * 30 ms} ~= 16 minutes
	// 24 hours ~= 16 minutes + (n * 8 minutes)
	// n ~= 178
	submitStateChangeExtraRetries = 178
)

// newSubmitStateChangeClient returns a client intended to be used for
// Submit*StateChange APIs which has the behavior of retrying the call on
// retriable errors for an extended period of time (roughly 24 hours).
func newSubmitStateChangeClient(awsConfig *aws.Config) *ecs.ECS {
	sscConfig := awsConfig.Copy()
	sscConfig.MaxRetries = submitStateChangeMaxDelayRetries
	client := ecs.New(&sscConfig)
	client.Handlers.AfterRetry.Clear()
	client.Handlers.AfterRetry.PushBack(
		extendedRetryMaxDelayHandlerFactory(submitStateChangeExtraRetries))
	client.DefaultMaxRetries = submitStateChangeMaxDelayRetries
	return client
}

var awsAfterRetryHandler = func(r *aws.Request) {
	aws.AfterRetryHandler(r)
}

// ExtendedRetryMaxDelayHandlerFactory returns a function which can be used as an AfterRetryHandler
// in the AWS Go SDK.  This AfterRetryHandler can be used to have a large number of retries where
// the initial delay between backoff grows exponentially (2^n * 30ms; defined in service.retryRules)
// and extra retries are performed at the maximum delay of the initial exponential growth.
func extendedRetryMaxDelayHandlerFactory(maxExtendedRetries uint) func(r *aws.Request) {
	return func(r *aws.Request) {
		realRetryCount := r.RetryCount
		maxDelayRetries := r.MaxRetries()
		if r.RetryCount >= (maxDelayRetries + maxExtendedRetries) {
			return
		} else if r.RetryCount >= maxDelayRetries {
			r.RetryCount = maxDelayRetries - 1
		}
		awsAfterRetryHandler(r)
		r.RetryCount = (realRetryCount + 1)
	}
}
