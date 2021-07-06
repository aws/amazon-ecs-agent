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
	"path/filepath"
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/engine/image"

	bolt "go.etcd.io/bbolt"
)

const (
	dbName = "agent.db"
	dbMode = 0600

	containersBucketName     = "containers"
	tasksBucketName          = "tasks"
	imagesBucketName         = "images"
	eniAttachmentsBucketName = "eniattachments"
	metadataBucketName       = "metadata"
)

var (
	dbClient Client
	once     sync.Once

	buckets = []string{
		imagesBucketName,
		containersBucketName,
		tasksBucketName,
		eniAttachmentsBucketName,
		metadataBucketName,
	}
)

// Client specifies the data management interface to persist and manage various kinds of data in the agent.
type Client interface {
	// SaveContainer saves the data of a container.
	SaveContainer(*container.Container) error
	// SaveDockerContainer saves the data of a docker container.
	// We have both SaveContainer and SaveDockerContainer so that a caller who doesn't have docker information
	// of the container can save container with SaveContainer, while a caller who wants to save the docker
	// information of the container can save it with SaveDockerContainer.
	SaveDockerContainer(*container.DockerContainer) error
	// DeleteContainer deletes the data of a container.
	DeleteContainer(string) error
	// GetContainers gets the data of all the containers.
	GetContainers() ([]*container.DockerContainer, error)

	// SaveTask saves the data of a task.
	SaveTask(*task.Task) error
	// DeleteTask deletes the data of a task.
	DeleteTask(string) error
	// GetTasks gets the data of all the tasks.
	GetTasks() ([]*task.Task, error)

	// SaveImageState saves the data of an image state.
	SaveImageState(*image.ImageState) error
	// DeleteImageState deletes the data of an image state.
	DeleteImageState(string) error
	// GetImageStates gets the data of all the image states.
	GetImageStates() ([]*image.ImageState, error)

	// SaveENIAttachment saves the data of an ENI attachment.
	SaveENIAttachment(*eni.ENIAttachment) error
	// DeleteENIAttachment deletes the data of an ENI atttachment.
	DeleteENIAttachment(string) error
	// GetENIAttachments gets the data of all the ENI attachment.
	GetENIAttachments() ([]*eni.ENIAttachment, error)

	// SaveMetadata saves a key value pair of metadata.
	SaveMetadata(string, string) error
	// GetMetadata gets the value of a certain kind of metadata.
	GetMetadata(string) (string, error)

	// Close closes the connection to database.
	Close() error
}

// client implements the Client interface using boltdb as the backing data store.
type client struct {
	db *bolt.DB
}

// New returns a data client that implements the Client interface with boltdb.
func New(dataDir string) (Client, error) {
	var err error
	once.Do(func() {
		dbClient, err = setup(dataDir)
	})
	if err != nil {
		return nil, err
	}
	return dbClient, nil
}

// NewWithSetup returns a data client that implements the Client interface with boltdb.
// It always runs the db setup. Used for testing.
func NewWithSetup(dataDir string) (Client, error) {
	return setup(dataDir)
}

// setup initiates the boltdb client and makes sure the buckets we use are created.
func setup(dataDir string) (*client, error) {
	db, err := bolt.Open(filepath.Join(dataDir, dbName), dbMode, nil)
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			_, err = tx.CreateBucketIfNotExists([]byte(b))
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &client{
		db: db,
	}, nil
}

// Close closes the boltdb connection.
func (c *client) Close() error {
	return c.db.Close()
}
