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
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

// InstanceTypeChangedErrorMessage is the error message to print for the
// instance type changed error when registering a container instance
const InstanceTypeChangedErrorMessage = "Container instance type changes are not supported."

const ClusterNotFoundErrorMessage = "Cluster not found."

// IsInstanceTypeChangedError returns true if the error when
// registering the container instance is because of instance type being
// changed
func IsInstanceTypeChangedError(err error) bool {
	if awserr, ok := err.(awserr.Error); ok {
		return strings.Contains(awserr.Message(), InstanceTypeChangedErrorMessage)
	}
	return false
}

func IsClusterNotFoundError(err error) bool {
	if awserr, ok := err.(awserr.Error); ok {
		return strings.Contains(awserr.Message(), ClusterNotFoundErrorMessage)
	}
	return false
}

// BadVolumeError represents an error caused by bad volume
type BadVolumeError struct {
	Msg string
}

func (err *BadVolumeError) Error() string { return err.Msg }

// ErrorName returns name of the BadVolumeError
func (err *BadVolumeError) ErrorName() string { return "InvalidVolumeError" }

// Retry implements Retirable interface
func (err *BadVolumeError) Retry() bool { return false }

// DefaultNamedError is a wrapper type for 'error' which adds an optional name and provides a symmetric
// marshal/unmarshal
type DefaultNamedError struct {
	Err  string `json:"error"`
	Name string `json:"name"`
}

// Error implements error
func (err *DefaultNamedError) Error() string {
	if err.Name == "" {
		return "UnknownError: " + err.Err
	}
	return err.Name + ": " + err.Err
}

// ErrorName implements NamedError
func (err *DefaultNamedError) ErrorName() string {
	return err.Name
}

// NewNamedError creates a NamedError.
func NewNamedError(err error) *DefaultNamedError {
	if namedErr, ok := err.(NamedError); ok {
		return &DefaultNamedError{Err: namedErr.Error(), Name: namedErr.ErrorName()}
	}
	return &DefaultNamedError{Err: err.Error()}
}

// HostConfigError represents an error caused by host configuration
type HostConfigError struct {
	Msg string
}

// Error returns the error as a string
func (err *HostConfigError) Error() string { return err.Msg }

// ErrorName returns the name of the error
func (err *HostConfigError) ErrorName() string { return "HostConfigError" }

// DockerClientConfigError represents the error caused by docker client
type DockerClientConfigError struct {
	Msg string
}

// Error returns the error as a string
func (err *DockerClientConfigError) Error() string { return err.Msg }

// ErrorName returns the name of the error
func (err *DockerClientConfigError) ErrorName() string { return "DockerClientConfigError" }

// ResourceInitError is a task error for which a required resource cannot
// be initialized
type ResourceInitError struct {
	taskARN string
	origErr error
}

// NewResourceInitError creates an error for resource initialize failure
func NewResourceInitError(taskARN string, origErr error) *ResourceInitError {
	return &ResourceInitError{taskARN, origErr}
}

// Error returns the error as a string
func (err *ResourceInitError) Error() string {
	return fmt.Sprintf("resource cannot be initialized for task %s: %v", err.taskARN, err.origErr)
}

// ErrorName is the name of the error
func (err *ResourceInitError) ErrorName() string {
	return "ResourceInitializationError"
}
