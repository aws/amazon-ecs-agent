//go:build !linux

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

package serviceconnect

import (
	"errors"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	dockercontainer "github.com/docker/docker/api/types/container"
)

type manager struct {
}

func NewManager() Manager {
	return &manager{}
}

func (m *manager) AugmentTaskContainer(task *apitask.Task, container *apicontainer.Container, hostConfig *dockercontainer.HostConfig) error {
	return errors.New("ServiceConnect is only supported on linux")
}
