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

package dockerapi

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"
)

const (
	// DockerTimeoutErrorName is the name of docker timeout error.
	DockerTimeoutErrorName = "DockerTimeoutError"
	// CannotInspectContainerErrorName is the name of container inspect error.
	CannotInspectContainerErrorName = "CannotInspectContainerError"
	// CannotStartContainerErrorName is the name of container start error.
	CannotStartContainerErrorName = "CannotStartContainerError"
	// CannotDescribeContainerErrorName is the name of describe container error.
	CannotDescribeContainerErrorName = "CannotDescribeContainerError"
	// CannotGetContainerTopErrorName is the name of the top container error.
	CannotGetContainerTopErrorName = "CannotGetContainerTopError"
	// TopProcessNotFoundErrorName is the error thrown when the specified pid does
	// not exist in the container
	TopProcessNotFoundErrorName = "ps: exit status 1"
)

// DockerTimeoutError is an error type for describing timeouts
type DockerTimeoutError struct {
	// Duration is the timeout period.
	Duration time.Duration
	// Transition is the description of operation that timed out.
	Transition string
}

func (err *DockerTimeoutError) Error() string {
	return "Could not transition to " + err.Transition + "; timed out after waiting " + err.Duration.String()
}

// ErrorName returns the name of the error
func (err *DockerTimeoutError) ErrorName() string { return DockerTimeoutErrorName }

// IsRetriableError returns a boolean indicating whether the call that
// generated the error can be retried.
func (err DockerTimeoutError) IsRetriableError() bool {
	return true
}

// OutOfMemoryError is a type for errors caused by running out of memory
type OutOfMemoryError struct{}

func (err OutOfMemoryError) Error() string { return "Container killed due to memory usage" }

// ErrorName returns the name of the error
func (err OutOfMemoryError) ErrorName() string { return "OutOfMemoryError" }

// DockerStateError is a wrapper around the error docker puts in the '.State.Error' field of its inspect output.
type DockerStateError struct {
	dockerError string
	name        string
}

// NewDockerStateError creates a DockerStateError
func NewDockerStateError(err string) DockerStateError {
	// Add stringmatching logic as needed to provide better output than docker
	return DockerStateError{
		dockerError: err,
		name:        "DockerStateError",
	}
}

func (err DockerStateError) Error() string {
	return err.dockerError
}

// ErrorName returns the name of the DockerStateError.
func (err DockerStateError) ErrorName() string {
	return err.name
}

// CannotGetDockerClientError is a type for failing to get a specific Docker client
type CannotGetDockerClientError struct {
	version dockerclient.DockerVersion
	err     error
}

func (c CannotGetDockerClientError) Error() string {
	if c.version != "" {
		return "(v" + string(c.version) + ") - " + c.err.Error()
	}
	return c.err.Error()
}

// ErrorName returns the name of the CannotGetDockerClientError.
func (CannotGetDockerClientError) ErrorName() string {
	return "CannotGetDockerclientError"
}

// CannotStopContainerError indicates any error when trying to stop a container
type CannotStopContainerError struct {
	FromError error
}

func (err CannotStopContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotStopContainerError.
func (err CannotStopContainerError) ErrorName() string {
	return "CannotStopContainerError"
}

// IsRetriableError returns a boolean indicating whether the call that
// generated the error can be retried.
// When stopping a container, most errors that we can get should be
// considered retriable. However, in the case where the container is
// already stopped or doesn't exist at all, there's no sense in
// retrying.
func (err CannotStopContainerError) IsRetriableError() bool {
	if _, ok := err.FromError.(NoSuchContainerError); ok {
		return false
	}

	return true
}

// CannotPullContainerError indicates any error when trying to pull
// a container image
type CannotPullContainerError struct {
	FromError error
}

func (err CannotPullContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotPullContainerError.
func (err CannotPullContainerError) ErrorName() string {
	return "CannotPullContainerError"
}

func (err CannotPullContainerError) WithAugmentedErrorMessage(msg string) apierrors.NamedError {
	return CannotPullContainerError{errors.New(msg)}
}

// CannotPullImageManifestError indicates any error when trying to pull a container image manifest.
type CannotPullImageManifestError struct {
	FromError error
}

func (err CannotPullImageManifestError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns the name of CannotPullImageManifestError.
func (err CannotPullImageManifestError) ErrorName() string {
	return "CannotPullImageManifestError"
}

// CannotPullECRContainerError indicates any error when trying to pull
// a container image from ECR
type CannotPullECRContainerError struct {
	FromError error
}

func (err CannotPullECRContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotPullECRContainerError.
func (err CannotPullECRContainerError) ErrorName() string {
	return "CannotPullECRContainerError"
}

// Retry fulfills the utils.Retrier interface and allows retries to be skipped by utils.Retry* functions
func (err CannotPullECRContainerError) Retry() bool {
	return false
}

func (err CannotPullECRContainerError) WithAugmentedErrorMessage(msg string) apierrors.NamedError {
	return CannotPullECRContainerError{errors.New(msg)}
}

// CannotPullContainerAuthError indicates any error when trying to pull
// a container image
type CannotPullContainerAuthError struct {
	FromError error
}

func (err CannotPullContainerAuthError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotPullContainerAuthError.
func (err CannotPullContainerAuthError) ErrorName() string {
	return "CannotPullContainerAuthError"
}

// Retry fulfills the utils.Retrier interface and allows retries to be skipped by utils.Retry* functions
func (err CannotPullContainerAuthError) Retry() bool {
	return false
}

// CannotCreateContainerError indicates any error when trying to create a container
type CannotCreateContainerError struct {
	FromError error
}

func (err CannotCreateContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotCreateContainerError.
func (err CannotCreateContainerError) ErrorName() string {
	return "CannotCreateContainerError"
}

// CannotStartContainerError indicates any error when trying to start a container
type CannotStartContainerError struct {
	FromError error
}

func (err CannotStartContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotStartContainerError
func (err CannotStartContainerError) ErrorName() string {
	return CannotStartContainerErrorName
}

// CannotInspectContainerError indicates any error when trying to inspect a container
type CannotInspectContainerError struct {
	FromError error
}

func (err CannotInspectContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotInspectContainerError
func (err CannotInspectContainerError) ErrorName() string {
	return CannotInspectContainerErrorName
}

// CannotGetContainerTopError indicates any error when trying to get container top processes
type CannotGetContainerTopError struct {
	FromError error
}

func (err CannotGetContainerTopError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotGetContainerTopError
func (err CannotGetContainerTopError) ErrorName() string {
	return CannotGetContainerTopErrorName
}

// CannotRemoveContainerError indicates any error when trying to remove a container
type CannotRemoveContainerError struct {
	FromError error
}

func (err CannotRemoveContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotRemoveContainerError
func (err CannotRemoveContainerError) ErrorName() string {
	return "CannotRemoveContainerError"
}

// CannotDescribeContainerError indicates any error when trying to describe a container
type CannotDescribeContainerError struct {
	FromError error
}

func (err CannotDescribeContainerError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotDescribeContainerError
func (err CannotDescribeContainerError) ErrorName() string {
	return CannotDescribeContainerErrorName
}

// CannotListContainersError indicates any error when trying to list containers
type CannotListContainersError struct {
	FromError error
}

func (err CannotListContainersError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotListContainersError
func (err CannotListContainersError) ErrorName() string {
	return "CannotListContainersError"
}

type CannotListImagesError struct {
	FromError error
}

func (err CannotListImagesError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotListImagesError
func (err CannotListImagesError) ErrorName() string {
	return "CannotListImagesError"
}

// CannotCreateVolumeError indicates any error when trying to create a volume
type CannotCreateVolumeError struct {
	fromError error
}

func (err CannotCreateVolumeError) Error() string {
	return err.fromError.Error()
}

func (err CannotCreateVolumeError) ErrorName() string {
	return "CannotCreateVolumeError"
}

// CannotInspectVolumeError indicates any error when trying to inspect a volume
type CannotInspectVolumeError struct {
	fromError error
}

func (err CannotInspectVolumeError) Error() string {
	return err.fromError.Error()
}

func (err CannotInspectVolumeError) ErrorName() string {
	return "CannotInspectVolumeError"
}

// CannotRemoveVolumeError indicates any error when trying to inspect a volume
type CannotRemoveVolumeError struct {
	fromError error
}

func (err CannotRemoveVolumeError) Error() string {
	return err.fromError.Error()
}

func (err CannotRemoveVolumeError) ErrorName() string {
	return "CannotRemoveVolumeError"
}

// CannotListPluginsError indicates any error when trying to list docker plugins
type CannotListPluginsError struct {
	fromError error
}

func (err CannotListPluginsError) Error() string {
	return err.fromError.Error()
}

func (err CannotListPluginsError) ErrorName() string {
	return "CannotListPluginsError"
}

// NoSuchContainerError indicates error when a given container is not found.
type NoSuchContainerError struct {
	ID string
}

func (err NoSuchContainerError) Error() string {
	return "Container not found: " + err.ID
}

func (err NoSuchContainerError) ErrorName() string {
	return "NoSuchContainerError"
}

// CannotCreateContainerExecError indicates any error when trying to create an exec object
type CannotCreateContainerExecError struct {
	FromError error
}

func (err CannotCreateContainerExecError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotCreateContainerExecError.
func (err CannotCreateContainerExecError) ErrorName() string {
	return "CannotCreateContainerExecError"
}

// CannotStartContainerExecError indicates any error when trying to start an exec process
type CannotStartContainerExecError struct {
	FromError error
}

func (err CannotStartContainerExecError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotCreateContainerExecError.
func (err CannotStartContainerExecError) ErrorName() string {
	return "CannotStartContainerExecError"
}

// CannotInspectContainerExecError indicates any error when trying to start an exec process
type CannotInspectContainerExecError struct {
	FromError error
}

func (err CannotInspectContainerExecError) Error() string {
	return err.FromError.Error()
}

// ErrorName returns name of the CannotCreateContainerExecError.
func (err CannotInspectContainerExecError) ErrorName() string {
	return "CannotInspectContainerExecError"
}

// Redact ECR bucket urls from error string
// Return a new error with redacted string - replacing ECR bucket (*starport-layer-bucket*) urls with a string.
// This is done because container runtime's request may sometimes contain security tokens when accessing ECR buckets for image layers.
// When these requests error out, the URLs with secrets may get bubbled up to Agent logs.
// So we redact the otherwise hidden URLs for security.
func redactEcrUrls(overrideStr string, err error) error {
	if err == nil {
		return nil
	}
	urlRegex := regexp.MustCompile(`\"?https[^\s]+starport-layer-bucket[^\s]+`)
	redactedStr := urlRegex.ReplaceAllString(err.Error(), fmt.Sprintf("REDACTED ECR URL related to %s", overrideStr))
	return errors.New(redactedStr)
}
