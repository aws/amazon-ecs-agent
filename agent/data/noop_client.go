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

package data

import (
	"github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"
)

type noopClient struct{}

// NewNoopClient returns a client that implements the data interface with no-op. It is used
// by the agent when ECS_CHECKPOINT is set to false, and is also used in testing.
func NewNoopClient() Client {
	return &noopClient{}
}

func (c *noopClient) SaveContainer(*container.Container) error {
	return nil
}

func (c *noopClient) SaveDockerContainer(*container.DockerContainer) error {
	return nil
}

func (c *noopClient) DeleteContainer(string) error {
	return nil
}

func (c *noopClient) GetContainers() ([]*container.DockerContainer, error) {
	return nil, nil
}

func (c *noopClient) SaveTask(*task.Task) error {
	return nil
}

func (c *noopClient) DeleteTask(string) error {
	return nil
}

func (c *noopClient) GetTasks() ([]*task.Task, error) {
	return nil, nil
}

func (c *noopClient) SaveImageState(*image.ImageState) error {
	return nil
}

func (c *noopClient) DeleteImageState(string) error {
	return nil
}

func (c *noopClient) GetImageStates() ([]*image.ImageState, error) {
	return nil, nil
}

func (c *noopClient) SaveENIAttachment(*eni.ENIAttachment) error {
	return nil
}

func (c *noopClient) DeleteENIAttachment(string) error {
	return nil
}

func (c *noopClient) GetENIAttachments() ([]*eni.ENIAttachment, error) {
	return nil, nil
}

func (c *noopClient) SaveMetadata(string, string) error {
	return nil
}

func (c *noopClient) GetMetadata(string) (string, error) {
	return "", nil
}

func (c *noopClient) Close() error {
	return nil
}
