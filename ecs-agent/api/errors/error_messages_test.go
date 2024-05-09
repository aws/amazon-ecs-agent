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

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type testCase struct {
	testName    string
	errMsg      string
	args        []string
	expectedMsg string
}

func TestAugmentMessage(t *testing.T) {
	testCases := []testCase{
		{
			testName:    "Successful augmentation 1",
			errMsg:      "API error (404): repository repo/image not found",
			args:        []string{"execution-role-arn"},
			expectedMsg: "The task can’t pull the image 'repo/image' from Amazon Elastic Container Registry using the task execution role 'execution-role-arn'. To fix this, verify that the image URI is correct. Also check that the task execution role has the additional permissions to pull Amazon ECR images. Status Code: 404. Response: 'not found'",
		},
		{
			testName:    "Successful augmentation 2",
			errMsg:      "API error (404): repository 111122223333.dkr.ecr.us-east-1.amazonaws.com/repo1/image1 not found",
			args:        []string{"FooBarRole"},
			expectedMsg: "The task can’t pull the image '111122223333.dkr.ecr.us-east-1.amazonaws.com/repo1/image1' from Amazon Elastic Container Registry using the task execution role 'FooBarRole'. To fix this, verify that the image URI is correct. Also check that the task execution role has the additional permissions to pull Amazon ECR images. Status Code: 404. Response: 'not found'",
		},
		{
			testName:    "Successful augmentation 3",
			errMsg:      "API error (404): repository 111122223333.dkr.ecr.us-east-1.amazonaws.com/repo1/image1 not found",
			args:        []string{"FooBarRole"},
			expectedMsg: "The task can’t pull the image '111122223333.dkr.ecr.us-east-1.amazonaws.com/repo1/image1' from Amazon Elastic Container Registry using the task execution role 'FooBarRole'. To fix this, verify that the image URI is correct. Also check that the task execution role has the additional permissions to pull Amazon ECR images. Status Code: 404. Response: 'not found'",
		},
		{
			testName:    "Match with arg surplus",
			errMsg:      "API error (404): repository example/image not found",
			args:        []string{"taskRole", "foo", "Bar"},
			expectedMsg: "The task can’t pull the image 'example/image' from Amazon Elastic Container Registry using the task execution role 'taskRole'. To fix this, verify that the image URI is correct. Also check that the task execution role has the additional permissions to pull Amazon ECR images. Status Code: 404. Response: 'not found'",
		},
		{
			testName:    "Match with args deficit - return original",
			errMsg:      "API error (404): repository example/repository not found",
			args:        []string{},
			expectedMsg: "API error (404): repository example/repository not found",
		},
		{
			testName:    "Match but case sensitivity does not match - return original",
			errMsg:      "API ERror (404): repository example/repository nOt fouNd",
			args:        []string{},
			expectedMsg: "API ERror (404): repository example/repository nOt fouNd",
		},
		{
			testName:    "Does not recognize unknown error",
			errMsg:      "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
			args:        []string{},
			expectedMsg: "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
		},
		{
			testName:    "Does not recognize unknown error (with args)",
			errMsg:      "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
			args:        []string{"example/image", "taskRole", "foo", "Bar"},
			expectedMsg: "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
		},
		{
			testName:    "empty (with args)",
			errMsg:      "",
			args:        []string{"example/image", "taskRole", "foo", "Bar"},
			expectedMsg: "",
		},
		{
			testName:    "empty",
			errMsg:      "",
			args:        []string{},
			expectedMsg: "",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("AugmentMessage %s", tc.testName), func(t *testing.T) {
			actualMsg := AugmentMessage(tc.errMsg, tc.args...)
			require.Equal(t, tc.expectedMsg, actualMsg)
		})
	}
}
