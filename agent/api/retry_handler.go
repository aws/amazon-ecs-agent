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

	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

const minSubmitRetryDelay = 5 * time.Minute

const opSubmitContainerStateChange = "SubmitContainerStateChange"
const opSubmitTaskStateChange = "SubmitTaskStateChange"

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

// ECSRetryHandler defines how to retry ECS service calls. It behaves like the default retry handler, except for the SubmitStateChange operations where it does not have an upper limit on retry counts
func ECSRetryHandler(r *aws.Request) {
	if r.Operation != nil && r.Operation.Name == opSubmitContainerStateChange || r.Operation.Name == opSubmitTaskStateChange {
		// For these operations, don't include retrycount in the 'WillRetry' check.
		// Do this by temporarily setting it to 0 before calling that check.
		// We still keep the value around for sleep calculations
		// See https://github.com/aws/aws-sdk-go/blob/b2d953f489cf94029392157225e893d7b69cd447/aws/handler_functions.go#L107
		// for this code's inspiration
		realRetryCount := r.RetryCount
		r.RetryCount = 0

		r.Retryable.Set(r.Service.ShouldRetry(r))
		if r.WillRetry() {
			r.RetryCount = realRetryCount
			r.RetryDelay = durationMin(minSubmitRetryDelay, r.Service.RetryRules(r))
			ttime.Sleep(r.RetryDelay)

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
	} else {
		aws.AfterRetryHandler(r)
	}
}
