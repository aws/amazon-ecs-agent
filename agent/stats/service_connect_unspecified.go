//go:build !linux

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

package stats

import (
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"

	tcstypes "github.com/aws/aws-sdk-go-v2/service/tcs/types"
	"github.com/pkg/errors"
)

type ServiceConnectStats struct {
	stats []*tcstypes.GeneralMetricsWrapper
	sent  bool
}

func newServiceConnectStats() (*ServiceConnectStats, error) {
	return nil, errors.New("Unsupported platform")
}

func (sc *ServiceConnectStats) retrieveServiceConnectStats(task *apitask.Task) {
}

func (sc *ServiceConnectStats) GetStats() []*tcstypes.GeneralMetricsWrapper {
	return nil
}

func (sc *ServiceConnectStats) SetStatsSent(sent bool) {
	sc.sent = false
}

func (sc *ServiceConnectStats) HasStatsBeenSent() bool {
	return false
}
