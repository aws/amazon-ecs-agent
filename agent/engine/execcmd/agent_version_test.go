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

package execcmd

import (
	"errors"
	"io/ioutil"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAgentVersionCompare(t *testing.T) {
	tt := []struct {
		name     string
		lhs, rhs agentVersion
		res      int
	}{
		{
			"lhs < rhs", []uint{3, 0, 236, 0}, []uint{3, 0, 237, 0}, -1,
		},
		{
			"lhs == rhs, where both are empty", []uint{}, []uint{}, 0,
		},
		{
			"lhs == rhs", []uint{3, 0, 236, 0}, []uint{3, 0, 236, 0}, 0,
		},
		{
			"lhs > rhs", []uint{3, 0, 237, 0}, []uint{3, 0, 236, 0}, 1,
		},
		{
			"fewer elements lhs", []uint{3, 0, 236}, []uint{1, 1, 1, 1}, 1,
		},
		{
			"fewer elements rhs", []uint{1, 1, 1, 1}, []uint{3, 0, 236}, -1,
		},
		{
			"blank lhs", []uint{}, []uint{1, 1, 1, 1}, -1,
		},
		{
			"blank rhs", []uint{1, 1, 1, 1}, []uint{}, 1,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.lhs.compare(tc.rhs)
			assert.Equal(t, tc.res, res)
		})
	}
}

func TestAgentVersionString(t *testing.T) {
	tt := []struct {
		name           string
		av             agentVersion
		expectedString string
	}{
		{
			"nil returns empty string",
			nil,
			""},
		{
			"single element",
			agentVersion{1},
			"1",
		},
		{
			"multi element",
			agentVersion{1, 2},
			"1.2",
		},
		{
			"real world case",
			agentVersion{3, 0, 236, 0},
			"3.0.236.0",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedString, tc.av.String())
		})
	}
}

func TestByAgentVersion(t *testing.T) {
	bav := byAgentVersion(
		[]agentVersion{
			{3, 0, 236, 0},
			{1, 1, 1, 1},
			{1, 1, 1, 1},
			{2, 0, 0, 0},
		})

	assert.Equal(t, bav.Len(), 4)
	assert.True(t, bav.Less(3, 0))  // {2, 0, 0, 0} < {3, 0, 236, 0}
	assert.False(t, bav.Less(0, 3)) // {3, 0, 236, 0} > {2, 0, 0, 0}
	assert.False(t, bav.Less(1, 2)) // {1, 1, 1, 1} == {1, 1, 1, 1}

	swappedBav := byAgentVersion(
		[]agentVersion{
			{2, 0, 0, 0},
			{1, 1, 1, 1},
			{1, 1, 1, 1},
			{3, 0, 236, 0},
		})
	bav.Swap(0, 3)
	assert.Equal(t, swappedBav, bav)

	sortedBav := byAgentVersion(
		[]agentVersion{
			{1, 1, 1, 1},
			{1, 1, 1, 1},
			{2, 0, 0, 0},
			{3, 0, 236, 0},
		})
	sort.Sort(bav)
	assert.Equal(t, sortedBav, bav)
}

type mockFileInfo struct {
	name  string
	isDir bool
}

func (fi *mockFileInfo) Name() string       { return fi.name }
func (fi *mockFileInfo) IsDir() bool        { return fi.isDir }
func (fi *mockFileInfo) Size() int64        { return 0 }
func (fi *mockFileInfo) Mode() os.FileMode  { return 0 }
func (fi *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *mockFileInfo) Sys() interface{}   { return nil }

func TestRetrieveAgentVersions(t *testing.T) {
	defer func() {
		ioUtilReadDir = ioutil.ReadDir
	}()
	tt := []struct {
		name             string
		files            []os.FileInfo
		expectedVersions []agentVersion
		expectedError    error
	}{
		{
			name:          "ioutil.ReadDir error",
			expectedError: errors.New("mock error"),
		},
		{
			name:  "no files",
			files: nil,
		},
		{
			name: "non-version directories/files",
			files: []os.FileInfo{
				&mockFileInfo{
					name:  "0.0.0.0",
					isDir: true,
				},
				&mockFileInfo{
					name:  "randomDir",
					isDir: true,
				},
				&mockFileInfo{
					name:  "randomFile",
					isDir: false,
				},
				&mockFileInfo{
					name:  "1.0.0.0",
					isDir: true,
				},
			},
			expectedVersions: []agentVersion{
				{0, 0, 0, 0},
				{1, 0, 0, 0},
			},
		},
		{
			name: "happy path",
			files: []os.FileInfo{
				&mockFileInfo{
					name:  "3.0.236.0",
					isDir: true,
				},
				&mockFileInfo{
					name:  "0.0.0.0",
					isDir: true,
				},
				&mockFileInfo{
					name:  "1.0.0.0",
					isDir: true,
				},
			},
			expectedVersions: []agentVersion{
				{3, 0, 236, 0},
				{0, 0, 0, 0},
				{1, 0, 0, 0},
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ioUtilReadDir = func(dirname string) ([]os.FileInfo, error) {
				return tc.files, tc.expectedError
			}
			versions, err := retrieveAgentVersions("/var/lib/ecs/deps/execute-command/bin")
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expectedVersions, versions)
		})
	}
}
