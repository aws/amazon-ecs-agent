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

package restart

import (
	"fmt"
	"sync"
	"time"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
)

type RestartTracker struct {
	RestartCount  int           `json:"restartCount,omitempty"`
	LastRestartAt time.Time     `json:"lastRestartAt,omitempty"`
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty"`
	lock          sync.RWMutex
}

// RestartPolicy represents a policy that contains key information considered when
// deciding whether or not a container should be restarted after it has exited.
type RestartPolicy struct {
	Enabled              bool  `json:"enabled"`
	IgnoredExitCodes     []int `json:"ignoredExitCodes"`
	RestartAttemptPeriod int   `json:"restartAttemptPeriod"`
}

func NewRestartTracker(restartPolicy RestartPolicy) *RestartTracker {
	return &RestartTracker{
		RestartPolicy: restartPolicy,
	}
}

func (rt *RestartTracker) GetLastRestartAt() time.Time {
	rt.lock.RLock()
	defer rt.lock.RUnlock()
	return rt.LastRestartAt
}

func (rt *RestartTracker) GetRestartCount() int {
	rt.lock.RLock()
	defer rt.lock.RUnlock()
	return rt.RestartCount
}

// RecordRestart updates the restart tracker's metadata after a restart has occurred.
// This metadata is used to calculate when restarts should occur and track how many
// have occurred. It is not the job of this method to determine if a restart should
// occur or restart the container.
func (rt *RestartTracker) RecordRestart() {
	rt.lock.Lock()
	defer rt.lock.Unlock()
	rt.RestartCount++
	rt.LastRestartAt = time.Now()
}

// ShouldRestart returns whether the container should restart and a reason string
// explaining why not. The reset attempt period will be calculated first
// with LastRestart at, using the passed in startedAt if it does not exist.
func (rt *RestartTracker) ShouldRestart(exitCode *int, startedAt time.Time,
	desiredStatus apicontainerstatus.ContainerStatus) (bool, string) {
	rt.lock.RLock()
	defer rt.lock.RUnlock()

	if !rt.RestartPolicy.Enabled {
		return false, "restart policy is not enabled"
	}
	if desiredStatus == apicontainerstatus.ContainerStopped {
		return false, "container's desired status is stopped"
	}
	if exitCode == nil {
		return false, "exit code is nil"
	}
	for _, ignoredCode := range rt.RestartPolicy.IgnoredExitCodes {
		if ignoredCode == *exitCode {
			return false, fmt.Sprintf("exit code %d should be ignored", *exitCode)
		}
	}

	startTime := startedAt
	if !rt.LastRestartAt.IsZero() {
		startTime = rt.LastRestartAt
	}
	if time.Since(startTime).Seconds() < float64(rt.RestartPolicy.RestartAttemptPeriod) {
		return false, "attempt reset period has not elapsed"
	}
	return true, ""
}
