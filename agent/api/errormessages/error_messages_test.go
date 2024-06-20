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

package errormessages

import (
	"errors"
	"fmt"
	"testing"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
	"github.com/stretchr/testify/require"
)

type testCaseAugmentMessage struct {
	testName    string
	errMsg      string
	expectedMsg string
}

func TestAugmentMessage(t *testing.T) {
	testCases := []testCaseAugmentMessage{
		{
			testName:    "Successful augmentation - missing pull permissions",
			errMsg:      "Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedMsg: "The task can’t pull the image. Check that the role has the permissions to pull images from the registry. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
		},
		{
			testName:    "Successful augmentation - image does not exist",
			errMsg:      "Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
			expectedMsg: "The task can’t pull the image. Check whether the image exists. Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
		},
		{
			testName:    "Successful augmentation - network issues",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			expectedMsg: "The task can’t pull the image. Check your network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Does not recognize unknown error",
			errMsg:      "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
			expectedMsg: "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
		},
		{
			testName:    "empty",
			errMsg:      "",
			expectedMsg: "",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("AugmentMessage %s", tc.testName), func(t *testing.T) {
			actualMsg := AugmentMessage(tc.errMsg)
			require.Equal(t, tc.expectedMsg, actualMsg)
		})
	}
}

type testCaseAugmentNamedErrMsg struct {
	testName    string
	errMsg      apierrors.NamedError
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

func (err KnownError) WithAugmentedErrorMessage(msg string) apierrors.NamedError {
	return KnownError{errors.New(msg)}
}

// does not implement WithAugmentedErrorMessage()
type UnknownError struct {
	FromError error
}

func (err UnknownError) Error() string {
	return err.FromError.Error()
}

func (err UnknownError) ErrorName() string {
	return "UnknownError"
}

func TestAugmentNamedErrMsg(t *testing.T) {
	tests := []struct {
		name         string
		err          apierrors.NamedError
		expectedMsg  string
		expectedName string
	}{
		{
			name:         "Non-augmentable NamedError",
			err:          UnknownError{FromError: errors.New("some err")},
			expectedMsg:  "some err",
			expectedName: "UnknownError",
		},
		{
			name:         "NamedError error message updated with args",
			err:          KnownError{FromError: errors.New("Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action")},
			expectedMsg:  "The task can’t pull the image. Check that the role has the permissions to pull images from the registry. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedName: "KnownError",
		},
		{
			name:         "NamedError error message updated no args",
			err:          KnownError{FromError: errors.New("Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action")},
			expectedMsg:  "The task can’t pull the image. Check that the role has the permissions to pull images from the registry. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedName: "KnownError",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			augmentedErr := AugmentNamedErrMsg(tc.err)
			require.Equal(t, augmentedErr.Error(), tc.expectedMsg)
			require.Equal(t, augmentedErr.ErrorName(), tc.expectedName)
		})
	}
}
