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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/doctor/statustracker"
	"github.com/aws/amazon-ecs-agent/agent/gpu"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/cihub/seelog"
)

// gpuBootGracePeriod tolerates a missing file until dcgm-init's first write
// (~60s in, plus jitter/skew). 2x the 60s producer tick.
const gpuBootGracePeriod = 120 * time.Second

// gpuStalenessThreshold is the max snapshot age before the file is stale
// (INSUFFICIENT_DATA), catching a dead dcgm-init. 2x the 60s producer tick.
const gpuStalenessThreshold = 120 * time.Second

// gpuHealthcheck implements the ACCELERATED_COMPUTE check from the shared
// metrics file written by dcgm-init.
type gpuHealthcheck struct {
	reader    *gpu.DCGMMetricsReader
	createdAt time.Time
	*statustracker.HealthCheckStatusTracker
}

// NewGPUHealthcheck creates a GPU health check backed by the shared metrics
// file. It starts INITIALIZING and derives status on the first successful read.
func NewGPUHealthcheck(reader *gpu.DCGMMetricsReader) doctor.Healthcheck {
	return &gpuHealthcheck{
		reader:                   reader,
		createdAt:                timeNow(),
		HealthCheckStatusTracker: statustracker.NewHealthCheckStatusTracker(),
	}
}

// GetHealthcheckType returns the ACCELERATED_COMPUTE type.
func (ghc *gpuHealthcheck) GetHealthcheckType() string {
	return ecstcs.InstanceHealthCheckTypeAcceleratedCompute
}

// RunCheck reads the shared GPU metrics file and derives the health status.
//
// Decision order (first match wins):
//  1. Reader returns nil (missing/corrupt) → INSUFFICIENT_DATA, or INITIALIZING
//     within gpuBootGracePeriod.
//  2. Timestamp older than gpuStalenessThreshold → INSUFFICIENT_DATA.
//  3. ConnectionLost → INSUFFICIENT_DATA (health unknown).
//  4. Healthy → OK, else IMPAIRED.
func (ghc *gpuHealthcheck) RunCheck() ecstcs.InstanceHealthCheckStatus {
	healthStatus := ghc.reader.GetGPUMetrics()
	if healthStatus == nil {
		if ghc.GetHealthcheckStatus() == ecstcs.InstanceHealthCheckStatusInitializing &&
			timeNow().Sub(ghc.createdAt) < gpuBootGracePeriod {
			seelog.Debug("[GPUHealthcheck] GPU health status not yet available (within boot grace)")
			return ecstcs.InstanceHealthCheckStatusInitializing
		}
		seelog.Debug("[GPUHealthcheck] GPU health status not available")
		ghc.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusInsufficientData)
		return ecstcs.InstanceHealthCheckStatusInsufficientData
	}

	// A stale timestamp means dcgm-init stopped writing; the verdict is unreliable.
	if ts, err := time.Parse(time.RFC3339, healthStatus.Timestamp); err == nil {
		if timeNow().Sub(ts) > gpuStalenessThreshold {
			seelog.Infof("[GPUHealthcheck] GPU metrics file is stale (age %v > %v)", timeNow().Sub(ts), gpuStalenessThreshold)
			ghc.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusInsufficientData)
			return ecstcs.InstanceHealthCheckStatusInsufficientData
		}
	}

	if healthStatus.ConnectionLost {
		seelog.Info("[GPUHealthcheck] DCGM connection lost, reporting insufficient data")
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
