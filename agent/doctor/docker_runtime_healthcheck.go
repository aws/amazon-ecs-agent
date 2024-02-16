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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/cihub/seelog"
)

const systemPingTimeout = time.Second * 2

type DockerRuntimeHealthcheck struct {
	commonHealthcheckConfig
	client dockerapi.DockerClient
}

func NewDockerRuntimeHealthcheck(client dockerapi.DockerClient) *DockerRuntimeHealthcheck {
	nowTime := time.Now()
	return &DockerRuntimeHealthcheck{
		commonHealthcheckConfig: commonHealthcheckConfig{
			HealthcheckType:  doctor.HealthcheckTypeContainerRuntime,
			HealthcheckName:  "Docker",
			Status:           doctor.HealthcheckStatusInitializing,
			TimeStamp:        nowTime,
			StatusChangeTime: nowTime,
		},
		client: client,
	}
}

func (dhc *DockerRuntimeHealthcheck) RunCheck() doctor.HealthcheckStatus {
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

func (dhc *DockerRuntimeHealthcheck) SetHealthcheckStatus(healthStatus doctor.HealthcheckStatus) {
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

func (dhc *DockerRuntimeHealthcheck) GetHealthcheckType() string {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.HealthcheckType
}

func (dhc *DockerRuntimeHealthcheck) GetHealthcheckName() string {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.HealthcheckName
}

func (dhc *DockerRuntimeHealthcheck) GetHealthcheckStatus() doctor.HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.Status
}

func (dhc *DockerRuntimeHealthcheck) GetHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.TimeStamp
}

func (dhc *DockerRuntimeHealthcheck) GetStatusChangeTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.StatusChangeTime
}

func (dhc *DockerRuntimeHealthcheck) GetLastHealthcheckStatus() doctor.HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastStatus
}

func (dhc *DockerRuntimeHealthcheck) GetLastHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LastTimeStamp
}
