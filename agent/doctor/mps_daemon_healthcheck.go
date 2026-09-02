//go:build linux
// +build linux

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

package doctor

import (
	"fmt"
	"os"

	"github.com/aws/amazon-ecs-agent/agent/doctor/statustracker"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/mps"
)

// mpsDaemonImpairedThreshold is the number of consecutive probe failures before the
// instance is reported ACCELERATED_COMPUTE=IMPAIRED
const mpsDaemonImpairedThreshold = 3

type mpsDaemonHealthcheck struct {
	*statustracker.HealthCheckStatusTracker

	exec        execwrapper.Exec
	statPipeDir func(string) (os.FileInfo, error)
	probeCmd    string

	consecutiveFailures int
	threshold           int
}

// NewMpsDaemonHealthcheck is the constructor for the MPS control-daemon health check.
func NewMpsDaemonHealthcheck() doctor.Healthcheck {
	return newMpsDaemonHealthcheck(execwrapper.NewExec(), os.Stat)
}

func newMpsDaemonHealthcheck(exec execwrapper.Exec,
	statPipeDir func(string) (os.FileInfo, error)) *mpsDaemonHealthcheck {
	return &mpsDaemonHealthcheck{
		HealthCheckStatusTracker: statustracker.NewHealthCheckStatusTracker(),
		exec:                     exec,
		statPipeDir:              statPipeDir,
		probeCmd:                 mps.ProbeCommand,
		threshold:                mpsDaemonImpairedThreshold,
	}
}

// RunCheck runs one probe per tick and folds the result into the consecutive-failure
// counter. It reports Impaired only after threshold failures in a row; any success
// resets the counter, returning the status to Ok.
func (m *mpsDaemonHealthcheck) RunCheck() ecstcs.InstanceHealthCheckStatus {
	res := m.probe()
	serving := res.Err == nil

	if serving {
		m.consecutiveFailures = 0
	} else {
		m.consecutiveFailures++
	}

	status := ecstcs.InstanceHealthCheckStatusOk
	if m.consecutiveFailures >= m.threshold {
		status = ecstcs.InstanceHealthCheckStatusImpaired
	}

	switch {
	case status == ecstcs.InstanceHealthCheckStatusImpaired:
		logger.Error("MPS control daemon health check impaired", logger.Fields{
			"consecutiveFailures": m.consecutiveFailures,
			"timedOut":            res.TimedOut,
			field.Error:           res.Err,
		})
	case !serving:
		logger.Warn("MPS control daemon probe failed, below impairment threshold", logger.Fields{
			"consecutiveFailures": m.consecutiveFailures,
			"timedOut":            res.TimedOut,
			field.Error:           res.Err,
		})
	default:
		logger.Debug("MPS control daemon is serving")
	}

	m.SetHealthcheckStatus(status)
	return m.GetHealthcheckStatus()
}

// probe runs the pipe-directory pre-check and, if it passes, a single control-daemon
// probe. The daemon is serving when the returned result has a nil Err. An unusable
// pipe directory counts as this tick's failure and skips the exec; it can recover on a
// later tick.
func (m *mpsDaemonHealthcheck) probe() mps.ProbeResult {
	fi, err := m.statPipeDir(mps.PipeDirectory)
	if err != nil {
		return mps.ProbeResult{Err: err}
	}
	if !fi.IsDir() {
		return mps.ProbeResult{
			Err: fmt.Errorf("mps pipe directory %s is not a directory", mps.PipeDirectory),
		}
	}
	return mps.ProbeControlDaemon(m.exec, m.probeCmd)
}

// GetHealthcheckType returns the type of this health check.
func (m *mpsDaemonHealthcheck) GetHealthcheckType() string {
	return ecstcs.InstanceHealthCheckTypeAcceleratedCompute
}
