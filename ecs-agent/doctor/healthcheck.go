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

	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
)

const (
	HealthcheckTypeContainerRuntime = "ContainerRuntime"
	HealthcheckTypeAgent            = "Agent"
	HealthcheckTypeEBSDaemon        = "EBSDaemon"
)

// Healthcheck defines the interface for performing health checks on various components.
type Healthcheck interface {
	GetHealthcheckType() string
	GetHealthcheckStatus() ecstcs.HealthcheckStatus
	GetHealthcheckTime() time.Time
	GetStatusChangeTime() time.Time
	GetLastHealthcheckStatus() ecstcs.HealthcheckStatus
	GetLastHealthcheckTime() time.Time
	RunCheck() ecstcs.HealthcheckStatus
	SetHealthcheckStatus(status ecstcs.HealthcheckStatus)
}
