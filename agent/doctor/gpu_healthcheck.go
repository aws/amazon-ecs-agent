//go:build linux

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
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/cihub/seelog"
)

type gpuHealthcheck struct {
	HealthcheckType  string
	Status           ecstcs.InstanceHealthCheckStatus
	TimeStamp        time.Time
	StatusChangeTime time.Time
	LastStatus       ecstcs.InstanceHealthCheckStatus
	LastTimeStamp    time.Time

	handler *gpu.DCGMHandler
	lock    sync.RWMutex
}

// NewGPUHealthcheck creates a new GPU health check that queries the DCGMHandler
// for health status from the shared GPU metrics file written by dcgm-init.
func NewGPUHealthcheck(handler *gpu.DCGMHandler) *gpuHealthcheck {
	nowTime := timeNow()
	return &gpuHealthcheck{
		HealthcheckType:  ecstcs.InstanceHealthCheckTypeAcceleratedCompute,
		Status:           ecstcs.InstanceHealthCheckStatusInitializing,
		TimeStamp:        nowTime,
		StatusChangeTime: nowTime,
		LastTimeStamp:    nowTime,
		handler:          handler,
	}
}

// RunCheck queries the DCGMHandler for GPU health status.
func (ghc *gpuHealthcheck) RunCheck() ecstcs.InstanceHealthCheckStatus {
	healthStatus := ghc.handler.GetGPUHealthStatus()
	if healthStatus == nil {
		seelog.Debug("[GPUHealthcheck] GPU health status not available")
		ghc.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusInsufficientData)
		return ecstcs.InstanceHealthCheckStatusInsufficientData
	}

	var resultStatus ecstcs.InstanceHealthCheckStatus
	if healthStatus.Healthy {
		resultStatus = ecstcs.InstanceHealthCheckStatusOk
	} else {
		seelog.Infof("[GPUHealthcheck] GPU reported unhealthy: %s", healthStatus.UnhealthyReason)
		resultStatus = ecstcs.InstanceHealthCheckStatusImpaired
	}

	ghc.SetHealthcheckStatus(resultStatus)
	return resultStatus
}

func (ghc *gpuHealthcheck) SetHealthcheckStatus(healthStatus ecstcs.InstanceHealthCheckStatus) {
	ghc.lock.Lock()
	defer ghc.lock.Unlock()
	nowTime := time.Now()
	if ghc.Status != healthStatus {
		ghc.StatusChangeTime = nowTime
	}
	ghc.LastStatus = ghc.Status
	ghc.LastTimeStamp = ghc.TimeStamp
	ghc.Status = healthStatus
	ghc.TimeStamp = nowTime
}

func (ghc *gpuHealthcheck) GetHealthcheckType() string {
	ghc.lock.RLock()
	defer ghc.lock.RUnlock()
	return ghc.HealthcheckType
}

func (ghc *gpuHealthcheck) GetHealthcheckStatus() ecstcs.InstanceHealthCheckStatus {
	ghc.lock.RLock()
	defer ghc.lock.RUnlock()
	return ghc.Status
}

func (ghc *gpuHealthcheck) GetHealthcheckTime() time.Time {
	ghc.lock.RLock()
	defer ghc.lock.RUnlock()
	return ghc.TimeStamp
}

func (ghc *gpuHealthcheck) GetStatusChangeTime() time.Time {
	ghc.lock.RLock()
	defer ghc.lock.RUnlock()
	return ghc.StatusChangeTime
}

func (ghc *gpuHealthcheck) GetLastHealthcheckStatus() ecstcs.InstanceHealthCheckStatus {
	ghc.lock.RLock()
	defer ghc.lock.RUnlock()
	return ghc.LastStatus
}

func (ghc *gpuHealthcheck) GetLastHealthcheckTime() time.Time {
	ghc.lock.RLock()
	defer ghc.lock.RUnlock()
	return ghc.LastTimeStamp
}
