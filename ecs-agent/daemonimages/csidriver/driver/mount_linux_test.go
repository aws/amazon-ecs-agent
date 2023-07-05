//go:build linux && unit
// +build linux,unit

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

package driver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPathExists(t *testing.T) {
	// set up the full driver and its environment
	dir, err := os.MkdirTemp("", "mount-ebs-csi")
	if err != nil {
		t.Fatalf("error creating directory %v", err)
	}
	defer os.RemoveAll(dir)

	targetPath := filepath.Join(dir, "notafile")

	mountObj, err := newNodeMounter()
	require.NoError(t, err, "error when creating mounter")

	exists, err := mountObj.PathExists(targetPath)
	require.NoError(t, err, "error when checking if the path exists")

	require.False(t, exists, "expect file not exist")
}
