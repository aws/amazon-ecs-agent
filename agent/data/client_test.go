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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func newTestClient(t *testing.T) (Client, func()) {
	testDir, err := ioutil.TempDir("", "agent_data_unit_test")
	require.NoError(t, err)

	testDB, err := bolt.Open(filepath.Join(testDir, dbName), dbMode, nil)
	require.NoError(t, err)
	require.NoError(t, testDB.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			_, err = tx.CreateBucketIfNotExists([]byte(b))
			if err != nil {
				return err
			}
		}

		return nil
	}))
	testClient := &client{
		db: testDB,
	}

	cleanup := func() {
		require.NoError(t, testClient.Close())
		require.NoError(t, os.RemoveAll(testDir))
	}
	return testClient, cleanup
}
