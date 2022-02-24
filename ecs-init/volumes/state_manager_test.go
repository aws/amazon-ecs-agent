// Copyright 2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package volumes

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSaveStateSuccess(t *testing.T) {
	s := NewStateManager()
	s.VolState.Volumes["vol"] = &VolumeInfo{
		Type: "efs",
		Path: "/vol",
	}
	saveStateToDisk = func(b []byte) error {
		assert.Equal(t, string(`{
	"volumes": {
		"vol": {
			"type": "efs",
			"path": "/vol"
		}
	}
}`), string(b))
		return nil
	}
	defer func() {
		saveStateToDisk = saveState
	}()
	assert.NoError(t, s.save())
}

func TestSaveStateToDisk(t *testing.T) {
	s := NewStateManager()
	s.VolState.Volumes["vol"] = &VolumeInfo{
		Type: "efs",
		Path: "/vol",
	}
	saveStateToDisk = func(b []byte) error {
		return nil
	}
	defer func() {
		saveStateToDisk = saveState
	}()
	assert.NoError(t, s.save())
}

func TestSaveStateToDiskFail(t *testing.T) {
	s := NewStateManager()
	s.VolState.Volumes["vol"] = &VolumeInfo{
		Type: "efs",
		Path: "/vol",
	}
	saveStateToDisk = func(b []byte) error {
		return errors.New("write to disk failure")
	}
	defer func() {
		saveStateToDisk = saveState
	}()
	assert.Error(t, s.save())
}

func TestLoadStateSuccess(t *testing.T) {
	s := NewStateManager()
	oldState := &VolumeState{}
	readStateFile = func() ([]byte, error) {
		return []byte(`{"volumes":{"efsVolume":{"type":"efs","options":{"device":"fs-123","o":"tls","type":"efs"}}}}`), nil
	}
	defer func() {
		readStateFile = readFile
	}()
	assert.NoError(t, s.load(oldState))
}

func TestLoadInvalidState(t *testing.T) {
	s := NewStateManager()
	oldState := &VolumeState{}
	readStateFile = func() ([]byte, error) {
		return []byte(`"junk"`), nil
	}
	defer func() {
		readStateFile = readFile
	}()
	assert.Error(t, s.load(oldState))
}
