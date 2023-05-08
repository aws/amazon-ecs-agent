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

	"github.com/stretchr/testify/assert"
)

func TestOk(t *testing.T) {
	initializingStatus := HealthcheckStatusInitializing
	okStatus := HealthcheckStatusOk
	impairedStatus := HealthcheckStatusImpaired
	assert.True(t, initializingStatus.Ok())
	assert.True(t, okStatus.Ok())
	assert.False(t, impairedStatus.Ok())
}

type testHealthcheckStatus struct {
	SomeStatus HealthcheckStatus `json:"status"`
}

func TestUnmarshalHealthcheckStatus(t *testing.T) {
	status := HealthcheckStatusInitializing
	initializingStr := "INITIALIZING"

	err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, initializingStr)), &status)
	assert.NoError(t, err)
	// INITIALIZING should unmarshal to INITIALIZING
	assert.Equal(t, HealthcheckStatusInitializing, status)
	assert.Equal(t, initializingStr, status.String())

	var test testHealthcheckStatus
	impairedStr := "IMPAIRED"
	err = json.Unmarshal([]byte(fmt.Sprintf(`{"status":"%s"}`, impairedStr)), &test)
	assert.NoError(t, err)
	// IMPAIRED should unmarshal to IMPAIRED
	assert.Equal(t, HealthcheckStatusImpaired, test.SomeStatus)
	assert.Equal(t, impairedStr, test.SomeStatus.String())
}
