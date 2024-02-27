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
	"bytes"
	"encoding/json"

	"github.com/aws/amazon-ecs-agent/ecs-agent/modeltransformer"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

type Client struct {
	Accessor    DBAccessor
	DB          *bolt.DB
	Transformer *modeltransformer.Transformer
}

type DBAccessor struct{}

func (DBAccessor) PutObject(bucket *bolt.Bucket, key string, obj interface{}) error {
	keyBytes := []byte(key)
	data, err := json.Marshal(obj)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal object with key %q", key)
	}

	if err := bucket.Put(keyBytes, data); err != nil {
		return errors.Wrapf(err, "failed to insert object with key %q", key)
	}

	return nil
}

func (DBAccessor) GetObject(tx *bolt.Tx, bucketName, id string, out interface{}) error {
	bucket := tx.Bucket([]byte(bucketName))
	data := bucket.Get([]byte(id))
	if data == nil {
		return errors.Errorf("object %s not found in bucket %s", id, bucketName)
	}

	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return errors.Wrapf(err, "failed to unmarshal object with key %q", id)
		}
	}

	return nil
}

func (DBAccessor) Walk(bucket *bolt.Bucket, callback func(id string, data []byte) error) error {
	cursor := bucket.Cursor()

	for id, data := cursor.First(); id != nil; id, data = cursor.Next() {
		if err := callback(string(id), data); err != nil {
			return err
		}
	}

	return nil
}

func (DBAccessor) WalkPrefix(bucket *bolt.Bucket, prefix string, callback func(id string, data []byte) error) error {
	var (
		cursor = bucket.Cursor()
		search = []byte(prefix)
	)

	for id, data := cursor.Seek(search); id != nil && bytes.HasPrefix(id, search); id, data = cursor.Next() {
		if err := callback(string(id), data); err != nil {
			return err
		}
	}

	return nil
}

func (DBAccessor) GetBucket(tx *bolt.Tx, name string, nested ...string) (*bolt.Bucket, error) {
	bucket := tx.Bucket([]byte(name))
	if bucket == nil {
		return nil, errors.New("bucket does not exist: " + name)
	}

	for _, n := range nested {
		bucket = bucket.Bucket([]byte(n))
		if bucket == nil {
			return nil, errors.New("bucket does not exist: " + name)
		}
	}

	return bucket, nil
}
