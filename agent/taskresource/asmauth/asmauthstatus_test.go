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

package asmauth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	var resourceStatus ASMAuthStatus

	resourceStatus = ASMAuthStatusNone
	assert.Equal(t, resourceStatus.String(), "NONE")
	resourceStatus = ASMAuthStatusCreated
	assert.Equal(t, resourceStatus.String(), "CREATED")
	resourceStatus = ASMAuthStatusRemoved
	assert.Equal(t, resourceStatus.String(), "REMOVED")
}

func TestMarshalASMAuthStatus(t *testing.T) {
	status := ASMAuthStatusNone
	bytes, err := status.MarshalJSON()

	assert.NoError(t, err)
	assert.Equal(t, `"NONE"`, string(bytes[:]))
}

func TestMarshalNilASMAuthStatus(t *testing.T) {
	var status *ASMAuthStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Nil(t, err)
}

type testASMAuthStatus struct {
	SomeStatus ASMAuthStatus `json:"status"`
}

func TestUnmarshalASMAuthStatus(t *testing.T) {
	status := ASMAuthStatusNone

	err := json.Unmarshal([]byte(`"CREATED"`), &status)
	assert.NoError(t, err)
	assert.Equal(t, ASMAuthStatusCreated, status, "CREATED should unmarshal to CREATED, not "+status.String())

	var testStatus testASMAuthStatus
	err = json.Unmarshal([]byte(`{"status":"REMOVED"}`), &testStatus)
	assert.NoError(t, err)
	assert.Equal(t, ASMAuthStatusRemoved, testStatus.SomeStatus, "REMOVED should unmarshal to REMOVED, not "+testStatus.SomeStatus.String())
}

func TestUnmarshalNullASMAuthStatus(t *testing.T) {
	status := ASMAuthStatusCreated
	err := json.Unmarshal([]byte("null"), &status)
	assert.NoError(t, err)
	assert.Equal(t, ASMAuthStatusNone, status, "null should unmarshal to None, not "+status.String())

}

func TestUnmarshalNonStringASMAuthStatusDefaultNone(t *testing.T) {
	status := ASMAuthStatusCreated
	err := json.Unmarshal([]byte(`1`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, ASMAuthStatusNone, status, "non-string status should unmarshal to None, not "+status.String())

}

func TestUnmarshalUnmappedASMAuthStatusDefaultNone(t *testing.T) {
	status := ASMAuthStatusCreated
	err := json.Unmarshal([]byte(`"SOMEOTHER"`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, ASMAuthStatusNone, status, "Unmapped status should unmarshal to None, not "+status.String())
}
