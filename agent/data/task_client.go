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

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// SaveTask saves a task to the task bucket.
func (c *client) SaveTask(task *apitask.Task) error {
	id, err := utils.GetTaskID(task.Arn)
	if err != nil {
		return errors.Wrap(err, "failed to generate database id")
	}
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tasksBucketName))
		return putObject(b, id, task)
	})
}

// DeleteTask deletes a task from the task bucket.
func (c *client) DeleteTask(id string) error {
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tasksBucketName))
		return b.Delete([]byte(id))
	})
}

// GetTasks returns all the tasks in the task bucket.
func (c *client) GetTasks() ([]*apitask.Task, error) {
	var tasks []*apitask.Task
	err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(tasksBucketName))
		return walk(bucket, func(id string, data []byte) error {
			task := apitask.Task{}
			if err := json.Unmarshal(data, &task); err != nil {
				return err
			}
			tasks = append(tasks, &task)
			return nil
		})
	})
	return tasks, err
}
