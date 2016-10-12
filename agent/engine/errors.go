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

package engine

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
)

const (
	dockerTimeoutErrorName     = "DockerTimeoutError"
	unretriableDockerErrorName = "UnretriableDockerError"
	dockerStateErrorName       = "DockerStateError"
)

// engineError wraps the error interface with an identifier method that
// is used to classify the error type
type engineError interface {
	error
	ErrorName() string
}

// impossibleTransitionError is an error that occurs when an event causes a
// container to try and transition to a state that it cannot be moved to
type impossibleTransitionError struct {
	state api.ContainerStatus
}

func (err *impossibleTransitionError) Error() string {
	return "Cannot transition to " + err.state.String()
}
func (err *impossibleTransitionError) ErrorName() string { return "ImpossibleStateTransitionError" }

// DockerTimeoutError is an error type for describing timeouts
type DockerTimeoutError struct {
	duration   time.Duration
	transition string
}

func (err *DockerTimeoutError) Error() string {
	return "Could not transition to " + err.transition + "; timed out after waiting " + err.duration.String()
}

// ErrorName returns the name of the error
func (err *DockerTimeoutError) ErrorName() string { return dockerTimeoutErrorName }

// ContainerVanishedError is a type for describing a container that does not exist
type ContainerVanishedError struct{}

func (err ContainerVanishedError) Error() string { return "No container matching saved ID found" }

// ErrorName returns the name of the error
func (err ContainerVanishedError) ErrorName() string { return "ContainerVanishedError" }

// CannotXContainerError is a type for errors involving containers
type CannotXContainerError struct {
	transition string
	msg        string
}

func (err CannotXContainerError) Error() string { return err.msg }

// ErrorName returns the name of the error
func (err CannotXContainerError) ErrorName() string {
	return "Cannot" + err.transition + "ContainerError"
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
		name:        dockerStateErrorName,
	}
}

func (err DockerStateError) Error() string {
	return err.dockerError
}

// ErrorName returns the name of the error
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

// ErrorName returns the name of the error
func (CannotGetDockerClientError) ErrorName() string {
	return "CannotGetDockerclientError"
}

// TaskStoppedBeforePullBeginError is a type for task errors involving pull
type TaskStoppedBeforePullBeginError struct {
	taskArn string
}

func (err TaskStoppedBeforePullBeginError) Error() string {
	return "Task stopped before image pull could begin for task: " + err.taskArn
}

// ErrorName returns the name of the error
func (TaskStoppedBeforePullBeginError) ErrorName() string {
	return "TaskStoppedBeforePullBeginError"
}

type UnretriableDockerError struct {
	dockerError error
}

func (err UnretriableDockerError) Error() string {
	return err.dockerError.Error()
}

func (UnretriableDockerError) ErrorName() string {
	return unretriableDockerErrorName
}
