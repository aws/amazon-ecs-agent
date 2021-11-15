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
	"context"
	"fmt"
	"time"

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	dockerstats "github.com/docker/docker/api/types"
)

// statsTaskCommon contains the common fields in StatsTask for both Linux and Windows.
// StatsTask abstracts methods to gather and aggregate network data for a task. Used only for AWSVPC mode.
type statsTaskCommon struct {
	StatsQueue            *Queue
	TaskMetadata          *TaskMetadata
	Ctx                   context.Context
	Cancel                context.CancelFunc
	Resolver              resolver.ContainerMetadataResolver
	metricPublishInterval time.Duration
}

func (taskStat *StatsTask) StartStatsCollection() {
	queueSize := int(config.DefaultContainerMetricsPublishInterval.Seconds() * 4)
	taskStat.StatsQueue = NewQueue(queueSize)
	taskStat.StatsQueue.Reset()
	go taskStat.collect()
}

func (taskStat *StatsTask) StopStatsCollection() {
	taskStat.Cancel()
}

func (taskStat *StatsTask) collect() {
	taskArn := taskStat.TaskMetadata.TaskArn
	backoff := retry.NewExponentialBackoff(time.Second*1, time.Second*10, 0.5, 2)

	for {
		err := taskStat.processStatsStream()
		select {
		case <-taskStat.Ctx.Done():
			seelog.Debugf("Stopping stats collection for taskStat %s", taskArn)
			return
		default:
			if err != nil {
				d := backoff.Duration()
				time.Sleep(d)
				seelog.Debugf("Error querying stats for task %s: %v", taskArn, err)
			}
			// We were disconnected from the stats stream.
			// Check if the task is terminal. If it is, stop collecting metrics.
			terminal, err := taskStat.terminal()
			if err != nil {
				// Error determining if the task is terminal. clean-up anyway.
				seelog.Warnf("Error determining if the task %s is terminal, stopping stats collection: %v",
					taskArn, err)
				taskStat.StopStatsCollection()
			} else if terminal {
				seelog.Infof("Task %s is terminal, stopping stats collection", taskArn)
				taskStat.StopStatsCollection()
			}
		}
	}
}

func (taskStat *StatsTask) processStatsStream() error {
	taskArn := taskStat.TaskMetadata.TaskArn
	awsvpcNetworkStats, errC := taskStat.getAWSVPCNetworkStats()

	returnError := false
	for {
		select {
		case <-taskStat.Ctx.Done():
			seelog.Info("task context is done")
			return nil
		case err := <-errC:
			seelog.Warnf("Error encountered processing metrics stream from host, this may affect "+
				"cloudwatch metric accuracy: %s", err)
			returnError = true
		case rawStat, ok := <-awsvpcNetworkStats:
			if !ok {
				if returnError {
					return fmt.Errorf("error encountered processing metrics stream from host")
				}
				return nil
			}
			if err := taskStat.StatsQueue.Add(rawStat); err != nil {
				seelog.Warnf("Task [%s]: error converting stats: %v", taskArn, err)
			}
		}

	}
}

func (taskStat *StatsTask) terminal() (bool, error) {
	resolvedTask, err := taskStat.Resolver.ResolveTaskByARN(taskStat.TaskMetadata.TaskArn)
	if err != nil {
		return false, err
	}
	return resolvedTask.GetKnownStatus() == apitaskstatus.TaskStopped, nil
}

func (taskStat *StatsTask) getAWSVPCNetworkStats() (<-chan *types.StatsJSON, <-chan error) {

	errC := make(chan error, 1)
	statsC := make(chan *dockerstats.StatsJSON)
	if taskStat.TaskMetadata.NumberContainers > 0 {
		go func() {
			defer close(statsC)
			statPollTicker := time.NewTicker(taskStat.metricPublishInterval)
			defer statPollTicker.Stop()
			for range statPollTicker.C {

				networkStats, err := taskStat.retrieveNetworkStatistics()
				if err != nil {
					errC <- err
					return
				}

				dockerStats := &types.StatsJSON{
					Networks: networkStats,
					Stats: types.Stats{
						Read: time.Now(),
					},
				}
				select {
				case <-taskStat.Ctx.Done():
					return
				case statsC <- dockerStats:
				}
			}
		}()
	}

	return statsC, errC
}
