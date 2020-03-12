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

package engine

import (
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"
)

type cannotStopContainerError interface {
	apierrors.NamedError
	IsRetriableError() bool
}

// impossibleTransitionError is an error that occurs when an event causes a
// container to try and transition to a state that it cannot be moved to
type impossibleTransitionError struct {
	state apicontainerstatus.ContainerStatus
}

func (err *impossibleTransitionError) Error() string {
	return "Cannot transition to " + err.state.String()
}
func (err *impossibleTransitionError) ErrorName() string { return "ImpossibleStateTransitionError" }

// ContainerVanishedError is a type for describing a container that does not exist
type ContainerVanishedError struct{}

func (err ContainerVanishedError) Error() string { return "No container matching saved ID found" }

// ErrorName returns the name of the error
func (err ContainerVanishedError) ErrorName() string { return "ContainerVanishedError" }

// TaskDependencyError is the error for task that dependencies can't
// be resolved
type TaskDependencyError struct {
	taskArn string
}

func (err TaskDependencyError) Error() string {
	return "Task dependencies cannot be resolved, taskArn: " + err.taskArn
}

// ErrorName is the name of the error
func (err TaskDependencyError) ErrorName() string {
	return "TaskDependencyError"
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

// ContainerNetworkingError indicates any error when dealing with the network
// namespace of container
type ContainerNetworkingError struct {
	fromError error
}

func (err ContainerNetworkingError) Error() string {
	return err.fromError.Error()
}

func (err ContainerNetworkingError) ErrorName() string {
	return "ContainerNetworkingError"
}

// CannotGetDockerClientVersionError indicates error when trying to get docker
// client api version
type CannotGetDockerClientVersionError struct {
	fromError error
}

func (err CannotGetDockerClientVersionError) ErrorName() string {
	return "CannotGetDockerClientVersionError"
}
func (err CannotGetDockerClientVersionError) Error() string {
	return err.fromError.Error()
}
