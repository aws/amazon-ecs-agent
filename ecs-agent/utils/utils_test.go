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
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	startTime            string = "2025-05-09T14:47:58.031Z"
	startTimeUTCSeconds  string = "1746802078.031"
	expectedFormatString string = `{"startTime":%s}`
)

type dummyStruct struct {
	// no contents
}

type timestampStruct struct {
	StartTime *Timestamp `json:"startTime,omitempty" type:"timestamp"`
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
			stringSlice := Uint16SliceToStringSlice(tc.param)
			assert.Equal(t, tc.expected, len(stringSlice), tc.name)

			for idx, num := range tc.param {
				reconverted, err := strconv.Atoi(stringSlice[idx])
				assert.NoError(t, err)
				assert.Equal(t, num, uint16(reconverted))
			}
		})
	}
}

func TestInt32PtrToIntPtr(t *testing.T) {
	testCases := []struct {
		input          *int32
		expectedOutput *int
		name           string
	}{
		{nil, nil, "nil"},
		{aws.Int32(2147483647), aws.Int(2147483647), "smallest max value type int can hold"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, Int32PtrToIntPtr(tc.input))
		})
	}
}

func TestInt64PtrToInt32Ptr(t *testing.T) {
	testCases := []struct {
		input          *int64
		expectedOutput *int32
		name           string
	}{
		{nil, nil, "nil"},
		{aws.Int64(2147483647), aws.Int32(2147483647), "smallest max value type int can hold"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedOutput, Int64PtrToInt32Ptr(tc.input))
		})
	}
}

func TestMaxNum(t *testing.T) {
	testMaxNumInt(t)
	testMaxNumFloat(t)
}

func testMaxNumInt(t *testing.T) {
	smallerVal := 8
	largerVal := 88

	require.Equal(t, largerVal, MaxNum(smallerVal, largerVal))
	require.Equal(t, largerVal, MaxNum(largerVal, largerVal))
}

func testMaxNumFloat(t *testing.T) {
	smallerVal := 2.718283
	largerVal := 3.14159

	require.Equal(t, largerVal, MaxNum(largerVal, smallerVal))
	require.Equal(t, largerVal, MaxNum(largerVal, largerVal))
}

func TestAddScheme(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "bare domain",
			input:    "example.com",
			expected: "https://example.com",
		},
		{
			name:     "already has https",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "already has http",
			input:    "http://example.com",
			expected: "http://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpointReturned := AddScheme(tt.input)
			if endpointReturned != tt.expected {
				t.Errorf("AddScheme() = %v, want %v", endpointReturned, tt.expected)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	cases := []struct {
		Name     string
		Value    string
		Expected string
	}{
		{
			Name:     "happy case",
			Value:    startTime,
			Expected: startTimeUTCSeconds,
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// parse it
			value, err := time.Parse(time.RFC3339, test.Value)
			// check for no errors
			assert.NoError(t, err)
			// format it
			actual := FormatTime(value)
			// check expected result
			assert.Equal(t, test.Expected, actual)
		})
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		Name     string
		Value    string
		Expected string
	}{
		{
			Name:     "happy case",
			Value:    startTimeUTCSeconds,
			Expected: startTime,
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// parse it
			value, err := time.Parse(time.RFC3339, test.Expected)
			// check for no errors
			assert.NoError(t, err)
			// parse it
			actual, err := ParseTime(test.Value)
			// check for no errors
			assert.NoError(t, err)
			// check expected result
			assert.Equal(t, value, actual)
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	cases := []struct {
		Name      string
		Value     timestampStruct
		StartTime string
		Expected  string
	}{
		{
			Name:      "happy case",
			Value:     timestampStruct{},
			StartTime: startTimeUTCSeconds,
			Expected:  fmt.Sprintf(expectedFormatString, startTimeUTCSeconds),
		},
		{
			Name:     "empty timestampStruct",
			Value:    timestampStruct{},
			Expected: "{}",
		},
		{
			Name: "non-nil StartTime",
			Value: timestampStruct{
				StartTime: nil,
			},
			Expected: "{}",
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// add startTime if non-empty
			if test.StartTime != "" {
				value, err := ParseTime(test.StartTime)
				assert.NoError(t, err)
				test.Value = timestampStruct{
					StartTime: (*Timestamp)(&value),
				}
			}

			// serialize it
			actual, err := json.Marshal(test.Value)
			// check expected result
			assert.Equal(t, test.Expected, string(actual))
			// check for no errors
			assert.NoError(t, err)
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	cases := []struct {
		Name      string
		Value     []byte
		StartTime string
		Expected  timestampStruct
	}{
		{
			Name:      "happy case",
			Value:     []byte(fmt.Sprintf(expectedFormatString, startTimeUTCSeconds)),
			StartTime: startTimeUTCSeconds,
			Expected:  timestampStruct{},
		},
		{
			Name:     "empty timestampStruct",
			Value:    []byte("{}"),
			Expected: timestampStruct{},
		},
		{
			Name:     "non-nil StartTime",
			Value:    []byte(fmt.Sprintf(`{"startTime":null}`)),
			Expected: timestampStruct{},
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			// add startTime if non-empty
			if test.StartTime != "" {
				value, err := ParseTime(test.StartTime)
				assert.NoError(t, err)
				test.Expected.StartTime = (*Timestamp)(&value)
			}
			var actual timestampStruct
			// deserialize it
			err := json.Unmarshal(test.Value, &actual)
			// check expected result
			assert.Equal(t, test.Expected, actual)
			// check for no errors
			assert.NoError(t, err)
		})
	}
}

func TestToPtrSlice(t *testing.T) {
	t.Run("Empty slice", func(t *testing.T) {
		input := []int{}
		result := ToPtrSlice(input)
		assert.Empty(t, result, "Result should be an empty slice")
	})

	t.Run("Slice of ints", func(t *testing.T) {
		input := []int{1, 2, 3}
		result := ToPtrSlice(input)
		assert.Len(t, result, 3, "Result should have the same length as input")
		for i, ptr := range result {
			assert.NotNil(t, ptr, "Pointer should not be nil")
			assert.Equal(t, input[i], *ptr, "Pointed value should match input")
		}
	})

	t.Run("Slice of strings", func(t *testing.T) {
		input := []string{"a", "b", "c"}
		result := ToPtrSlice(input)
		assert.Len(t, result, 3, "Result should have the same length as input")
		for i, ptr := range result {
			assert.NotNil(t, ptr, "Pointer should not be nil")
			assert.Equal(t, input[i], *ptr, "Pointed value should match input")
		}
	})

	t.Run("Slice of structs", func(t *testing.T) {
		type TestStruct struct {
			Value int
		}
		input := []TestStruct{{1}, {2}, {3}}
		result := ToPtrSlice(input)
		assert.Len(t, result, 3, "Result should have the same length as input")
		for i, ptr := range result {
			assert.NotNil(t, ptr, "Pointer should not be nil")
			assert.Equal(t, input[i], *ptr, "Pointed value should match input")
		}
	})
}
