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

package fsxwindowsfileserver

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	var resourceStatus FSxWindowsFileServerVolumeStatus

	resourceStatus = FSxWindowsFileServerVolumeStatusNone
	assert.Equal(t, resourceStatus.String(), "NONE")
	resourceStatus = FSxWindowsFileServerVolumeCreated
	assert.Equal(t, resourceStatus.String(), "CREATED")
	resourceStatus = FSxWindowsFileServerVolumeRemoved
	assert.Equal(t, resourceStatus.String(), "REMOVED")
}

func TestMarshalFSxWindowsFileServerVolumeStatus(t *testing.T) {
	status := FSxWindowsFileServerVolumeStatusNone
	bytes, err := status.MarshalJSON()

	assert.NoError(t, err)
	assert.Equal(t, `"NONE"`, string(bytes[:]))
}

func TestMarshalNilFSxWindowsFileServerVolumeStatus(t *testing.T) {
	var status *FSxWindowsFileServerVolumeStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Nil(t, err)
}

type testFSxWindowsFileServerVolumeStatus struct {
	SomeStatus FSxWindowsFileServerVolumeStatus `json:"status"`
}

func TestUnmarshalFSxWindowsFileServerVolumeStatus(t *testing.T) {
	status := FSxWindowsFileServerVolumeStatusNone

	err := json.Unmarshal([]byte(`"CREATED"`), &status)
	assert.NoError(t, err)
	assert.Equal(t, FSxWindowsFileServerVolumeCreated, status, "CREATED should unmarshal to CREATED, not "+status.String())

	var testStatus testFSxWindowsFileServerVolumeStatus
	err = json.Unmarshal([]byte(`{"status":"REMOVED"}`), &testStatus)
	assert.NoError(t, err)
	assert.Equal(t, FSxWindowsFileServerVolumeRemoved, testStatus.SomeStatus, "REMOVED should unmarshal to REMOVED, not "+testStatus.SomeStatus.String())
}

func TestUnmarshalNullFSxWindowsFileServerVolumeStatus(t *testing.T) {
	status := FSxWindowsFileServerVolumeCreated
	err := json.Unmarshal([]byte("null"), &status)
	assert.NoError(t, err)
	assert.Equal(t, FSxWindowsFileServerVolumeStatusNone, status, "null should unmarshal to None, not "+status.String())
}

func TestUnmarshalNonStringFSxWindowsFileServerVolumeStatusDefaultNone(t *testing.T) {
	status := FSxWindowsFileServerVolumeCreated
	err := json.Unmarshal([]byte(`1`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, FSxWindowsFileServerVolumeStatusNone, status, "non-string status should unmarshal to None, not "+status.String())
}

func TestUnmarshalUnmappedFSxWindowsFileServerVolumeStatusDefaultNone(t *testing.T) {
	status := FSxWindowsFileServerVolumeRemoved
	err := json.Unmarshal([]byte(`"SOMEOTHER"`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, FSxWindowsFileServerVolumeStatusNone, status, "Unmapped status should unmarshal to None, not "+status.String())
}
