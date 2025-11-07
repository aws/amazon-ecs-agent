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

package doctor

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/stretchr/testify/assert"
)

func TestOk(t *testing.T) {
	initializingStatus := ecstcs.InstanceHealthcheckStatusInitializing
	okStatus := ecstcs.InstanceHealthcheckStatusOk
	impairedStatus := ecstcs.InstanceHealthcheckStatusImpaired
	assert.True(t, initializingStatus.Ok())
	assert.True(t, okStatus.Ok())
	assert.False(t, impairedStatus.Ok())
}

type testHealthcheckStatus struct {
	SomeStatus ecstcs.InstanceHealthcheckStatus `json:"status"`
}

func TestUnmarshalHealthcheckStatus(t *testing.T) {
	status := ecstcs.InstanceHealthcheckStatusInitializing
	initializingStr := "INITIALIZING"

	err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, initializingStr)), &status)
	assert.NoError(t, err)
	// INITIALIZING should unmarshal to INITIALIZING.
	assert.Equal(t, ecstcs.InstanceHealthcheckStatusInitializing, status)
	assert.Equal(t, initializingStr, status.String())

	var test testHealthcheckStatus
	impairedStr := "IMPAIRED"
	err = json.Unmarshal([]byte(fmt.Sprintf(`{"status":"%s"}`, impairedStr)), &test)
	assert.NoError(t, err)
	// IMPAIRED should unmarshal to IMPAIRED.
	assert.Equal(t, ecstcs.InstanceHealthcheckStatusImpaired, test.SomeStatus)
	assert.Equal(t, impairedStr, test.SomeStatus.String())
}
