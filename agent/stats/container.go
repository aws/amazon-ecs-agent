// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/cihub/seelog"
	"golang.org/x/net/context"
)

const (
	// SleepBetweenUsageDataCollection is the sleep duration between collecting usage data for a container.
	SleepBetweenUsageDataCollection = 1 * time.Second

	// ContainerStatsBufferLength is the number of usage metrics stored in memory for a container. It is calculated as
	// Number of usage metrics gathered in a second (1) * 60 * Time duration in minutes to store the data for (2)
	ContainerStatsBufferLength = 120
)

func newStatsContainer(dockerID string, client ecsengine.DockerClient) *StatsContainer {
	ctx, cancel := context.WithCancel(context.Background())
	return &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID: dockerID,
		},
		ctx:    ctx,
		cancel: cancel,
		client: client,
	}
}

func (container *StatsContainer) StartStatsCollection() {
	// Create the queue to store utilization data from docker stats
	container.statsQueue = NewQueue(ContainerStatsBufferLength)

	go container.collect()
}

func (container *StatsContainer) StopStatsCollection() {
	container.cancel()
}

func (container *StatsContainer) collect() {
	dockerID := container.containerMetadata.DockerID
	for {
		select {
		case <-container.ctx.Done():
			seelog.Debugf("Stopping stats collection for container %s", dockerID)
			return
		default:
			seelog.Debugf("Collecting stats for container %s", dockerID)
			dockerStats, err := container.client.Stats(dockerID, container.ctx)
			if err != nil {
				seelog.Warnf("Error retrieving stats for container %s: %v", dockerID, err)
				continue
			}
			for rawStat := range dockerStats {
				stat, err := dockerStatsToConatinerStats(rawStat)
				if err == nil {
					container.statsQueue.Add(stat)
				} else {
					seelog.Warnf("Error converting stats for container %s: %v", dockerID, err)
				}
			}
			seelog.Debugf("Disconnected from docker stats for container %s", dockerID)
		}
	}
}
