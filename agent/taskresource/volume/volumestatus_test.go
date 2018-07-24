// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package volume

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	var resourceStatus VolumeStatus

	resourceStatus = VolumeStatusNone
	assert.Equal(t, resourceStatus.String(), "NONE")
	resourceStatus = VolumeCreated
	assert.Equal(t, resourceStatus.String(), "CREATED")
	resourceStatus = VolumeRemoved
	assert.Equal(t, resourceStatus.String(), "REMOVED")
}

func TestMarshalVolumeStatus(t *testing.T) {
	status := VolumeStatusNone
	bytes, err := status.MarshalJSON()

	assert.NoError(t, err)
	assert.Equal(t, `"NONE"`, string(bytes[:]))
}

func TestMarshalNilVolumeStatus(t *testing.T) {
	var status *VolumeStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Nil(t, err)
}

type testVolumeStatus struct {
	SomeStatus VolumeStatus `json:"status"`
}

func TestUnmarshalVolumeStatus(t *testing.T) {
	status := VolumeStatusNone

	err := json.Unmarshal([]byte(`"CREATED"`), &status)
	assert.NoError(t, err)
	assert.Equal(t, VolumeCreated, status, "CREATED should unmarshal to CREATED, not "+status.String())

	var testStatus testVolumeStatus
	err = json.Unmarshal([]byte(`{"status":"REMOVED"}`), &testStatus)
	assert.NoError(t, err)
	assert.Equal(t, VolumeRemoved, testStatus.SomeStatus, "REMOVED should unmarshal to REMOVED, not "+testStatus.SomeStatus.String())
}

func TestUnmarshalNullVolumeStatus(t *testing.T) {
	status := VolumeCreated
	err := json.Unmarshal([]byte("null"), &status)
	assert.NoError(t, err)
	assert.Equal(t, VolumeStatusNone, status, "null should unmarshal to None, not "+status.String())
}

func TestUnmarshalNonStringVolumeStatusDefaultNone(t *testing.T) {
	status := VolumeCreated
	err := json.Unmarshal([]byte(`1`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, VolumeStatusNone, status, "non-string status should unmarshal to None, not "+status.String())
}

func TestUnmarshalUnmappedVolumeStatusDefaultNone(t *testing.T) {
	status := VolumeRemoved
	err := json.Unmarshal([]byte(`"SOMEOTHER"`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, VolumeStatusNone, status, "Unmapped status should unmarshal to None, not "+status.String())
}
