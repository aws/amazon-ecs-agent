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
	"time"

	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/pkg/errors"
)

// Dummy StatsTasks
type StatsTask struct {
	// AWSVPC network stats only supported on linux
	TaskMetadata *TaskMetadata
	StatsQueue   *Queue
}

func newStatsTaskContainer(taskARN string, containerPID string, numberOfContainers int,
	resolver resolver.ContainerMetadataResolver, publishInterval time.Duration) (*StatsTask, error) {
	return nil, errors.New("Unsupported platform")
}

func (task *StatsTask) StartStatsCollection() {
	// AWSVPC network stats only supported on linux
}

func (task *StatsTask) StopStatsCollection() {
	// AWSVPC network stats only supported on linux
}
