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

const dockerTimeoutErrorName = "DockerTimeoutError"

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

type DockerTimeoutError struct {
	duration   time.Duration
	transition string
}

func (err *DockerTimeoutError) Error() string {
	return "Could not transition to " + err.transition + "; timed out after waiting " + err.duration.String()
}
func (err *DockerTimeoutError) ErrorName() string { return dockerTimeoutErrorName }

type ContainerVanishedError struct{}

func (err ContainerVanishedError) Error() string     { return "No container matching saved ID found" }
func (err ContainerVanishedError) ErrorName() string { return "ContainerVanishedError" }

type CannotXContainerError struct {
	transition string
	msg        string
}

func (err CannotXContainerError) Error() string { return err.msg }
func (err CannotXContainerError) ErrorName() string {
	return "Cannot" + err.transition + "ContainerError"
}

type OutOfMemoryError struct{}

func (err OutOfMemoryError) Error() string     { return "Container killed due to memory usage" }
func (err OutOfMemoryError) ErrorName() string { return "OutOfMemoryError" }

// DockerStateError is a wrapper around the error docker puts in the '.State.Error' field of its inspect output.
type DockerStateError struct {
	dockerError string
	name        string
}

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
func (err DockerStateError) ErrorName() string {
	return err.name
}

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

func (CannotGetDockerClientError) ErrorName() string {
	return "CannotGetDockerclientError"
}

type TaskStoppedBeforePullBeginError struct {
	taskArn string
}

func (err TaskStoppedBeforePullBeginError) Error() string {
	return "Task stopped before image pull could begin for task: " + err.taskArn
}

func (TaskStoppedBeforePullBeginError) ErrorName() string {
	return "TaskStoppedBeforePullBeginError"
}
