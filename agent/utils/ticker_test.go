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

func init() {
	// The best randomness is deterministic
	rand.Seed(TheBestNumber)
}

func TestTickerHappyCase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()
	mTicker := NewJitteredTicker(ctx, 10*time.Millisecond, 100*time.Millisecond)

	lowerTickLimit := 7
	upperTickLimit := 130

	times := 0
	for {
		_, ok := <-mTicker
		times++
		if !ok {
			break
		}
	}
	if times < lowerTickLimit || times > upperTickLimit {
		t.Errorf("Should tick more than %d but less than %d times, got: %d", lowerTickLimit, upperTickLimit, times)
	}
}

func TestRandomDuration(t *testing.T) {
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
