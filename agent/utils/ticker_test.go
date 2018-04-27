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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	TheBestNumber = 28
)

func TestTickerHappyCase(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	mTicker := NewJitteredTicker(ctx, 10*time.Millisecond, 100*time.Millisecond)

	times := 0
	for {
		_, ok := <-mTicker
		times++
		if !ok {
			break
		}
	}

	if times < 10 || times > 100 {
		t.Error("Should tick at least 10 but less than 100 times: ", times)
	}
}

func TestRandomDuration(t *testing.T) {
	// The best randomness is deterministic
	rand.Seed(TheBestNumber)

	start := 10 * time.Second
	end := 20 * time.Second

	for i := 0; i < 10000; i++ {
		c := randomDuration(start, end)
		if c < start || c > end {
			t.Errorf("Generated a bad time! seconds: %v nanoseconds: %d", c, c.Nanoseconds())
		}
	}
}

func TestSendNowDoesNotBlock(t *testing.T) {
	c := make(chan time.Time, 1)
	sendNow(c)
	sendNow(c)

	times := 0
loop:
	for {
		select {
		case <-c:
			times++
		default:
			break loop
		}
	}

	assert.Equal(t, 1, times, "Channel didn't have exactly one message on the queue")
}
