// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"math/rand"
	"time"
)

type Backoff interface {
	Reset()
	Duration() time.Duration
}

// AddJitter adds an amount of jitter between 0 and the given jitter to the
// given duration
func AddJitter(duration time.Duration, jitter time.Duration) time.Duration {
	var randJitter int64
	if jitter.Nanoseconds() == 0 {
		randJitter = 0
	} else {
		randJitter = rand.Int63n(jitter.Nanoseconds())
	}
	return time.Duration(duration.Nanoseconds() + randJitter)
}
