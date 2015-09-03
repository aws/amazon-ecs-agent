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

package util

import "testing"

func TestVersionLessThan(t *testing.T) {
	testCases := []struct {
		lhs            string
		rhs            string
		expectedError  bool
		expectedOutput bool // ignored if an error is expected
	}{
		{
			lhs:            "1.0.0",
			rhs:            "0.0.1",
			expectedOutput: false,
		},
		{
			lhs:            "0.0.1",
			rhs:            "1.0.0",
			expectedOutput: true,
		},
		{
			lhs:            "1.2.3",
			rhs:            "1.2.4",
			expectedOutput: true,
		},
		{
			lhs:            "2.2.3",
			rhs:            "1.9.9",
			expectedOutput: false,
		},
		{
			lhs:           "2.2",
			rhs:           "1.9.1",
			expectedError: true,
		},
		{
			lhs:           "2.2.2",
			rhs:           "1",
			expectedError: true,
		},
		{
			lhs:           "",
			rhs:           "1",
			expectedError: true,
		},
		{
			lhs:            "1.1.1",
			rhs:            "1.1.1",
			expectedOutput: false,
		},
	}

	for i, testCase := range testCases {
		result, err := Version(testCase.lhs).lessThan(Version(testCase.rhs))
		if testCase.expectedError {
			if err == nil {
				t.Errorf("#%v: expected error, did not get one", i)
			}
		} else {
			if err != nil {
				t.Errorf("#%v: unexpected error: %v", i, err)
			}
			if result != testCase.expectedOutput {
				t.Errorf("#%v: expected %v but got %v", i, testCase.expectedOutput, result)
			}
		}
	}
}

func TestVersionMatches(t *testing.T) {
	testCases := []struct {
		version        string
		selector       string
		expectedOutput bool
	}{
		{
			version:        "1.0.0",
			selector:       "1.0.0",
			expectedOutput: true,
		},
		{
			version:        "1.0.0",
			selector:       ">=1.0.0",
			expectedOutput: true,
		},
		{
			version:        "2.1.0",
			selector:       ">=1.0.0",
			expectedOutput: true,
		},
		{
			version:        "2.1.0",
			selector:       "<=1.0.0",
			expectedOutput: false,
		},
		{
			version:        "0.1.0",
			selector:       "<1.0.0",
			expectedOutput: true,
		},
		{
			version:        "0.1.0",
			selector:       "<=1.0.0",
			expectedOutput: true,
		},
	}

	for i, testCase := range testCases {
		result, err := Version(testCase.version).Matches(testCase.selector)
		if err != nil {
			t.Errorf("#%v: Unexpected error %v", i, err)
		}
		if result != testCase.expectedOutput {
			t.Errorf("#%v: expected %v but got %v", i, testCase.expectedOutput, result)
		}
	}
}
