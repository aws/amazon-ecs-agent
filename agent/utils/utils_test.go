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
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/golang/mock/gomock"
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

	if ZeroOrNil(struct{ uncomparable []uint16 }{uncomparable: []uint16{1, 2, 3}}) {
		t.Error("Uncomparable structs are never zero")
	}

	if ZeroOrNil(struct{ uncomparable []uint16 }{uncomparable: nil}) {
		t.Error("Uncomparable structs are never zero")
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mocktime := mock_ttime.NewMockTime(ctrl)
	_time = mocktime
	defer func() { _time = &ttime.DefaultTime{} }()

	mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(3)
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

	// no sleeps
	RetryWithBackoff(NewSimpleBackoff(10*time.Second, 20*time.Second, 0, 2), func() error {
		return NewRetriableError(NewRetriable(false), errors.New("can't retry"))
	})
}

func TestRetryNWithBackoff(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mocktime := mock_ttime.NewMockTime(ctrl)
	_time = mocktime
	defer func() { _time = &ttime.DefaultTime{} }()

	// 2 tries, 1 sleep
	mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(1)
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

	// 3 tries, 2 sleeps
	mocktime.EXPECT().Sleep(100 * time.Millisecond).Times(2)
	counter = 3
	err = RetryNWithBackoff(NewSimpleBackoff(100*time.Millisecond, 100*time.Millisecond, 0, 1), 5, func() error {
		counter--
		if counter == 0 {
			return nil
		}
		return errors.New("err")
	})

	if counter != 0 {
		t.Errorf("Counter expected to be 0, was %v", counter)
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestParseBool(t *testing.T) {

	truthyStrings := []string{"true", "1", "t", "true\r", "true ", "true \r"}
	falsyStrings := []string{"false", "0", "f", "false\r", "false ", "false \r"}
	neitherStrings := []string{"apple", " ", "\r", "orange", "maybe"}

	for ndx, str := range truthyStrings {
		if !ParseBool(str, false) {
			t.Fatal("Truthy string should be truthy: ", ndx)
		}
		if !ParseBool(str, true) {
			t.Fatal("Truthy string should be truthy (regardless of default): ", ndx)
		}
	}

	for ndx, str := range falsyStrings {
		if ParseBool(str, false) {
			t.Fatal("Falsy string should be falsy: ", ndx)
		}
		if ParseBool(str, true) {
			t.Fatal("falsy string should be falsy (regardless of default): ", ndx)
		}
	}

	for ndx, str := range neitherStrings {
		if ParseBool(str, false) {
			t.Fatal("Should default to false", ndx)
		}
		if !ParseBool(str, true) {
			t.Fatal("Should default to true", ndx)
		}
	}
}
