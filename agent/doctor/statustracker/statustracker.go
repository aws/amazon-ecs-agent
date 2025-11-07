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
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
)

// HealthCheckStatusTracker is a helper for keeping track of current and last health check status.
type HealthCheckStatusTracker struct {
	status           ecstcs.InstanceHealthcheckStatus
	timeStamp        time.Time
	statusChangeTime time.Time
	lastStatus       ecstcs.InstanceHealthcheckStatus
	lastTimeStamp    time.Time
	now              func() time.Time // Function that returns current time (injected for testing).
	lock             sync.RWMutex
}

// GetHealthcheckStatus returns the current health check status.
func (e *HealthCheckStatusTracker) GetHealthcheckStatus() ecstcs.InstanceHealthcheckStatus {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.status
}

// GetHealthcheckTime returns the timestamp of the current health check status.
func (e *HealthCheckStatusTracker) GetHealthcheckTime() time.Time {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.timeStamp
}

// GetStatusChangeTime returns the timestamp when the status last changed.
func (e *HealthCheckStatusTracker) GetStatusChangeTime() time.Time {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.statusChangeTime
}

// GetLastHealthcheckStatus returns the previous health check status.
func (e *HealthCheckStatusTracker) GetLastHealthcheckStatus() ecstcs.InstanceHealthcheckStatus {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.lastStatus
}

// GetLastHealthcheckTime returns the timestamp of the previous health check status.
func (e *HealthCheckStatusTracker) GetLastHealthcheckTime() time.Time {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.lastTimeStamp
}

// SetHealthcheckStatus updates the health check status and timestamps.
func (e *HealthCheckStatusTracker) SetHealthcheckStatus(healthStatus ecstcs.InstanceHealthcheckStatus) {
	e.lock.Lock()
	defer e.lock.Unlock()
	nowTime := e.now()

	// If the status has changed, update status change timestamp.
	if e.status != healthStatus {
		e.statusChangeTime = nowTime
	}

	// Track previous status.
	e.lastStatus = e.status
	e.lastTimeStamp = e.timeStamp

	// Update latest status.
	e.status = healthStatus
	e.timeStamp = nowTime
}

// NewHealthCheckStatusTracker returns a new HealthCheckStatusTracker.
func NewHealthCheckStatusTracker() *HealthCheckStatusTracker {
	return newHealthCheckStatusTrackerWithTimeFn(time.Now)
}

func newHealthCheckStatusTrackerWithTimeFn(timeNow func() time.Time) *HealthCheckStatusTracker {
	now := timeNow()
	return &HealthCheckStatusTracker{
		status:           ecstcs.InstanceHealthcheckStatusInitializing,
		timeStamp:        now,
		statusChangeTime: now,
		now:              timeNow,
	}
}
