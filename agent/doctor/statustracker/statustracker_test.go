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
package statustracker

import (
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheckStatusTracker(t *testing.T) {
	t.Run("initialization", func(t *testing.T) {
		now := time.Unix(1000, 0)
		tracker := newHealthCheckStatusTrackerWithTimeFn(func() time.Time { return now })
		assert.Equal(t, doctor.HealthcheckStatusInitializing, tracker.GetHealthcheckStatus())
		assert.Equal(t, now, tracker.GetHealthcheckTime())
		assert.Equal(t, now, tracker.GetStatusChangeTime())
	})
	t.Run("last status and timestamp is captured", func(t *testing.T) {
		tracker := newHealthCheckStatusTrackerWithTimeFn(incrementalTime())
		tracker.SetHealthcheckStatus(doctor.HealthcheckStatusOk)

		assert.Equal(t, doctor.HealthcheckStatusOk, tracker.GetHealthcheckStatus())
		assert.Equal(t, doctor.HealthcheckStatusInitializing, tracker.GetLastHealthcheckStatus())
		assert.Equal(t, int64(1), tracker.GetLastHealthcheckTime().Unix())
		assert.Equal(t, int64(2), tracker.GetHealthcheckTime().Unix())
		assert.Equal(t, int64(2), tracker.GetStatusChangeTime().Unix()) // changed to OK at time 2
	})
	t.Run("status change time is not changed if status hasn't changed", func(t *testing.T) {
		tracker := newHealthCheckStatusTrackerWithTimeFn(incrementalTime())
		// update (but not change) status a bunch of times
		for i := 0; i < 10; i++ {
			tracker.SetHealthcheckStatus(doctor.HealthcheckStatusOk)
		}

		assert.Equal(t, doctor.HealthcheckStatusOk, tracker.GetHealthcheckStatus())
		assert.Equal(t, doctor.HealthcheckStatusOk, tracker.GetLastHealthcheckStatus())

		// status change time remains at 2
		assert.Equal(t, int64(2), tracker.GetStatusChangeTime().Unix())
	})
	t.Run("multiple updates", func(t *testing.T) {
		tracker := newHealthCheckStatusTrackerWithTimeFn(incrementalTime())
		tracker.SetHealthcheckStatus(doctor.HealthcheckStatusOk)
		tracker.SetHealthcheckStatus(doctor.HealthcheckStatusImpaired)

		assert.Equal(t, doctor.HealthcheckStatusImpaired, tracker.GetHealthcheckStatus())
		assert.Equal(t, doctor.HealthcheckStatusOk, tracker.GetLastHealthcheckStatus())
		assert.Equal(t, int64(2), tracker.GetLastHealthcheckTime().Unix())
		assert.Equal(t, int64(3), tracker.GetHealthcheckTime().Unix())
		assert.Equal(t, int64(3), tracker.GetStatusChangeTime().Unix())
	})
}

// Returns a replacement function for time.Now() for testing.
// The returned function returns Unix epoch on first call and then returns times in one second
// increments.
func incrementalTime() func() time.Time {
	currentTime := 0
	return func() time.Time {
		currentTime++
		return time.Unix(int64(currentTime), 0)
	}
}
