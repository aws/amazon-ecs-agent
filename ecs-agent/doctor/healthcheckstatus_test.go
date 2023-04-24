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

	err := json.Unmarshal([]byte(`"INITIALIZING"`), &status)
	if err != nil {
		t.Error(err)
	}
	if status != HealthcheckStatusInitializing {
		t.Error("INITIALIZING should unmarshal to INITIALIZING, not " + status.String())
	}

	var test testHealthcheckStatus
	err = json.Unmarshal([]byte(`{"status":"IMPAIRED"}`), &test)
	if err != nil {
		t.Error(err)
	}
	if test.SomeStatus != HealthcheckStatusImpaired {
		t.Error("IMPAIRED should unmarshal to IMPAIRED, not " + test.SomeStatus.String())
	}
}
