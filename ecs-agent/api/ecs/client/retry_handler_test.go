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

// TestOneDayRetrierMaxAttempts verifies that MaxAttempts returns the correct total number
// of retry attempts (298), which is the sum of initial exponential backoff retries (13)
// and extended fixed-delay retries (285). This ensures the agent-level retry loop will
// attempt SubmitTaskStateChange up to 298 times before giving up. Note that each of these
// attempts may trigger additional AWS SDK internal retries (typically 3), so the actual
// number of API calls could be higher.
func TestOneDayRetrierMaxAttempts(t *testing.T) {
	testCases := []struct {
		name             string
		expectedAttempts int
	}{
		{
			name:             "max attempts equals initial plus extra retries",
			expectedAttempts: submitStateChangeInitialRetries + submitStateChangeExtraRetries,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			retrier := oneDayRetrier{
				Standard: retry.NewStandard(),
			}
			assert.Equal(t, tc.expectedAttempts, retrier.MaxAttempts())
		})
	}
}

// TestOneDayRetrierRetryDelay verifies the retry delay behavior transitions correctly from
// exponential backoff (attempts 0-13) to fixed 5-minute delays (attempts 14+). This ensures
// the retrier quickly backs off initially but maintains consistent retry intervals for
// extended periods when dealing with persistent throttling or service issues.
func TestOneDayRetrierRetryDelay(t *testing.T) {
	testCases := []struct {
		name          string
		attempt       int
		minDelay      time.Duration
		maxDelay      time.Duration
		shouldBeFixed bool
	}{
		{
			name:          "first attempt has exponential backoff",
			attempt:       0,
			minDelay:      30 * time.Millisecond,
			maxDelay:      60 * time.Millisecond,
			shouldBeFixed: false,
		},
		{
			name:          "13th attempt still has exponential backoff",
			attempt:       13,
			minDelay:      4 * time.Minute,
			maxDelay:      9 * time.Minute,
			shouldBeFixed: false,
		},
		{
			name:          "14th attempt has fixed 5 minute delay",
			attempt:       14,
			minDelay:      5 * time.Minute,
			maxDelay:      5 * time.Minute,
			shouldBeFixed: true,
		},
		{
			name:          "100th attempt has fixed 5 minute delay",
			attempt:       100,
			minDelay:      5 * time.Minute,
			maxDelay:      5 * time.Minute,
			shouldBeFixed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			retrier := oneDayRetrier{
				Standard: retry.NewStandard(),
			}
			delay, err := retrier.RetryDelay(tc.attempt, &types.LimitExceededException{})
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, delay, tc.minDelay)
			assert.LessOrEqual(t, delay, tc.maxDelay)
			if tc.shouldBeFixed {
				assert.Equal(t, 5*time.Minute, delay)
			}
		})
	}
}

// TestOneDayRetrierIsErrorRetryable verifies that common throttling and transient errors
// are correctly identified as retryable. This ensures the retrier will continue attempting
// SubmitTaskStateChange when encountering rate limits or temporary service issues.
func TestOneDayRetrierIsErrorRetryable(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		shouldRetry bool
	}{
		{
			name:        "LimitExceededException is retryable",
			err:         &types.LimitExceededException{},
			shouldRetry: true,
		},
		{
			name:        "ServerException is not retryable by default SDK retrier",
			err:         &types.ServerException{},
			shouldRetry: false,
		},
		{
			name:        "ClientException is not retryable",
			err:         &types.ClientException{},
			shouldRetry: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			retrier := oneDayRetrier{
				Standard: retry.NewStandard(),
			}
			assert.Equal(t, tc.shouldRetry, retrier.IsErrorRetryable(tc.err))
		})
	}
}
