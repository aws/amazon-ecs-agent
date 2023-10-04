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

	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
)

// Helper for keeping track of current and last health check status.
type HealthCheckStatusTracker struct {
	status           doctor.HealthcheckStatus
	timeStamp        time.Time
	statusChangeTime time.Time
	lastStatus       doctor.HealthcheckStatus
	lastTimeStamp    time.Time
	now              func() time.Time // function that returns current time (injected for testing)
	lock             sync.RWMutex
}

func (e *HealthCheckStatusTracker) GetHealthcheckStatus() doctor.HealthcheckStatus {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.status
}

func (e *HealthCheckStatusTracker) GetHealthcheckTime() time.Time {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.timeStamp
}

func (e *HealthCheckStatusTracker) GetStatusChangeTime() time.Time {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.statusChangeTime
}

func (e *HealthCheckStatusTracker) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.lastStatus
}

func (e *HealthCheckStatusTracker) GetLastHealthcheckTime() time.Time {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.lastTimeStamp
}

func (e *HealthCheckStatusTracker) SetHealthcheckStatus(healthStatus doctor.HealthcheckStatus) {
	e.lock.Lock()
	defer e.lock.Unlock()
	nowTime := e.now()

	// if the status has changed, update status change timestamp
	if e.status != healthStatus {
		e.statusChangeTime = nowTime
	}

	// track previous status
	e.lastStatus = e.status
	e.lastTimeStamp = e.timeStamp

	// update latest status
	e.status = healthStatus
	e.timeStamp = nowTime
}

// Returns a new HealthCheckStatusTracker
func NewHealthCheckStatusTracker() *HealthCheckStatusTracker {
	return newHealthCheckStatusTrackerWithTimeFn(time.Now)
}

func newHealthCheckStatusTrackerWithTimeFn(timeNow func() time.Time) *HealthCheckStatusTracker {
	now := timeNow()
	return &HealthCheckStatusTracker{
		status:           doctor.HealthcheckStatusInitializing,
		timeStamp:        now,
		statusChangeTime: now,
		now:              timeNow,
	}
}
