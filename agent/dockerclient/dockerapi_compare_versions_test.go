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

package dockerclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDockerVersions(t *testing.T) {
	testCases := []struct {
		input  string
		output dockerVersion
	}{
		{
			input: "1.0",
			output: dockerVersion{
				major: 1,
				minor: 0,
			},
		},
		{
			input: "5.2",
			output: dockerVersion{
				major: 5,
				minor: 2,
			},
		},
	}

	for i, testCase := range testCases {
		result, err := parseDockerVersions(testCase.input)
		assert.NoError(t, err, "#%v: Error parsing %v: %v", i, testCase.input, err)
		assert.Equal(t, testCase.output, result, "#%v: Expected %v, got %v", i, testCase.output, result)
	}

	invalidCases := []string{"foo", "", "bar", "x.y.z", "1.x.y", "1.1.z"}
	for i, invalidCase := range invalidCases {
		_, err := parseDockerVersions(invalidCase)
		assert.Error(t, err, "#%v: Expected error, didn't get one. Input: %v", i, invalidCase)
	}
}

func TestDockerVersionMatches(t *testing.T) {
	testCases := []struct {
		version        string
		selector       string
		expectedOutput bool
	}{
		{
			version:        "1.0",
			selector:       "1.0",
			expectedOutput: true,
		},
		{
			version:        "1.0",
			selector:       ">=1.0",
			expectedOutput: true,
		},
		{
			version:        "2.1",
			selector:       ">=1.0",
			expectedOutput: true,
		},
		{
			version:        "2.1",
			selector:       "<=1.0",
			expectedOutput: false,
		},
		{
			version:        "0.1",
			selector:       "<1.0",
			expectedOutput: true,
		},
		{
			version:        "0.1",
			selector:       "<=1.0",
			expectedOutput: true,
		},
		{
			version:        "1.0",
			selector:       "<1.1",
			expectedOutput: true,
		},
		{
			version:        "1.1",
			selector:       ">1.0",
			expectedOutput: true,
		},
		{
			version:        "1.1",
			selector:       "2.1,1.1",
			expectedOutput: true,
		},
		{
			version:        "2.0",
			selector:       "2.1,1.1",
			expectedOutput: false,
		},
		{
			version:        "1.9",
			selector:       ">=1.9,<=1.9",
			expectedOutput: true,
		},
	}

	for i, testCase := range testCases {
		result, err := DockerAPIVersion(testCase.version).Matches(testCase.selector)
		assert.NoError(t, err, "#%v: Unexpected error %v", i, err)
		assert.Equal(t, testCase.expectedOutput, result, "#%v: %v(%v) expected %v but got %v", i, testCase.version, testCase.selector, testCase.expectedOutput, result)
	}
}
