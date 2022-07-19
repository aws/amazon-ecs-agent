//go:build !linux
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

package serviceconnect

import (
	"fmt"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/serviceconnect"
	dockercontainer "github.com/docker/docker/api/types/container"
)

type manager struct {
}

func NewManager(serviceConnectLoader serviceconnect.Loader) Manager {
	return &manager{}
}

func (m *manager) AugmentTaskContainer(*apitask.Task, *apicontainer.Container, *dockercontainer.HostConfig) error {
	return fmt.Errorf("ServiceConnect is only supported on linux")
}
func (m *manager) CreateInstanceTask(config *config.Config) (*apitask.Task, error) {
	return nil, fmt.Errorf("ServiceConnect is only supported on linux")
}
func (m *manager) AugmentInstanceContainer(*apitask.Task, *apicontainer.Container, *dockercontainer.HostConfig) error {
	return fmt.Errorf("ServiceConnect is only supported on linux")
}
