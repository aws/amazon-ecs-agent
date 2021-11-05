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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	TEST_CLUSTER      = "test-cluster"
	TEST_INSTANCE_ARN = "test-instance-arn"
)

type trueHealthcheck struct{}

func (tc *trueHealthcheck) RunCheck() HealthcheckStatus                   { return HealthcheckStatusOk }
func (tc *trueHealthcheck) SetHealthcheckStatus(status HealthcheckStatus) {}
func (tc *trueHealthcheck) GetHealthcheckType() string                    { return HealthcheckTypeAgent }
func (tc *trueHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (tc *trueHealthcheck) GetLastHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (tc *trueHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *trueHealthcheck) GetStatusChangeTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (tc *trueHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}

type falseHealthcheck struct{}

func (fc *falseHealthcheck) RunCheck() HealthcheckStatus                   { return HealthcheckStatusImpaired }
func (fc *falseHealthcheck) SetHealthcheckStatus(status HealthcheckStatus) {}
func (fc *falseHealthcheck) GetHealthcheckType() string                    { return HealthcheckTypeAgent }
func (fc *falseHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (fc *falseHealthcheck) GetLastHealthcheckStatus() HealthcheckStatus {
	return HealthcheckStatusInitializing
}
func (fc *falseHealthcheck) GetHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (fc *falseHealthcheck) GetStatusChangeTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}
func (fc *falseHealthcheck) GetLastHealthcheckTime() time.Time {
	return time.Date(1974, time.May, 19, 1, 2, 3, 4, time.UTC)
}

func TestNewDoctor(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}
	healthchecks := []Healthcheck{trueCheck, falseCheck}
	newDoctor, _ := NewDoctor(healthchecks, TEST_CLUSTER, TEST_INSTANCE_ARN)
	assert.Len(t, newDoctor.healthchecks, 2)
}

func TestAddHealthcheck(t *testing.T) {
	newDoctor := Doctor{}
	assert.Len(t, newDoctor.healthchecks, 0)
	newTestHealthcheck := &trueHealthcheck{}
	newDoctor.AddHealthcheck(newTestHealthcheck)
	assert.Len(t, newDoctor.healthchecks, 1)
}

func TestGetCluster(t *testing.T) {
	clusterName := "test-cluster"
	newDoctor := Doctor{cluster: clusterName}
	assert.Equal(t, newDoctor.GetCluster(), clusterName)
}

func TestGetContainerInstanceArn(t *testing.T) {
	containerInstanceArn := "this:is:a:test:container:instance:arn"
	newDoctor := Doctor{containerInstanceArn: containerInstanceArn}
	assert.Equal(t, newDoctor.GetContainerInstanceArn(), containerInstanceArn)
}

func TestSetStatusReported(t *testing.T) {
	newDoctor := Doctor{}
	assert.Equal(t, newDoctor.HasStatusBeenReported(), false)
	newDoctor.SetStatusReported(true)
	assert.Equal(t, newDoctor.HasStatusBeenReported(), true)
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
			newDoctor, _ := NewDoctor(tc.checks, TEST_CLUSTER, TEST_INSTANCE_ARN)
			overallResult := newDoctor.RunHealthchecks()
			assert.Equal(t, overallResult, tc.expectedResult)
		})
	}
}

func TestGetHealthchecks(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}

	newDoctor := Doctor{healthchecks: []Healthcheck{trueCheck, trueCheck}}
	gottenChecks := newDoctor.GetHealthchecks()
	assert.Len(t, *gottenChecks, 2)

	regottenChecks := newDoctor.GetHealthchecks()
	(*regottenChecks)[1] = falseCheck
	assert.NotEqual(t, (*gottenChecks)[1], (*regottenChecks)[1])
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
