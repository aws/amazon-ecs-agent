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

package ecsclient

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
)

func TestOneDayRetrier(t *testing.T) {
	retrier := oneDayRetrier{
		Standard: retry.NewStandard(),
	}

	retryErr := &types.LimitExceededException{}
	var totalDelay time.Duration

	for retries := 0; retries < retrier.MaxAttempts(); retries++ {
		if retrier.IsErrorRetryable(retryErr) {
			delay, err := retrier.RetryDelay(retries, retryErr)
			assert.NoError(t, err)
			totalDelay += delay
		}
	}

	if totalDelay > 25*time.Hour || totalDelay < 23*time.Hour {
		t.Errorf("Expected accumulated retry delay to be roughly 24 hours; was %v", totalDelay)
	}
}
