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
	ctx         ErrorContext
	expectedMsg string
}

func TestAugmentMessage(t *testing.T) {
	testCases := []testCaseAugmentMessage{
		{
			testName:    "Successful augmentation with args",
			errMsg:      "Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			ctx:         ErrorContext{ExecRole: "MyBrokenRole"},
			expectedMsg: "Check if image exists and role 'MyBrokenRole' has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
		},
		{
			testName:    "Successful augmentation with args 2",
			errMsg:      "Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
			ctx:         ErrorContext{ExecRole: "MyBrokenRole"},
			expectedMsg: "Check if image exists and role 'MyBrokenRole' has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
		},
		{
			testName:    "Successful augmentation with args 3 (no args)",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{},
			expectedMsg: "Check your network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args wrong arg",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{NetworkMode: "foo"},
			expectedMsg: "Check your host network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args 3 (host) 1",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{NetworkMode: "bridge"},
			expectedMsg: "Check your host network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args 3 (host) 2",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{NetworkMode: "none"},
			expectedMsg: "Check your host network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args 3 (host) 3",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{NetworkMode: "host"},
			expectedMsg: "Check your host network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args 3 (host) 4",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{NetworkMode: "default"},
			expectedMsg: "Check your host network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args 3 (task) 1",
			errMsg:      "RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
			ctx:         ErrorContext{NetworkMode: "awsvpc"},
			expectedMsg: "Check your task network configuration. RequestError: send request failed\ncaused by: Post \"https://api.ecr.us-east-1.amazonaws.com/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)",
		},
		{
			testName:    "Successful augmentation with args 4",
			errMsg:      "ResourceNotFoundException: Secrets Manager can't find the specified secret.",
			ctx:         ErrorContext{SecretID: "aws::arn::my-secret"},
			expectedMsg: "ResourceInitializationError: The task can't retrieve the secret with ARN 'aws::arn::my-secret' from AWS Secrets Manager. Check that the secret ARN is correct. ResourceNotFoundException: Secrets Manager can't find the specified secret.",
		},
		{
			testName:    "Successful augmentation without args",
			errMsg:      "Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			ctx:         ErrorContext{},
			expectedMsg: "Check if image exists and role has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
		},
		{
			testName:    "Successful augmentation without args 2",
			errMsg:      "Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
			ctx:         ErrorContext{},
			expectedMsg: "Check if image exists and role has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for some/nonsense, repository does not exist or may require 'docker login': denied: requested access to the resource is denied",
		},
		{
			testName:    "Does not recognize unknown error",
			errMsg:      "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
			ctx:         ErrorContext{},
			expectedMsg: "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
		},
		{
			testName:    "Does not recognize unknown error (with args)",
			errMsg:      "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
			ctx:         ErrorContext{ExecRole: "foo", NetworkMode: "bridge"},
			expectedMsg: "API error (500): Get https://registry-1.docker.io/v2/library/amazonlinux/manifests/1: unauthorized: incorrect username or password",
		},
		{
			testName:    "empty (with args)",
			errMsg:      "",
			ctx:         ErrorContext{},
			expectedMsg: "",
		},
		{
			testName:    "empty",
			errMsg:      "",
			ctx:         ErrorContext{},
			expectedMsg: "",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("AugmentMessage %s", tc.testName), func(t *testing.T) {
			actualMsg := AugmentMessage(tc.errMsg, tc.ctx)
			require.Equal(t, tc.expectedMsg, actualMsg)
		})
	}
}

type testCaseAugmentNamedErrMsg struct {
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

func TestAugmentNamedErrMsg(t *testing.T) {
	tests := []struct {
		name         string
		err          NamedError
		ctx          ErrorContext
		expectedMsg  string
		expectedName string
	}{
		{
			name:         "Non-constructible NamedError",
			err:          UnknownError{FromError: errors.New("some err")},
			ctx:          ErrorContext{},
			expectedMsg:  "some err",
			expectedName: "UnknownError",
		},
		{
			name:         "NamedError error message updated with args",
			err:          KnownError{FromError: errors.New("Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action")},
			ctx:          ErrorContext{ExecRole: "MyBrokenRole"},
			expectedMsg:  "Check if image exists and role 'MyBrokenRole' has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedName: "KnownError",
		},
		{
			name:         "NamedError error message updated no args",
			err:          KnownError{FromError: errors.New("Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action")},
			ctx:          ErrorContext{},
			expectedMsg:  "Check if image exists and role has permissions to pull images from Amazon ECR. Error response from daemon: pull access denied for 123123123123.dkr.ecr.us-east-1.amazonaws.com/my_image, repository does not exist or may require 'docker login': denied: User: arn:aws:sts::123123123123:assumed-role/MyBrokenRole/xyz is not authorized to perform: ecr:BatchGetImage on resource: arn:aws:ecr:us-east-1:123123123123:repository/test_image because no identity-based policy allows the ecr:BatchGetImage action",
			expectedName: "KnownError",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			augmentedErr := AugmentNamedErrMsg(tc.err, tc.ctx)
			require.Equal(t, augmentedErr.Error(), tc.expectedMsg)
			require.Equal(t, augmentedErr.ErrorName(), tc.expectedName)
		})
	}
}
