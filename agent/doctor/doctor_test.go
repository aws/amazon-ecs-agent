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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type trueHealthcheck struct{}

func (tc *trueHealthcheck) RunCheck() HealthcheckStatus { return HealthcheckStatusOk }
func (tc *trueHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (tc *trueHealthcheck) GetLastHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (tc *trueHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *trueHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *trueHealthcheck) SetHealthcheckStatus(status HealthcheckStatus) {}

type falseHealthcheck struct{}

func (fc *falseHealthcheck) RunCheck() HealthcheckStatus { return HealthcheckStatusImpaired }
func (fc *falseHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (fc *falseHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *falseHealthcheck) GetLastHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (tc *falseHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *falseHealthcheck) SetHealthcheckStatus(status HealthcheckStatus) {}

func TestNewDoctor(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}
	healthchecks := []Healthcheck{trueCheck, falseCheck}
	newDoctor, _ := NewDoctor(healthchecks)
	assert.Len(t, newDoctor.healthchecks, 2)
}

type testHealthcheck struct {
	testName string
}

func (thc *testHealthcheck) RunCheck() HealthcheckStatus {
	return HealthcheckStatusOk
}

func (thc *testHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusOk
}

func (thc *testHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}

func (tc *testHealthcheck) GetLastHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (tc *testHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *testHealthcheck) SetHealthcheckStatus(status HealthcheckStatus) {}

func TestAddHealthcheck(t *testing.T) {
	newDoctor := Doctor{}
	assert.Len(t, newDoctor.healthchecks, 0)
	newTestHealthcheck := &testHealthcheck{testName: "testAddHealthcheck"}
	newDoctor.AddHealthcheck(newTestHealthcheck)
	assert.Len(t, newDoctor.healthchecks, 1)
}

func TestRunHealthchecks(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}

	testcases := []struct {
		name           string
		checks         []Healthcheck
		expectedResult bool
	}{
		{
			name:           "empty checks",
			checks:         []Healthcheck{},
			expectedResult: true,
		},
		{
			name:           "all true checks",
			checks:         []Healthcheck{trueCheck},
			expectedResult: true,
		},
		{
			name:           "all false checks",
			checks:         []Healthcheck{falseCheck},
			expectedResult: false,
		},
		{
			name:           "mixed checks",
			checks:         []Healthcheck{trueCheck, falseCheck},
			expectedResult: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctor, _ := NewDoctor(tc.checks)
			overallResult := newDoctor.RunHealthchecks()
			assert.Equal(t, overallResult, tc.expectedResult)
		})
	}
}

func TestAllRight(t *testing.T) {
	testcases := []struct {
		name             string
		testChecksResult []HealthcheckStatus
		expectedResult   bool
	}{
		{
			name:             "empty checks",
			testChecksResult: []HealthcheckStatus{},
			expectedResult:   true,
		},
		{
			name:             "all true checks",
			testChecksResult: []HealthcheckStatus{HealthcheckStatusOk, HealthcheckStatusOk},
			expectedResult:   true,
		},
		{
			name:             "all false checks",
			testChecksResult: []HealthcheckStatus{HealthcheckStatusImpaired, HealthcheckStatusImpaired},
			expectedResult:   false,
		},
		{
			name:             "mixed checks",
			testChecksResult: []HealthcheckStatus{HealthcheckStatusOk, HealthcheckStatusImpaired},
			expectedResult:   false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctor := Doctor{}
			overallResult := newDoctor.allRight(tc.testChecksResult)
			assert.Equal(t, overallResult, tc.expectedResult)
		})
	}
}
