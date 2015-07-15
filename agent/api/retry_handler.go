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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

const maxSubmitRetryDelay = 5 * time.Minute
const submitRetryDelayJitter = 3 * time.Minute

// maxSubmitRetryCount is how many times it will try to send the
// Submit*StateChange results on retriable errors before giving up forever.
// The below number was arrived at by estimating the number for retrying for 24 hours.
// The base retry handler does 2*n * 50 milliseconds of sleep.
// First we figure out how many retries it needs to reach 5 minutes:
// 5 min = \sum_{i=0}^n{2^i * 50 ms}
// 300000 ms = \sum_{i=0}^n{2^i} * 50ms
// 6000 = 2^{n+1} - 1
// n ~= 11.5
// So the first 12 tries will take just over 5 minutes, meaning that to get 24 hours we have:
// 24 hours ~= 12 baseTries * 5min / 12 baseTries + n extraTries * 5min/extraTry
// extraTries = 288
// retries = 12 + 288
const maxSubmitRetryCount = 12 + 288

const opSubmitContainerStateChange = "SubmitContainerStateChange"
const opSubmitTaskStateChange = "SubmitTaskStateChange"

// ECSRetryHandler defines how to retry ECS service calls. It behaves like the default retry handler, except for the SubmitStateChange operations where it has a massive upper limit on retry counts
func ECSRetryHandler(r *aws.Request) {
	if r.Operation == nil || (r.Operation.Name != opSubmitContainerStateChange && r.Operation.Name != opSubmitTaskStateChange) {
		aws.AfterRetryHandler(r)
		return
	}
	// else this is a Submit*StateChange operation
	// For these operations, fake the retry count for the sake of the WillRetry check.
	// Do this by temporarily setting it to 0 before calling that check.
	// We still keep the value around for sleep calculations
	// See https://github.com/aws/aws-sdk-go/blob/b2d953f489cf94029392157225e893d7b69cd447/aws/handler_functions.go#L107
	// for this code's inspiration
	realRetryCount := r.RetryCount
	if r.RetryCount < maxSubmitRetryCount {
		r.RetryCount = 0
	}

	r.Retryable.Set(r.Service.ShouldRetry(r))
	if r.WillRetry() {
		r.RetryCount = realRetryCount
		if r.RetryCount > 20 {
			// Hardcoded max for calling RetryRules here because it *will* overflow if you let it and result in sleeping negative time
			r.RetryDelay = maxSubmitRetryDelay
		} else {
			r.RetryDelay = durationMin(maxSubmitRetryDelay, r.Service.RetryRules(r))
		}
		// AddJitter is purely additive, so subtracting half the amount of jitter
		// makes it average out to RetryDelay
		ttime.Sleep(utils.AddJitter(r.RetryDelay-submitRetryDelayJitter/2, submitRetryDelayJitter))

		if r.Error != nil {
			if err, ok := r.Error.(awserr.Error); ok {
				if isCodeExpiredCreds(err.Code()) {
					r.Config.Credentials.Expire()
				}
			}
		}

		r.RetryCount++
		r.Error = nil
	}
}

func durationMin(d1 time.Duration, d2 time.Duration) time.Duration {
	if d1 < d2 {
		return d1
	}
	return d2
}

// credsExpiredCodes is a collection of error codes which signify the credentials
// need to be refreshed. Expired tokens require refreshing of credentials, and
// resigning before the request can be retried.
var credsExpiredCodes = map[string]struct{}{
	"ExpiredToken":          {},
	"ExpiredTokenException": {},
	"RequestExpired":        {}, // EC2 Only
}

func isCodeExpiredCreds(code string) bool {
	_, ok := credsExpiredCodes[code]
	return ok
}
