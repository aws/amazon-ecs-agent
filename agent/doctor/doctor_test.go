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

	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"

	"github.com/aws/aws-sdk-go/aws"
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

func TestGetInstanceStatuses(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}
	trueStatus := &ecstcs.InstanceStatus{
		LastStatusChange: aws.Time(trueCheck.GetStatusChangeTime()),
		LastUpdated:      aws.Time(trueCheck.GetLastHealthcheckTime()),
		Status:           aws.String(trueCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(trueCheck.GetHealthcheckType()),
	}
	falseStatus := &ecstcs.InstanceStatus{
		LastStatusChange: aws.Time(falseCheck.GetStatusChangeTime()),
		LastUpdated:      aws.Time(falseCheck.GetLastHealthcheckTime()),
		Status:           aws.String(falseCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(falseCheck.GetHealthcheckType()),
	}

	testcases := []struct {
		name           string
		checks         []Healthcheck
		expectedResult []*ecstcs.InstanceStatus
	}{
		{
			name:           "empty checks",
			checks:         []Healthcheck{},
			expectedResult: nil,
		},
		{
			name:           "all true checks",
			checks:         []Healthcheck{trueCheck},
			expectedResult: []*ecstcs.InstanceStatus{trueStatus},
		},
		{
			name:           "all false checks",
			checks:         []Healthcheck{falseCheck},
			expectedResult: []*ecstcs.InstanceStatus{falseStatus},
		},
		{
			name:           "mixed checks",
			checks:         []Healthcheck{trueCheck, falseCheck},
			expectedResult: []*ecstcs.InstanceStatus{trueStatus, falseStatus},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctor, _ := NewDoctor(tc.checks, TEST_CLUSTER, TEST_INSTANCE_ARN)
			newDoctor.RunHealthchecks()
			instanceStatuses := newDoctor.getInstanceStatuses()
			assert.Equal(t, instanceStatuses, tc.expectedResult)
		})
	}
}

func TestGetPublishInstanceStatusRequest(t *testing.T) {
	trueCheck := &trueHealthcheck{}
	falseCheck := &falseHealthcheck{}
	trueStatus := &ecstcs.InstanceStatus{
		LastStatusChange: aws.Time(trueCheck.GetStatusChangeTime()),
		LastUpdated:      aws.Time(trueCheck.GetLastHealthcheckTime()),
		Status:           aws.String(trueCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(trueCheck.GetHealthcheckType()),
	}
	falseStatus := &ecstcs.InstanceStatus{
		LastStatusChange: aws.Time(falseCheck.GetStatusChangeTime()),
		LastUpdated:      aws.Time(falseCheck.GetLastHealthcheckTime()),
		Status:           aws.String(falseCheck.GetHealthcheckStatus().String()),
		Type:             aws.String(falseCheck.GetHealthcheckType()),
	}

	testcases := []struct {
		name             string
		checks           []Healthcheck
		expectedStatuses []*ecstcs.InstanceStatus
	}{
		{
			name:             "empty checks",
			checks:           []Healthcheck{},
			expectedStatuses: nil,
		},
		{
			name:             "all true checks",
			checks:           []Healthcheck{trueCheck},
			expectedStatuses: []*ecstcs.InstanceStatus{trueStatus},
		},
		{
			name:             "all false checks",
			checks:           []Healthcheck{falseCheck},
			expectedStatuses: []*ecstcs.InstanceStatus{falseStatus},
		},
		{
			name:             "mixed checks",
			checks:           []Healthcheck{trueCheck, falseCheck},
			expectedStatuses: []*ecstcs.InstanceStatus{trueStatus, falseStatus},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctor, _ := NewDoctor(tc.checks, TEST_CLUSTER, TEST_INSTANCE_ARN)
			newDoctor.RunHealthchecks()

			// note: setting RequestId and Timestamp to nil so I can make the comparison
			metadata := &ecstcs.InstanceStatusMetadata{
				Cluster:           aws.String(TEST_CLUSTER),
				ContainerInstance: aws.String(TEST_INSTANCE_ARN),
				RequestId:         nil,
			}

			testResult, err := newDoctor.GetPublishInstanceStatusRequest()

			if tc.expectedStatuses != nil {
				expectedResult := &ecstcs.PublishInstanceStatusRequest{
					Metadata:  metadata,
					Statuses:  tc.expectedStatuses,
					Timestamp: nil,
				}
				// note: setting RequestId and Timestamp to nil so I can make the comparison
				testResult.Timestamp = nil
				testResult.Metadata.RequestId = nil
				assert.Equal(t, testResult, expectedResult)
			} else {
				assert.Error(t, err, "Test failed")
			}
		})
	}
}
