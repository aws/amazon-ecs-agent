// Copyright 2015-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAddJitter(t *testing.T) {
	for i := 0; i < 10; i++ {
		duration := addJitter(10*time.Second, 3*time.Second)

		assert.False(t, duration < 10*time.Second, "Expect jitter >= 10*time.Second")
		assert.False(t, duration > 13*time.Second, "Expect jitter <= 13*time.Second")
	}
}

func TestRetryBackoffShouldRetry(t *testing.T) {
	retryBackoff := NewBackoff(time.Second, time.Minute, 0.2, 2, 2)
	assert.True(t, retryBackoff.ShouldRetry(), "Expect to retry when count < max retries")
	retryBackoff.Duration()
	assert.True(t, retryBackoff.ShouldRetry(), "Expect to retry when count < max retries")
	retryBackoff.Duration()
	assert.False(t, retryBackoff.ShouldRetry(), "Expect to not retry when count >= max retries")
}

func TestRetryBackoffDuration(t *testing.T) {
	// Create a backoff with no jitter
	retryBackoff := NewBackoff(time.Second, 3*time.Second, 0, 2, 3)
	assert.Equal(t, retryBackoff.Duration(), time.Second, "expect initial backoff to equal min backoff duration")
	assert.Equal(t, retryBackoff.Duration(), 2*time.Second, "expect second backoff to equal 2 * min backoff")
	assert.Equal(t, retryBackoff.Duration(), 3*time.Second, "expect 3rd backoff to be max backoff")
	assert.False(t, retryBackoff.ShouldRetry(), "expect to not retry when count >= max retries")
}
