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

package execcmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareAgentVersion(t *testing.T) {
	tt := []struct {
		name        string
		v1, v2      string
		res         int
		errExpected bool
	}{
		{
			"v1 < v2", "3.0.236.0", "3.0.237.0", -1, false,
		},
		{
			"v1 == v2", "3.0.236.0", "3.0.236.0", 0, false,
		},
		{
			"v1 > v2", "3.0.237.0", "3.0.236.0", 1, false,
		},
		{
			"negative v1", "-1.0.0.0", "3.0.236.0", badInputReturn, true,
		},
		{
			"negative v2", "3.0.236.0", "-1.0.0.0", badInputReturn, true,
		},
		{
			"invalid v1", "a.b.c.d", "3.0.236.0", badInputReturn, true,
		},
		{
			"invalid v2", "3.0.236.0", "a.b.c.d", badInputReturn, true,
		},
		{
			"missing elements v1", "3.0.236", "1.1.1.1", badInputReturn, true,
		},
		{
			"missing elements v2", "1.1.1.1", "3.0.236", badInputReturn, true,
		},
		{
			"blank v1", "", "1.1.1.1", badInputReturn, true,
		},
		{
			"blank v2", "1.1.1.1", "", badInputReturn, true,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			res, err := compareAgentVersion(tc.v1, tc.v2)
			if tc.errExpected {
				assert.Error(t, err, "Error was expected")
			} else {
				assert.NoError(t, err, "Error was NOT expected")
			}
			assert.Equal(t, tc.res, res)
		})
	}
}

func TestDetermineLatestVersion(t *testing.T) {
	tt := []struct {
		name            string
		versions        []string
		expectedVersion string
		expectedError   error
	}{
		{
			name:          "no versions",
			versions:      nil,
			expectedError: errors.New("no versions to compare were provided"),
		},
		{
			name:          "empty versions",
			versions:      []string{},
			expectedError: errors.New("no versions to compare were provided"),
		},
		{
			name:          "bad version",
			versions:      []string{"1"},
			expectedError: errors.New("invalid version format"),
		},
		{
			name:            "single version",
			versions:        []string{"3.0.236.0"},
			expectedVersion: "3.0.236.0",
		},
		{
			name:            "multiple version same value",
			versions:        []string{"3.0.236.0", "3.0.236.0"},
			expectedVersion: "3.0.236.0",
		},
		{
			name:            "multiple version diff values",
			versions:        []string{"2.0.1.0", "3.0.236.0", "1.1.1.1", "0.0.0.0"},
			expectedVersion: "3.0.236.0",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			v, err := determineLatestVersion(tc.versions)
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expectedVersion, v)
		})
	}
}
