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

	"github.com/aws/amazon-ecs-agent/agent/engine/image"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

func (c *client) SaveImageState(img *image.ImageState) error {
	id := img.GetImageID()
	if id == "" {
		return errors.New("failed to generate database image id")
	}
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(imagesBucketName))
		return putObject(b, id, img)
	})
}

func (c *client) DeleteImageState(id string) error {
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(imagesBucketName))
		return b.Delete([]byte(id))
	})
}

func (c *client) GetImageStates() ([]*image.ImageState, error) {
	var imageStates []*image.ImageState
	err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(imagesBucketName))
		return walk(bucket, func(id string, data []byte) error {
			imageState := image.ImageState{}
			if err := json.Unmarshal(data, &imageState); err != nil {
				return err
			}
			imageStates = append(imageStates, &imageState)
			return nil
		})
	})
	return imageStates, err
}
