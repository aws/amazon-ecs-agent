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
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type testCaseAugmentMessage struct {
	testName    string
	errMsg      string
	args        []string
	expectedMsg string
}

func TestAugmentMessage(t *testing.T) {
	testCases := []testCaseAugmentMessage{
		{
			testName:    "Successful augmentation with args",
			errMsg:      "Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			args:        []string{"MyBrokenRole"},
			expectedMsg: "Check if image exists and role 'MyBrokenRole' has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
		},
		{
			testName:    "Successful augmentation with args 2",
			errMsg:      "Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
			args:        []string{"MyBrokenRole"},
			expectedMsg: "Check if image exists and role 'MyBrokenRole' has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
		},
		{
			testName:    "Successful augmentation without args",
			errMsg:      "Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			args:        []string{},
			expectedMsg: "Check if image exists and role has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
		},
		{
			testName:    "Successful augmentation without args 2",
			errMsg:      "Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
			args:        []string{},
			expectedMsg: "Check if image exists and role has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
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

type testCaseAugmentErrMsg struct {
	testName    string
	errMsg      NamedError
	expectedMsg string
}

type KnownError struct {
	FromError error
}

func (err KnownError) Error() string {
	return err.FromError.Error()
}

func (err KnownError) ErrorName() string {
	return "KnownError"
}

func (err KnownError) Constructor() func(string) NamedError {
	return func(msg string) NamedError {
		return KnownError{errors.New(msg)}
	}
}

// does not implement Constructor()
type UnknownError struct {
	FromError error
}

func (err UnknownError) Error() string {
	return err.FromError.Error()
}

func (err UnknownError) ErrorName() string {
	return "UnknownError"
}

func TestAugmentErrMsg(t *testing.T) {
	tests := []struct {
		name         string
		err          NamedError
		args         []string
		expectedMsg  string
		expectedName string
	}{
		{
			name:         "Non-constructible NamedError",
			err:          UnknownError{FromError: errors.New("some err")},
			args:         []string{"additional context"},
			expectedMsg:  "some err",
			expectedName: "UnknownError",
		},
		{
			name:         "NamedError error message updated with args",
			err:          KnownError{FromError: errors.New("Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action")},
			args:         []string{"MyBrokenRole"},
			expectedMsg:  "Check if image exists and role 'MyBrokenRole' has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedName: "KnownError",
		},
		{
			name:         "NamedError error message updated no args",
			err:          KnownError{FromError: errors.New("Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action")},
			args:         []string{},
			expectedMsg:  "Check if image exists and role has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedName: "KnownError",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			augmentedErr := AugmentErrMsg(tc.err, tc.args...)
			require.Equal(t, augmentedErr.Error(), tc.expectedMsg)
			require.Equal(t, augmentedErr.ErrorName(), tc.expectedName)
		})
	}
}
