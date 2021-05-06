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

type Healthcheck interface {
	GetHealthcheckStatus() HealthcheckStatus
	GetHealthcheckTime() time.Time
	RunCheck() bool
}

type dockerRuntimeHealthcheck struct {
	// Status is the container health status
	LatestCheckStatus HealthcheckStatus `json:"latestStatus,omitempty"`
	// Since is the timestamp when container health status changed
	LatestCheckTime time.Time `json:"latestStatusTime,omitempty"`
	// Status is the container health status
	LastCheckStatus HealthcheckStatus `json:"lastStatus,omitempty"`
	// Since is the timestamp when container health status changed
	LastCheckTime time.Time `json:"lastStatusTime,omitempty"`
	client        dockerapi.DockerClient
	lock          sync.RWMutex
}

func NewDockerRuntimeHealthcheck(client dockerapi.DockerClient) *dockerRuntimeHealthcheck {
	return &dockerRuntimeHealthcheck{
		client:            client,
		LatestCheckStatus: HealthcheckStatusInitializing,
		LatestCheckTime:   time.Now(),
		LastCheckStatus:   HealthcheckStatusInitializing,
		LastCheckTime:     time.Now(),
	}
}

func (dhc *dockerRuntimeHealthcheck) RunCheck() bool {
	healthcheckResponse := dhc.client.SystemPing(context.TODO(), time.Second*2)
	seelog.Infof("container runtime healthcheck response: %v", healthcheckResponse)
	return true
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckStatus() HealthcheckStatus {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LatestCheckStatus
}

func (dhc *dockerRuntimeHealthcheck) GetHealthcheckTime() time.Time {
	dhc.lock.RLock()
	defer dhc.lock.RUnlock()
	return dhc.LatestCheckTime
}
