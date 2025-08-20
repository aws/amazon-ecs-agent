//go:build linux && unit
// +build linux,unit

// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBlockDevice(t *testing.T) {
	volumePath := "./test"
	err := os.MkdirAll(volumePath, 0644)
	require.NoError(t, err, "fail to create dir")
	defer os.RemoveAll(volumePath)

	isBlockDevice, err := IsBlockDevice(volumePath)

	assert.Nil(t, err)
	assert.Equal(t, false, isBlockDevice)
}

func TestParseEndpoint(t *testing.T) {
	testCases := []struct {
		name      string
		endpoint  string
		expScheme string
		expAddr   string
		expErr    error
	}{
		{
			name:      "valid unix endpoint 1",
			endpoint:  "unix:///csi/csi.sock",
			expScheme: "unix",
			expAddr:   "/csi/csi.sock",
		},
		{
			name:      "valid unix endpoint 2",
			endpoint:  "unix://csi/csi.sock",
			expScheme: "unix",
			expAddr:   "/csi/csi.sock",
		},
		{
			name:      "valid unix endpoint 3",
			endpoint:  "unix:/csi/csi.sock",
			expScheme: "unix",
			expAddr:   "/csi/csi.sock",
		},
		{
			name:      "valid tcp endpoint",
			endpoint:  "tcp:///127.0.0.1/",
			expScheme: "tcp",
			expAddr:   "/127.0.0.1",
		},
		{
			name:      "valid tcp endpoint",
			endpoint:  "tcp:///127.0.0.1",
			expScheme: "tcp",
			expAddr:   "/127.0.0.1",
		},
		{
			name:     "invalid endpoint",
			endpoint: "http://127.0.0.1",
			expErr:   fmt.Errorf("unsupported protocol: http"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme, addr, err := ParseEndpoint(tc.endpoint)

			if tc.expErr != nil {
				assert.EqualError(t, err, tc.expErr.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, scheme, tc.expScheme, "scheme mismatches")
				assert.Equal(t, addr, tc.expAddr, "address mismatches")
			}
		})
	}

}

func TestGenerateGIDFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name: "empty path",
			path: "",
		},
		{
			name: "simple path",
			path: "/dev/xvda",
		},
		{
			name: "same path should generate same GID",
			path: "/dev/xvda",
		},
	}

	// Store GIDs to check for consistency
	gidMap := make(map[string]int)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gid := GenerateGIDFromPath(tc.path)

			// Check that GID is in the valid range
			assert.GreaterOrEqual(t, gid, minGID)
			assert.LessOrEqual(t, gid, maxGID)

			// If we've seen this path before, check that the GID is the same
			if prevGID, ok := gidMap[tc.path]; ok && tc.path != "" {
				assert.Equal(t, prevGID, gid, "Same path should generate same GID")
			}

			// Store the GID for this path
			gidMap[tc.path] = gid
		})
	}

	// Test that different paths generate different GIDs (with high probability)
	path1 := "/dev/xvda"
	path2 := "/dev/xvdb"
	gid1 := GenerateGIDFromPath(path1)
	gid2 := GenerateGIDFromPath(path2)
	assert.NotEqual(t, gid1, gid2, "Different paths should generate different GIDs")
}
