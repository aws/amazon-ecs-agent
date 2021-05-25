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

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

func putObject(bucket *bolt.Bucket, key string, obj interface{}) error {
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

func getObject(tx *bolt.Tx, bucketName, id string, out interface{}) error {
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

func walk(bucket *bolt.Bucket, callback func(id string, data []byte) error) error {
	cursor := bucket.Cursor()

	for id, data := cursor.First(); id != nil; id, data = cursor.Next() {
		if err := callback(string(id), data); err != nil {
			return err
		}
	}

	return nil
}
