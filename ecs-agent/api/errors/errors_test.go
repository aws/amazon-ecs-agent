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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
)

func TestIsInstanceTypeChangedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Instance type changed error",
			err:  &smithy.GenericAPIError{Code: "Error", Message: InstanceTypeChangedErrorMessage},
			want: true,
		},
		{
			name: "Other error",
			err:  errors.New("Some other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsInstanceTypeChangedError(tt.err))
		})
	}
}

func TestIsClusterNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Cluster not found error",
			err:  &types.ClusterNotFoundException{},
			want: true,
		},
		{
			name: "Cluster not found message",
			err:  &smithy.GenericAPIError{Code: "Error", Message: ClusterNotFoundErrorMessage},
			want: true,
		},
		{
			name: "ClientException with cluster not found message",
			err:  &types.ClientException{Message: aws.String(ClusterNotFoundErrorMessage)},
			want: true,
		},
		{
			name: "Other error",
			err:  errors.New("Some other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsClusterNotFoundError(tt.err))
		})
	}
}

func TestBadVolumeError(t *testing.T) {
	err := &BadVolumeError{Msg: "Bad volume"}
	assert.Equal(t, "Bad volume", err.Error())
	assert.Equal(t, "InvalidVolumeError", err.ErrorName())
	assert.False(t, err.Retry())
}

func TestDefaultNamedError(t *testing.T) {
	err := &DefaultNamedError{Err: "Test error", Name: "TestError"}
	assert.Equal(t, "TestError: Test error", err.Error())
	assert.Equal(t, "TestError", err.ErrorName())

	errNoName := &DefaultNamedError{Err: "Test error"}
	assert.Equal(t, "UnknownError: Test error", errNoName.Error())
}

func TestNewNamedError(t *testing.T) {
	namedErr := &BadVolumeError{Msg: "Bad volume"}
	err := NewNamedError(namedErr)
	assert.Equal(t, "InvalidVolumeError: Bad volume", err.Error())

	plainErr := errors.New("Plain error")
	err = NewNamedError(plainErr)
	assert.Equal(t, "UnknownError: Plain error", err.Error())
}

func TestHostConfigError(t *testing.T) {
	err := &HostConfigError{Msg: "Host config error"}
	assert.Equal(t, "Host config error", err.Error())
	assert.Equal(t, "HostConfigError", err.ErrorName())
}

func TestDockerClientConfigError(t *testing.T) {
	err := &DockerClientConfigError{Msg: "Docker client config error"}
	assert.Equal(t, "Docker client config error", err.Error())
	assert.Equal(t, "DockerClientConfigError", err.ErrorName())
}

func TestResourceInitError(t *testing.T) {
	origErr := errors.New("Original error")
	err := NewResourceInitError("task-arn", origErr)
	expectedMsg := "resource cannot be initialized for task task-arn: Original error"
	assert.Equal(t, expectedMsg, err.Error())
	assert.Equal(t, "ResourceInitializationError", err.ErrorName())
}
