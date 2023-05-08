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
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/cihub/seelog"
)

const systemPingTimeout = time.Second * 2

type dockerRuntimeHealthcheck struct {
	// HealthcheckType is the reported healthcheck type
	HealthcheckType string `json:"HealthcheckType,omitempty"`
	// Status is the container health status
	Status doctor.HealthcheckStatus `json:"HealthcheckStatus,omitempty"`
	// Timestamp is the timestamp when container health status changed
	TimeStamp time.Time `json:"TimeStamp,omitempty"`
	// StatusChangeTime is the latest time the health status changed
	StatusChangeTime time.Time `json:"StatusChangeTime,omitempty"`

	// LastStatus is the last container health status
	LastStatus doctor.HealthcheckStatus `json:"LastStatus,omitempty"`
	// LastTimeStamp is the timestamp of last container health status
	LastTimeStamp time.Time `json:"LastTimeStamp,omitempty"`

	client dockerapi.DockerClient
	lock   sync.RWMutex
}

func NewDockerRuntimeHealthcheck(client dockerapi.DockerClient) *dockerRuntimeHealthcheck {
	nowTime := time.Now()
	return &dockerRuntimeHealthcheck{
		HealthcheckType:  doctor.HealthcheckTypeContainerRuntime,
		Status:           doctor.HealthcheckStatusInitializing,
		TimeStamp:        nowTime,
		StatusChangeTime: nowTime,
		client:           client,
	}
}

func (dhc *dockerRuntimeHealthcheck) RunCheck() doctor.HealthcheckStatus {
	// TODO pass in context as an argument
	res := dhc.client.SystemPing(context.TODO(), systemPingTimeout)
	resultStatus := doctor.HealthcheckStatusOk
	if res.Error != nil {
		seelog.Infof("[DockerRuntimeHealthcheck] Docker Ping failed with error: %v", res.Error)
		resultStatus = doctor.HealthcheckStatusImpaired
	}
	dhc.SetHealthcheckStatus(resultStatus)
	return resultStatus
}

func (dhc *dockerRuntimeHealthcheck) SetHealthcheckStatus(healthStatus doctor.HealthcheckStatus) {
	dhc.lock.Lock()
	defer dhc.lock.Unlock()
	nowTime := time.Now()
	// if the status has changed, update status change timestamp
	if dhc.Status != healthStatus {
		dhc.StatusChangeTime = nowTime
	}
	// track previous status
	dhc.LastStatus = dhc.Status
	dhc.LastTimeStamp = dhc.TimeStamp

	// update latest status
	dhc.Status = healthStatus
	dhc.TimeStamp = nowTime
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckType() string {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.HealthcheckType
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckStatus() doctor.HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.Status
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.TimeStamp
}

func (dhc *dockerRuntimeHealthcheck) GetStatusChangeTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.StatusChangeTime
}

func (dhc *dockerRuntimeHealthcheck) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastStatus
}

func (dhc *dockerRuntimeHealthcheck) GetLastHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastTimeStamp
}
