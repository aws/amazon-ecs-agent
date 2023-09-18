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
	bolt "go.etcd.io/bbolt"
)

const (
	AgentVersionKey         = "agent-version"
	AvailabilityZoneKey     = "availability-zone"
	ClusterNameKey          = "cluster-name"
	ContainerInstanceARNKey = "container-instance-arn"
	EC2InstanceIDKey        = "ec2-instance-id"
	TaskManifestSeqNumKey   = "task-manifest-seq-num"
)

func (c *client) SaveMetadata(key, val string) error {
	return c.DB.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		return c.Accessor.PutObject(b, key, val)
	})
}

func (c *client) GetMetadata(key string) (string, error) {
	var val string
	err := c.DB.View(func(tx *bolt.Tx) error {
		return c.Accessor.GetObject(tx, metadataBucketName, key, &val)
	})
	return val, err
}
