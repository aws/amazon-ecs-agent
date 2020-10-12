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
	"math"
	"sync"
	"time"
)

type ExponentialBackoff struct {
	current        time.Duration
	start          time.Duration
	max            time.Duration
	jitterMultiple float64
	multiple       float64
	mu             sync.Mutex
}

// NewExponentialBackoff creates a Backoff which ranges from min to max increasing by
// multiple each time. (t = start * multiple * n for the nth iteration)
// It also adds (and yes, the jitter is always added, never
// subtracted) a random amount of jitter up to jitterMultiple percent (that is,
// jitterMultiple = 0.0 is no jitter, 0.15 is 15% added jitter). The total time
// may exceed "max" when accounting for jitter, such that the absolute max is
// max + max * jiterMultiple

func NewExponentialBackoff(min, max time.Duration, jitterMultiple, multiple float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		start:          min,
		current:        min,
		max:            max,
		jitterMultiple: jitterMultiple,
		multiple:       multiple,
	}
}

func (sb *ExponentialBackoff) Duration() time.Duration {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	ret := sb.current
	sb.current = time.Duration(math.Min(float64(sb.max.Nanoseconds()), float64(sb.current.Nanoseconds())*sb.multiple))
	return AddJitter(ret, time.Duration(int64(float64(ret)*sb.jitterMultiple)))
}

func (sb *ExponentialBackoff) Reset() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.current = sb.start
}
