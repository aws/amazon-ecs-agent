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

	"github.com/aws/amazon-ecs-agent/agent/utils"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

func (c *client) SaveENIAttachment(eni *ni.ENIAttachment) error {
	id, err := utils.GetENIAttachmentId(eni.AttachmentARN)
	if err != nil {
		return errors.Wrap(err, "failed to generate database id")
	}
	return c.DB.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(eniAttachmentsBucketName))
		return c.Accessor.PutObject(b, id, eni)
	})
}

func (c *client) DeleteENIAttachment(id string) error {
	return c.DB.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(eniAttachmentsBucketName))
		return b.Delete([]byte(id))
	})
}

func (c *client) GetENIAttachments() ([]*ni.ENIAttachment, error) {
	var eniAttachments []*ni.ENIAttachment
	err := c.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(eniAttachmentsBucketName))
		return c.Accessor.Walk(bucket, func(id string, data []byte) error {
			eniAttachment := ni.ENIAttachment{}
			if err := json.Unmarshal(data, &eniAttachment); err != nil {
				return err
			}
			eniAttachments = append(eniAttachments, &eniAttachment)
			return nil
		})
	})
	return eniAttachments, err
}
