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

package taskresource

import (
	"encoding/json"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/stretchr/testify/assert"
)

func TestCgroupStatusString(t *testing.T) {
	var resourceStatus CgroupStatus

	resourceStatus = CgroupStatusNone
	assert.Equal(t, resourceStatus.String(), "NONE")
	resourceStatus = CgroupCreated
	assert.Equal(t, resourceStatus.String(), "CREATED")
	resourceStatus = CgroupCleaned
	assert.Equal(t, resourceStatus.String(), "CLEANED")
}

func TestCgroupTaskStatus(t *testing.T) {
	var resourceStatus CgroupStatus

	resourceStatus = CgroupStatusNone
	assert.Equal(t, resourceStatus.TaskStatus(), api.TaskStatusNone)
	resourceStatus = CgroupCreated
	assert.Equal(t, resourceStatus.TaskStatus(), api.TaskCreated)
	resourceStatus = CgroupCleaned
	assert.Equal(t, resourceStatus.TaskStatus(), api.TaskZombie)
}

func TestMarshalCgroupStatus(t *testing.T) {
	status := CgroupStatusNone
	bytes, err := status.MarshalJSON()

	assert.NoError(t, err)
	assert.Equal(t, `"NONE"`, string(bytes[:]))
}

func TestMarshalNilCgroupStatus(t *testing.T) {
	var status *CgroupStatus
	bytes, err := status.MarshalJSON()

	assert.Nil(t, bytes)
	assert.Nil(t, err)
}

type testCgroupStatus struct {
	SomeStatus CgroupStatus `json:"status"`
}

func TestUnmarshalCgroupStatus(t *testing.T) {
	status := CgroupStatusNone

	err := json.Unmarshal([]byte(`"CREATED"`), &status)
	assert.NoError(t, err)
	assert.Equal(t, CgroupCreated, status, "CREATED should unmarshal to CREATED, not "+status.String())

	var testStatus testCgroupStatus
	err = json.Unmarshal([]byte(`{"status":"CLEANED"}`), &testStatus)
	assert.NoError(t, err)
	assert.Equal(t, CgroupCleaned, testStatus.SomeStatus, "CLEANED should unmarshal to CLEANED, not "+testStatus.SomeStatus.String())
}

func TestUnmarshalNullCgroupStatus(t *testing.T) {
	status := CgroupCreated
	err := json.Unmarshal([]byte("null"), &status)
	assert.NoError(t, err)
	assert.Equal(t, CgroupStatusNone, status, "null should unmarshal to None, not "+status.String())
}

func TestUnmarshalNonStringCgroupStatusDefaultNone(t *testing.T) {
	status := CgroupCreated
	err := json.Unmarshal([]byte(`1`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, CgroupStatusNone, status, "non-string status should unmarshal to None, not "+status.String())
}

func TestUnmarshalUnmappedCgroupStatusDefaultNone(t *testing.T) {
	status := CgroupCleaned
	err := json.Unmarshal([]byte(`"SOMEOTHER"`), &status)
	assert.NotNil(t, err)
	assert.Equal(t, CgroupStatusNone, status, "Unmapped status should unmarshal to None, not "+status.String())
}
