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
	"encoding/json"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// SaveDockerContainer saves a docker container to the container bucket.
func (c *client) SaveDockerContainer(container *apicontainer.DockerContainer) error {
	id, err := GetContainerID(container.Container)
	if err != nil {
		return errors.Wrap(err, "failed to generate database id")
	}
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(containersBucketName))
		return putObject(b, id, container)
	})
}

// SaveContainer saves an apicontainer.Container to the container bucket. If a corresponding
// apicontainer.DockerContainer exists, this updates the Container part of it; otherwise, a new
// apicontainer.DockerContainer is created and saved.
func (c *client) SaveContainer(container *apicontainer.Container) error {
	id, err := GetContainerID(container)
	if err != nil {
		return errors.Wrap(err, "failed to generate database id")
	}

	dockerContainer, err := c.getDockerContainer(id)
	if err != nil {
		dockerContainer = &apicontainer.DockerContainer{}
	}
	dockerContainer.Container = container
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(containersBucketName))
		return putObject(b, id, dockerContainer)
	})
}

func (c *client) getDockerContainer(id string) (*apicontainer.DockerContainer, error) {
	container := &apicontainer.DockerContainer{}
	err := c.db.View(func(tx *bolt.Tx) error {
		return getObject(tx, containersBucketName, id, container)
	})
	return container, err
}

// DeleteContainer deletes a container from the container bucket.
func (c *client) DeleteContainer(id string) error {
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(containersBucketName))
		return b.Delete([]byte(id))
	})
}

// GetContainers returns all the containers in the container bucket.
func (c *client) GetContainers() ([]*apicontainer.DockerContainer, error) {
	var containers []*apicontainer.DockerContainer
	err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(containersBucketName))
		return walk(bucket, func(id string, data []byte) error {
			container := apicontainer.DockerContainer{}
			if err := json.Unmarshal(data, &container); err != nil {
				return err
			}
			containers = append(containers, &container)
			return nil
		})
	})
	return containers, err
}

// GetContainerID returns a unique ID for a container to use as key when saving to DB.
func GetContainerID(c *apicontainer.Container) (string, error) {
	taskID, err := utils.GetTaskID(c.GetTaskARN())
	if err != nil {
		return "", err
	}
	return taskID + "-" + c.Name, nil
}
