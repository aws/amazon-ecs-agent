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
	"errors"
	"sort"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/stretchr/testify/assert"
)

func TestDefaultIfBlank(t *testing.T) {
	const defaultValue = "a boring default"
	const specifiedValue = "new value"
	result := DefaultIfBlank(specifiedValue, defaultValue)
	assert.Equal(t, specifiedValue, result)

	result = DefaultIfBlank("", defaultValue)
	assert.Equal(t, defaultValue, result)
}

func TestZeroOrNil(t *testing.T) {
	type ZeroTest struct {
		testInt int
		TestStr string
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

func TestSlicesDeepEqual(t *testing.T) {
	testCases := []struct {
		left     []string
		right    []string
		expected bool
		name     string
	}{
		{[]string{}, []string{}, true, "Two empty slices"},
		{[]string{"cat"}, []string{}, false, "One empty slice"},
		{[]string{}, []string{"cat"}, false, "Another empty slice"},
		{[]string{"cat"}, []string{"cat"}, true, "Two slices with one element each"},
		{[]string{"cat", "dog", "cat"}, []string{"dog", "cat", "cat"}, true, "Two slices with multiple elements"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, SlicesDeepEqual(tc.left, tc.right))
		})
	}
}

func TestParseBool(t *testing.T) {
	truthyStrings := []string{"true", "1", "t", "true\r", "true ", "true \r"}
	falsyStrings := []string{"false", "0", "f", "false\r", "false ", "false \r"}
	neitherStrings := []string{"apple", " ", "\r", "orange", "maybe"}

	for _, str := range truthyStrings {
		t.Run("truthy", func(t *testing.T) {
			assert.True(t, ParseBool(str, false), "Truthy string should be truthy")
			assert.True(t, ParseBool(str, true), "Truthy string should be truthy (regardless of default)")
		})
	}

	for _, str := range falsyStrings {
		t.Run("falsy", func(t *testing.T) {
			assert.False(t, ParseBool(str, false), "Falsy string should be falsy")
			assert.False(t, ParseBool(str, true), "Falsy string should be falsy (regardless of default)")
		})
	}

	for _, str := range neitherStrings {
		t.Run("defaults", func(t *testing.T) {
			assert.False(t, ParseBool(str, false), "Should default to false")
			assert.True(t, ParseBool(str, true), "Should default to true")
		})
	}
}

func TestIsAWSErrorCodeEqual(t *testing.T) {
	testcases := []struct {
		name string
		err  error
		res  bool
	}{
		{
			name: "Happy Path",
			err:  awserr.New(ecs.ErrCodeInvalidParameterException, "errMsg", errors.New("err")),
			res:  true,
		},
		{
			name: "Wrong Error Code",
			err:  awserr.New("errCode", "errMsg", errors.New("err")),
			res:  false,
		},
		{
			name: "Wrong Error Type",
			err:  errors.New("err"),
			res:  false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.res, IsAWSErrorCodeEqual(tc.err, ecs.ErrCodeInvalidParameterException))
		})
	}
}

func TestMapToTags(t *testing.T) {
	tagKey1 := "tagKey1"
	tagKey2 := "tagKey2"
	tagValue1 := "tagValue1"
	tagValue2 := "tagValue2"
	tagsMap := map[string]string{
		tagKey1: tagValue1,
		tagKey2: tagValue2,
	}
	tags := MapToTags(tagsMap)
	assert.Equal(t, 2, len(tags))
	sort.Slice(tags, func(i, j int) bool {
		return aws.StringValue(tags[i].Key) < aws.StringValue(tags[j].Key)
	})

	assert.Equal(t, aws.StringValue(tags[0].Key), tagKey1)
	assert.Equal(t, aws.StringValue(tags[0].Value), tagValue1)
	assert.Equal(t, aws.StringValue(tags[1].Key), tagKey2)
	assert.Equal(t, aws.StringValue(tags[1].Value), tagValue2)
}

func TestNilMapToTags(t *testing.T) {
	assert.Zero(t, len(MapToTags(nil)))
}
