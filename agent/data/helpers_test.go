//go:build unit
// +build unit

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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

const (
	testBucketName = "test"
)

type testObjType struct {
	Key string
	Val string
}

func setupHelpersTest(t *testing.T) (*bolt.DB, func()) {
	testDir, err := ioutil.TempDir("", "agent_data_unit_test")
	require.NoError(t, err)
	db, err := bolt.Open(filepath.Join(testDir, dbName), dbMode, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucket([]byte(testBucketName))
		return err
	}))

	return db, func() {
		require.NoError(t, db.Close())
		require.NoError(t, os.RemoveAll(testDir))
	}
}

func TestHelpers(t *testing.T) {
	db, cleanup := setupHelpersTest(t)
	defer cleanup()

	testObj := &testObjType{
		Key: "key",
		Val: "test",
	}

	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(testBucketName))
		return putObject(b, testObj.Key, testObj)
	}))

	assert.Error(t, db.Update(func(tx *bolt.Tx) error {
		return getObject(tx, testBucketName, "xx", &testObjType{})
	}))

	res := &testObjType{}
	assert.NoError(t, db.Update(func(tx *bolt.Tx) error {
		return getObject(tx, testBucketName, testObj.Key, res)
	}))
	assert.Equal(t, testObj.Val, res.Val)

	var resArr []*testObjType
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(testBucketName))
		return walk(b, func(id string, data []byte) error {
			obj := &testObjType{}
			if err := json.Unmarshal(data, &obj); err != nil {
				return err
			}
			resArr = append(resArr, obj)
			return nil
		})
	}))
	assert.Len(t, resArr, 1)
	assert.Equal(t, testObj.Val, resArr[0].Val)
}
