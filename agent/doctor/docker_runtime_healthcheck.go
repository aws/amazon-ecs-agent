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
	"github.com/cihub/seelog"
)

const systemPingTimeout = time.Second * 2

type dockerRuntimeHealthcheck struct {
	// Status is the container health status
	Status HealthcheckStatus `json:"Status,omitempty"`
	// Timestamp is the timestamp when container health status changed
	TimeStamp time.Time `json:"TimeStamp,omitempty"`
	// LastStatus is the last container health status
	LastStatus HealthcheckStatus `json:"lastStatus,omitempty"`
	// LastTimeStamp is the timestamp of last container health status
	LastTimeStamp time.Time `json:"LastTimeStamp,omitempty"`
	client        dockerapi.DockerClient
	lock          sync.RWMutex
}

func NewDockerRuntimeHealthcheck(client dockerapi.DockerClient) *dockerRuntimeHealthcheck {
	return &dockerRuntimeHealthcheck{
		client:    client,
		Status:    HealthcheckStatusInitializing,
		TimeStamp: time.Now(),
	}
}

func (dhc *dockerRuntimeHealthcheck) RunCheck() HealthcheckStatus {
	// TODO pass in context as an argument
	res := dhc.client.SystemPing(context.TODO(), systemPingTimeout)
	healthcheck := HealthcheckStatusOk
	if res.Error != nil {
		seelog.Infof("[DockerHealthcheck] Docker Ping failed with error: %v", res.Error)
		healthcheck = HealthcheckStatusImpaired
	}
	dhc.SetHealthcheckStatus(healthcheck)
	return healthcheck
}

func (dhc *dockerRuntimeHealthcheck) SetHealthcheckStatus(healthStatus HealthcheckStatus) {
	dhc.lock.Lock()
	defer dhc.lock.Unlock()
	dhc.LastStatus = dhc.Status
	dhc.Status = healthStatus
	dhc.LastTimeStamp = dhc.TimeStamp
	dhc.TimeStamp = time.Now()
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.Status
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.TimeStamp
}

func (dhc *dockerRuntimeHealthcheck) GetLastHealthcheckStatus() HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastStatus
}

func (dhc *dockerRuntimeHealthcheck) GetLastHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastTimeStamp
}
