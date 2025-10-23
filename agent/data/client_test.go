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
	"fmt"
	"path/filepath"
	"testing"

	generaldata "github.com/aws/amazon-ecs-agent/ecs-agent/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/modeltransformer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

const (
	statPathNotFoundErr = "stat %s: no such file or directory"
)

func newTestClient(t *testing.T) Client {
	testDir := t.TempDir()

	testDB, err := bolt.Open(filepath.Join(testDir, dbName), dbMode, nil)
	transformer := modeltransformer.NewTransformer()
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
		generaldata.Client{
			Accessor:    generaldata.DBAccessor{},
			DB:          testDB,
			Transformer: transformer,
		},
	}

	t.Cleanup(func() {
		require.NoError(t, testClient.Close())
	})
	return testClient
}

func TestSetupInvalidDirectoryErrors(t *testing.T) {
	originalDirExists := dirExists
	defer func() {
		dirExists = originalDirExists
	}()
	tcs := []struct {
		name          string
		dirPath       string
		mockDirExists func(string) error
		expectedError error
	}{
		{
			name:    "path not found",
			dirPath: "/path/",
			mockDirExists: func(path string) error {
				return fmt.Errorf(statPathNotFoundErr, path)
			},
			expectedError: fmt.Errorf(statPathNotFoundErr, "/path/"),
		},
		{
			name:    "path is not a directory",
			dirPath: "/data/blah.txt",
			mockDirExists: func(path string) error {
				return fmt.Errorf(notDirectoryErrorMsg, path)
			},
			expectedError: fmt.Errorf(notDirectoryErrorMsg, "/data/blah.txt"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			dirExists = tc.mockDirExists

			_, err := setup(tc.dirPath)

			assert.Equal(t, tc.expectedError, err)
		})
	}
}
