// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"errors"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
)

func TestDefaultIfBlank(t *testing.T) {
	const defaultValue = "a boring default"
	const specifiedValue = "new value"
	result := DefaultIfBlank(specifiedValue, defaultValue)

	if result != specifiedValue {
		t.Error("Expected " + specifiedValue + ", got " + result)
	}

	result = DefaultIfBlank("", defaultValue)
	if result != defaultValue {
		t.Error("Expected " + defaultValue + ", got " + result)
	}
}

func TestZeroOrNil(t *testing.T) {

	if !ZeroOrNil(nil) {
		t.Error("Nil is nil")
	}

	if !ZeroOrNil(0) {
		t.Error("0 is 0")
	}

	if !ZeroOrNil("") {
		t.Error("\"\" is the string zerovalue")
	}

	type ZeroTest struct {
		testInt int
		TestStr string
	}

	if !ZeroOrNil(ZeroTest{}) {
		t.Error("ZeroTest zero-value should be zero")
	}

	if ZeroOrNil(ZeroTest{TestStr: "asdf"}) {
		t.Error("ZeroTest with a field populated isn't zero")
	}

	if ZeroOrNil(1) {
		t.Error("1 is not 0")
	}

	uintSlice := []uint16{1, 2, 3}
	if ZeroOrNil(uintSlice) {
		t.Error("[1,2,3] is not zero")
	}

	uintSlice = []uint16{}
	if !ZeroOrNil(uintSlice) {
		t.Error("[] is Zero")
	}

}

func TestSlicesDeepEqual(t *testing.T) {
	if !SlicesDeepEqual([]string{}, []string{}) {
		t.Error("Empty slices should be equal")
	}
	if SlicesDeepEqual([]string{"cat"}, []string{}) {
		t.Error("Should not be equal")
	}
	if !SlicesDeepEqual([]string{"cat"}, []string{"cat"}) {
		t.Error("Should be equal")
	}
	if !SlicesDeepEqual([]string{"cat", "dog", "cat"}, []string{"dog", "cat", "cat"}) {
		t.Error("Should be equal")
	}
}

func TestRetryWithBackoff(t *testing.T) {
	test_time := &ttime.TestTime{}
	test_time.LudicrousSpeed(true)
	ttime.SetTime(test_time)

	start := ttime.Now()

	counter := 3
	RetryWithBackoff(NewSimpleBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), func() error {
		if counter == 0 {
			return nil
		}
		counter--
		return errors.New("err")
	})
	if counter != 0 {
		t.Error("Counter didn't go to 0; didn't get retried enough")
	}
	testTime := ttime.Since(start)

	if testTime.Seconds() < .29 || testTime.Seconds() > .31 {
		t.Error("Retry didn't backoff for as long as expected")
	}

	start = ttime.Now()
	RetryWithBackoff(NewSimpleBackoff(10*time.Second, 20*time.Second, 0, 2), func() error {
		return NewRetriableError(NewRetriable(false), errors.New("can't retry"))
	})

	if ttime.Since(start).Seconds() > .1 {
		t.Error("Retry for the trivial function took too long")
	}
}

func TestRetryNWithBackoff(t *testing.T) {
	test_time := &ttime.TestTime{}
	test_time.LudicrousSpeed(true)
	ttime.SetTime(test_time)

	start := ttime.Now()

	counter := 3
	err := RetryNWithBackoff(NewSimpleBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 2, func() error {
		counter--
		return errors.New("err")
	})
	if counter != 1 {
		t.Error("Should have stopped after two tries")
	}
	if err == nil {
		t.Error("Should have returned appropriate error")
	}
	testTime := ttime.Since(start)
	// Expect that it tried twice, sleeping once between them
	if testTime.Seconds() < 0.09 || testTime.Seconds() > 0.11 {
		t.Error("Retry didn't backoff for as long as expected: %v", testTime.Seconds())
	}

	start = ttime.Now()
	counter = 3
	err = RetryNWithBackoff(NewSimpleBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 5, func() error {
		counter--
		if counter == 0 {
			return nil
		}
		return errors.New("err")
	})
	testTime = ttime.Since(start)
	if counter != 0 {
		t.Errorf("Counter expected to be 0, was %v", counter)
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	// 3 tries; 2 backoffs
	if testTime.Seconds() < 0.190 || testTime.Seconds() > 0.210 {
		t.Error("Retry didn't backoff for as long as expected: %v", testTime.Seconds())
	}
}
