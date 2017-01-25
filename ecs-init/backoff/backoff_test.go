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
)

func TestAddJitter(t *testing.T) {
	for i := 0; i < 10; i++ {
		duration := addJitter(10*time.Second, 3*time.Second)
		if duration < 10*time.Second || duration > 13*time.Second {
			t.Error("Excessive amount of jitter", duration)
		}
	}
}

func TestRetryBackoffShouldRetry(t *testing.T) {
	retryBackoff := NewBackoff(time.Second, time.Minute, 0.2, 2, 2)
	if retryBackoff.ShouldRetry() != true {
		t.Errorf("Expected to retry when count < max retries")
	}
	retryBackoff.Duration()
	if retryBackoff.ShouldRetry() != true {
		t.Errorf("Expected to retry when count < max retries")
	}
	retryBackoff.Duration()
	if retryBackoff.ShouldRetry() != false {
		t.Errorf("Expected to not retry when count >= max retries")
	}
}

func TestRetryBackoffDuration(t *testing.T) {
	// Create a backoff with no jitter
	retryBackoff := NewBackoff(time.Second, 3*time.Second, 0, 2, 3)
	// Expect initial backoff == min backoff duration
	if duration := retryBackoff.Duration(); duration != time.Second {
		t.Error("Initial duration is incorrect")
	}
	// Expect next backoff == 2 * min backoff
	if duration := retryBackoff.Duration(); duration != 2*time.Second {
		t.Error("Increase in duration is incorrect")
	}
	// Expect next backoff duration to be max backoff
	if duration := retryBackoff.Duration(); duration != 3*time.Second {
		t.Error("Increase in duration is incorrect, exceeded maximum")
	}
	if retryBackoff.ShouldRetry() != false {
		t.Errorf("Expected to not retry when count >= max retries")
	}
}
