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
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

type dummyStruct struct {
	// no contents
}

func TestZeroOrNil(t *testing.T) {
	type ZeroTest struct {
		testInt     int
		TestStr     string
		testNilJson dummyStruct
	}

	var strMap map[string]string

	testCases := []struct {
		param    interface{}
		expected bool
		name     string
	}{
		{nil, true, "Nil is nil"},
		{0, true, "0 is 0"},
		{"", true, "\"\" is the string zerovalue"},
		{ZeroTest{}, true, "ZeroTest zero-value should be zero"},
		{ZeroTest{TestStr: "asdf"}, false, "ZeroTest with a field populated isn't zero"},
		{ZeroTest{testNilJson: dummyStruct{}}, true, "nil is nil"},
		{1, false, "1 is not 0"},
		{[]uint16{1, 2, 3}, false, "[1,2,3] is not zero"},
		{[]uint16{}, true, "[] is zero"},
		{struct{ uncomparable []uint16 }{uncomparable: []uint16{1, 2, 3}}, false, "Uncomparable structs are never zero"},
		{struct{ uncomparable []uint16 }{uncomparable: nil}, false, "Uncomparable structs are never zero"},
		{strMap, true, "map[string]string is zero or nil"},
		{make(map[string]string), true, "empty map[string]string is zero or nil"},
		{map[string]string{"foo": "bar"}, false, "map[string]string{foo:bar} is not zero or nil"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, ZeroOrNil(tc.param), tc.name)
		})
	}

}

// TestUint16SliceToStringSlice tests the utils method Uint16SliceToStringSlice
// By taking in a slice of untyped 16 bit ints, asserting the util function
// returns the correct size of array, and asserts their equality.
// This is done by re-converting the string into a uint16.
func TestUint16SliceToStringSlice(t *testing.T) {
	testCases := []struct {
		param    []uint16
		expected int
		name     string
	}{
		{nil, 0, "Nil argument"},
		{[]uint16{0, 1, 2, 3}, 4, "Basic set"},
		{[]uint16{65535}, 1, "Max Value"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := Uint16SliceToStringSlice(tc.param)
			assert.Equal(t, tc.expected, len(output), tc.name)
			for idx, num := range tc.param {
				reconverted, err := strconv.Atoi(*output[idx])
				assert.NoError(t, err)
				assert.Equal(t, num, uint16(reconverted))
			}

		})
	}
}

func TestInt64PtrToIntPtr(t *testing.T) {
	testCases := []struct {
		input          *int64
		expectedOutput *int
		name           string
	}{
		{nil, nil, "nil"},
		{aws.Int64(2147483647), aws.Int(2147483647), "smallest max value type int can hold"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, Int64PtrToIntPtr(tc.input))
		})
	}
}
