// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
package utils

import (
	"context"
	"math/rand"
	"time"
)

// NewJitteredTicker works like a time.Ticker except with randomly distributed ticks
// between start and end duration.
func NewJitteredTicker(ctx context.Context, start, end time.Duration) <-chan time.Time {
	ticker := make(chan time.Time, 1)

	go func() {
		defer close(ticker)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(randomDuration(start, end))
				sendNow(ticker)
			}
		}
	}()

	return ticker
}

func randomDuration(start, end time.Duration) time.Duration {
	return time.Duration(start.Nanoseconds()+rand.Int63n(end.Nanoseconds()-start.Nanoseconds())) * time.Nanosecond
}

func sendNow(ticker chan<- time.Time) {
	select {
	case ticker <- time.Now():
	default:
	}
}
