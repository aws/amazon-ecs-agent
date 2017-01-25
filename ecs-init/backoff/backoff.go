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
	"math"
	"math/rand"
	"sync"
	"time"
)

// Backoff defines a back-off/retry interface
type Backoff interface {
	Duration() time.Duration
	ShouldRetry() bool
}

//go:generate mockgen.sh docker $GOFILE ../docker

type retryBackoff struct {
	current        time.Duration
	max            time.Duration
	jitterMultiple float64
	multiple       float64
	maxRetries     int
	count          int
	countLock      sync.RWMutex
}

// NewBackoff creates a Backoff which ranges from min to max increasing by
// multiple each time.
// It also adds (and yes, the jitter is always added, never
// subtracted) a random amount of jitter up to jitterMultiple percent (that is,
// jitterMultiple = 0.0 is no jitter, 0.15 is 15% added jitter). The total time
// may exceed "max" when accounting for jitter, such that the absolute max is
// max + max * jiterMultiple
func NewBackoff(min, max time.Duration, jitterMultiple, multiple float64, maxRetries int) Backoff {
	return &retryBackoff{
		current:        min,
		max:            max,
		jitterMultiple: jitterMultiple,
		multiple:       multiple,
		maxRetries:     maxRetries,
		count:          0,
	}
}

func (rb *retryBackoff) Duration() time.Duration {
	rb.countLock.Lock()
	defer rb.countLock.Unlock()
	ret := rb.current
	rb.count += 1
	rb.current = time.Duration(math.Min(float64(rb.max.Nanoseconds()), float64(float64(rb.current.Nanoseconds())*rb.multiple)))
	return addJitter(ret, time.Duration(int64(float64(ret)*rb.jitterMultiple)))
}

func (rb *retryBackoff) ShouldRetry() bool {
	rb.countLock.RLock()
	defer rb.countLock.RUnlock()

	return rb.count < rb.maxRetries
}

// addJitter adds an amount of jitter between 0 and the given jitter to the
// given duration
func addJitter(duration time.Duration, jitter time.Duration) time.Duration {
	var randJitter int64
	if jitter.Nanoseconds() == 0 {
		randJitter = 0
	} else {
		randJitter = rand.Int63n(jitter.Nanoseconds())
	}
	return time.Duration(duration.Nanoseconds() + randJitter)
}
