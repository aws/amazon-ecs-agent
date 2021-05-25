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
	return c.db.Batch(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metadataBucketName))
		return putObject(b, key, val)
	})
}

func (c *client) GetMetadata(key string) (string, error) {
	var val string
	err := c.db.View(func(tx *bolt.Tx) error {
		return getObject(tx, metadataBucketName, key, &val)
	})
	return val, err
}
