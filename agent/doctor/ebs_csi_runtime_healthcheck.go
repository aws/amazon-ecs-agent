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
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/doctor/statustracker"
	"github.com/aws/amazon-ecs-agent/ecs-agent/csiclient"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
)

const (
	// DefaultEBSHealthRequestTimeout is the default request timeout for EBS CSI Daemon health check requests.
	DefaultEBSHealthRequestTimeout = 2 * time.Second
)

// ebsCSIDaemonHealthcheck is a health check for EBS CSI Daemon.
type ebsCSIDaemonHealthcheck struct {
	csiClient      csiclient.CSIClient
	requestTimeout time.Duration
	*statustracker.HealthCheckStatusTracker
}

// NewEBSCSIDaemonHealthCheck is the constructor for EBS CSI Daemon Health Check.
func NewEBSCSIDaemonHealthCheck(
	csiClient csiclient.CSIClient,
	requestTimeout time.Duration, // Timeout for health check requests.
) doctor.Healthcheck {
	return &ebsCSIDaemonHealthcheck{
		csiClient:                csiClient,
		requestTimeout:           requestTimeout,
		HealthCheckStatusTracker: statustracker.NewHealthCheckStatusTracker(),
	}
}

// RunCheck performs a health check for EBS CSI Daemon by sending a request to it to get node capabilities.
// If EBS CSI Daemon is not started yet then returns OK trivially.
func (e *ebsCSIDaemonHealthcheck) RunCheck() ecstcs.InstanceHealthCheckStatus {
	ctx, cancel := context.WithTimeout(context.Background(), e.requestTimeout)
	defer cancel()

	resp, err := e.csiClient.NodeGetCapabilities(ctx)
	if err != nil {
		logger.Error("EBS CSI Daemon health check failed", logger.Fields{field.Error: err})
		e.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusImpaired)
		return e.GetHealthcheckStatus()
	}

	logger.Info("EBS CSI Driver is healthy", logger.Fields{"nodeCapabilities": resp})
	e.SetHealthcheckStatus(ecstcs.InstanceHealthCheckStatusOk)
	return e.GetHealthcheckStatus()
}

// GetHealthcheckType returns the type of this health check.
func (e *ebsCSIDaemonHealthcheck) GetHealthcheckType() string {
	return ecstcs.InstanceHealthCheckTypeEBSDaemon
}
