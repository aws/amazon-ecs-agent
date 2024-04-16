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

func TestGetSupportedDockerAPIVersion(t *testing.T) {
	oldVersionResult := GetSupportedDockerAPIVersion(Version_1_17)
	assert.Equal(t, MinDockerAPIVersion, oldVersionResult)
	newVersionResult := GetSupportedDockerAPIVersion(Version_1_32)
	assert.Equal(t, Version_1_32, newVersionResult)
	invalidVersionResult := GetSupportedDockerAPIVersion(DockerVersion("Foo.Bar"))
	assert.Equal(t, MinDockerAPIVersion, invalidVersionResult)
}

func TestCompare(t *testing.T) {
	testCases := []struct {
		lhs            DockerVersion
		rhs            DockerVersion
		expectedResult int
	}{
		{
			lhs:            Version_1_17,
			rhs:            Version_1_17,
			expectedResult: 0,
		},
		{
			lhs:            Version_1_44,
			rhs:            Version_1_44,
			expectedResult: 0,
		},
		{
			lhs:            Version_1_17,
			rhs:            Version_1_33,
			expectedResult: -1,
		},
		{
			lhs:            Version_1_33,
			rhs:            Version_1_43,
			expectedResult: -1,
		},
		{
			lhs:            Version_1_34,
			rhs:            Version_1_35,
			expectedResult: -1,
		},
		{
			lhs:            Version_1_30,
			rhs:            Version_1_17,
			expectedResult: 1,
		},
		{
			lhs:            Version_1_18,
			rhs:            Version_1_17,
			expectedResult: 1,
		},
		{
			lhs:            Version_1_44,
			rhs:            Version_1_35,
			expectedResult: 1,
		},
		{
			lhs:            DockerVersion("Foo.Bar"),
			rhs:            Version_1_35,
			expectedResult: 0,
		},
		{
			lhs:            Version_1_35,
			rhs:            DockerVersion("foobar"),
			expectedResult: 0,
		},
		{
			lhs:            DockerVersion("Foo.Bar"),
			rhs:            DockerVersion("Foo.Bar"),
			expectedResult: 0,
		},
	}
	for _, tc := range testCases {
		actual := tc.lhs.Compare(tc.rhs)
		assert.Equal(t, tc.expectedResult, actual)
	}
}
