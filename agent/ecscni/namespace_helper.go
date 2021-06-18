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

package ecscni

import (
	"context"

	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/containernetworking/cni/pkg/types/current"
)

// NamespaceHelper defines the methods for performing additional actions to setup/clean the task namespace.
// Task namespace in awsvpc network mode is configured using pause container which is the first container
// launched for the task. These commands are executed inside that container.
type NamespaceHelper interface {
	ConfigureTaskNamespaceRouting(ctx context.Context, taskENI *apieni.ENI, config *Config, result *current.Result) error
}

// helper is the client for executing methods of NamespaceHelper interface.
type helper struct {
	dockerClient dockerapi.DockerClient
}

// NewNamespaceHelper returns a new instance of NamespaceHelper interface.
func NewNamespaceHelper(client dockerapi.DockerClient) NamespaceHelper {
	return &helper{dockerClient: client}
}
