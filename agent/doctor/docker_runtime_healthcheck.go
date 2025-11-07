// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package doctor

import (
	"context"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	ecsdoctor "github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/cihub/seelog"
)

const systemPingTimeout = time.Second * 2

var timeNow = time.Now

type dockerRuntimeHealthcheck struct {
	// HealthcheckType is the reported healthcheck type.
	HealthcheckType string `json:"HealthcheckType,omitempty"`
	// Status is the container health status.
	Status ecstcs.InstanceHealthcheckStatus `json:"HealthcheckStatus,omitempty"`
	// TimeStamp is the timestamp when container health status changed.
	TimeStamp time.Time `json:"TimeStamp,omitempty"`
	// StatusChangeTime is the latest time the health status changed.
	StatusChangeTime time.Time `json:"StatusChangeTime,omitempty"`

	// LastStatus is the last container health status.
	LastStatus ecstcs.InstanceHealthcheckStatus `json:"LastStatus,omitempty"`
	// LastTimeStamp is the timestamp of last container health status.
	LastTimeStamp time.Time `json:"LastTimeStamp,omitempty"`

	client dockerapi.DockerClient
	lock   sync.RWMutex
}

// NewDockerRuntimeHealthcheck creates a new Docker runtime health check.
func NewDockerRuntimeHealthcheck(client dockerapi.DockerClient) *dockerRuntimeHealthcheck {
	nowTime := timeNow()
	return &dockerRuntimeHealthcheck{
		HealthcheckType:  ecsdoctor.HealthcheckTypeContainerRuntime,
		Status:           ecstcs.InstanceHealthcheckStatusInitializing,
		TimeStamp:        nowTime,
		StatusChangeTime: nowTime,
		LastTimeStamp:    nowTime,
		client:           client,
	}
}

// RunCheck performs a health check by pinging the Docker daemon.
func (dhc *dockerRuntimeHealthcheck) RunCheck() ecstcs.InstanceHealthcheckStatus {
	// TODO: Pass in context as an argument.
	res := dhc.client.SystemPing(context.TODO(), systemPingTimeout)
	resultStatus := ecstcs.InstanceHealthcheckStatusOk
	if res.Error != nil {
		seelog.Infof("[DockerRuntimeHealthcheck] Docker Ping failed with error: %v", res.Error)
		resultStatus = ecstcs.InstanceHealthcheckStatusImpaired
	}
	dhc.SetHealthcheckStatus(resultStatus)
	return resultStatus
}

// SetHealthcheckStatus updates the health check status and timestamps.
func (dhc *dockerRuntimeHealthcheck) SetHealthcheckStatus(healthStatus ecstcs.InstanceHealthcheckStatus) {
	dhc.lock.Lock()
	defer dhc.lock.Unlock()
	nowTime := time.Now()
	// If the status has changed, update status change timestamp.
	if dhc.Status != healthStatus {
		dhc.StatusChangeTime = nowTime
	}
	// Track previous status.
	dhc.LastStatus = dhc.Status
	dhc.LastTimeStamp = dhc.TimeStamp

	// Update latest status.
	dhc.Status = healthStatus
	dhc.TimeStamp = nowTime
}

// GetHealthcheckType returns the type of this health check.
func (dhc *dockerRuntimeHealthcheck) GetHealthcheckType() string {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.HealthcheckType
}

// GetHealthcheckStatus returns the current health check status.
func (dhc *dockerRuntimeHealthcheck) GetHealthcheckStatus() ecstcs.InstanceHealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.Status
}

// GetHealthcheckTime returns the timestamp of the current health check status.
func (dhc *dockerRuntimeHealthcheck) GetHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.TimeStamp
}

// GetStatusChangeTime returns the timestamp when the status last changed.
func (dhc *dockerRuntimeHealthcheck) GetStatusChangeTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.StatusChangeTime
}

// GetLastHealthcheckStatus returns the previous health check status.
func (dhc *dockerRuntimeHealthcheck) GetLastHealthcheckStatus() ecstcs.InstanceHealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastStatus
}

// GetLastHealthcheckTime returns the timestamp of the previous health check status.
func (dhc *dockerRuntimeHealthcheck) GetLastHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastTimeStamp
}
