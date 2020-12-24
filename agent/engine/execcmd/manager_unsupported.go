// +build !linux

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
package execcmd

import (
	"context"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
)

const (
	// HostLogDir here is used used while cleaning up exec logs when task exits.
	// When this path is empty, nothing is cleaned up for unsupported platforms.
	HostLogDir = ""
)

// Note: exec cmd agent is a linux-only feature, thus implemented here as a no-op.
func (m *manager) RestartAgentIfStopped(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) (RestartStatus, error) {
	return NotRestarted, nil
}

// Note: exec cmd agent is a linux-only feature, thus implemented here as a no-op.
func (m *manager) StartAgent(ctx context.Context, client dockerapi.DockerClient, task *apitask.Task, container *apicontainer.Container, containerId string) error {
	return nil
}

// InitializeTask adds the necessary volumes and mount points in all of the task containers in order for the
// exec cmd agent to run upon container start up.
// Note: exec cmd agent is a linux-only feature, thus implemented here as a no-op.
func (m *manager) InitializeTask(task *apitask.Task) error {
	return nil
}
