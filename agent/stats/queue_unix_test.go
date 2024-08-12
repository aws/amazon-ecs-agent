//go:build linux && unit
// +build linux,unit

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
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

func TestAggregateOSDependentStats(t *testing.T) {
	dockerStat := getTestStatsJSONForOSDependentStats(1, 2, 3, 4, 5, 6, 7, 8, 9,
		[]types.BlkioStatEntry{
			{
				Major: 202,
				Minor: 192,
				Op:    "Read",
				Value: 1,
			},
			{
				Major: 202,
				Minor: 192,
				Op:    "Write",
				Value: 2,
			},
			{
				Major: 253,
				Minor: 1,
				Op:    "Read",
				Value: 3,
			},
			{
				Major: 253,
				Minor: 1,
				Op:    "Write",
				Value: 4,
			},
		})
	lastStatBeforeLastRestart := getTestStatsJSONForOSDependentStats(1, 2, 3, 4, 5, 6, 7, 8, 9,
		[]types.BlkioStatEntry{
			{
				Major: 253,
				Minor: 1,
				Op:    "Read",
				Value: 1234,
			},
			{
				Major: 253,
				Minor: 1,
				Op:    "Write",
				Value: 5678,
			},
		})

	// Sanity length checks/enforcement.
	require.Equal(t, 2, len(dockerStat.CPUStats.CPUUsage.PercpuUsage))
	require.Equal(t, 2, len(lastStatBeforeLastRestart.CPUStats.CPUUsage.PercpuUsage))
	require.Equal(t, 4, len(dockerStat.BlkioStats.IoServiceBytesRecursive))
	require.Equal(t, 2, len(lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive))

	expectedAggregatedStat := types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					PercpuUsage: []uint64{
						dockerStat.Stats.CPUStats.CPUUsage.PercpuUsage[0] +
							lastStatBeforeLastRestart.Stats.CPUStats.CPUUsage.PercpuUsage[0],
						dockerStat.Stats.CPUStats.CPUUsage.PercpuUsage[1] +
							lastStatBeforeLastRestart.Stats.CPUStats.CPUUsage.PercpuUsage[1]},
				},
				ThrottlingData: types.ThrottlingData{
					Periods: dockerStat.Stats.CPUStats.ThrottlingData.Periods +
						lastStatBeforeLastRestart.Stats.CPUStats.ThrottlingData.Periods,
					ThrottledPeriods: dockerStat.Stats.CPUStats.ThrottlingData.ThrottledPeriods +
						lastStatBeforeLastRestart.Stats.CPUStats.ThrottlingData.ThrottledPeriods,
					ThrottledTime: dockerStat.Stats.CPUStats.ThrottlingData.ThrottledTime +
						lastStatBeforeLastRestart.Stats.CPUStats.ThrottlingData.ThrottledTime,
				},
			},
			MemoryStats: types.MemoryStats{
				MaxUsage: utils.MaxNum(dockerStat.MemoryStats.MaxUsage, lastStatBeforeLastRestart.MemoryStats.MaxUsage),
				Failcnt:  dockerStat.MemoryStats.Failcnt + lastStatBeforeLastRestart.MemoryStats.Failcnt,
			},
			BlkioStats: types.BlkioStats{
				IoServiceBytesRecursive: []types.BlkioStatEntry{
					{
						Major: lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[0].Major,
						Minor: lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[0].Minor,
						Op:    lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[0].Op,
						Value: lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[0].Value +
							dockerStat.BlkioStats.IoServiceBytesRecursive[2].Value,
					},
					{
						Major: lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[1].Major,
						Minor: lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[1].Minor,
						Op:    lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[1].Op,
						Value: lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive[1].Value +
							dockerStat.BlkioStats.IoServiceBytesRecursive[3].Value,
					},
					{
						Major: dockerStat.BlkioStats.IoServiceBytesRecursive[0].Major,
						Minor: dockerStat.BlkioStats.IoServiceBytesRecursive[0].Minor,
						Op:    dockerStat.BlkioStats.IoServiceBytesRecursive[0].Op,
						Value: dockerStat.BlkioStats.IoServiceBytesRecursive[0].Value,
					},
					{
						Major: dockerStat.BlkioStats.IoServiceBytesRecursive[1].Major,
						Minor: dockerStat.BlkioStats.IoServiceBytesRecursive[1].Minor,
						Op:    dockerStat.BlkioStats.IoServiceBytesRecursive[1].Op,
						Value: dockerStat.BlkioStats.IoServiceBytesRecursive[1].Value,
					},
				},
			},
		},
		Networks: map[string]types.NetworkStats{
			testNetworkNameA: {
				RxErrors: dockerStat.Networks[testNetworkNameA].RxErrors +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].RxErrors,
				TxErrors: dockerStat.Networks[testNetworkNameA].TxErrors +
					lastStatBeforeLastRestart.Networks[testNetworkNameA].TxErrors,
			},
			testNetworkNameB: {
				RxErrors: dockerStat.Networks[testNetworkNameB].RxErrors +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].RxErrors,
				TxErrors: dockerStat.Networks[testNetworkNameB].TxErrors +
					lastStatBeforeLastRestart.Networks[testNetworkNameB].TxErrors,
			},
		},
	}

	dockerStat = aggregateOSDependentStats(dockerStat, lastStatBeforeLastRestart)
	require.Equal(t, expectedAggregatedStat, *dockerStat)
}

func getTestStatsJSONForOSDependentStats(usageCoreA, usageCoreB, periods, throttledPeriods, throttledTime, maxUsage,
	failCnt, rxErrors, txErrors uint64, ioServiceBytesRecursive []types.BlkioStatEntry) *types.StatsJSON {
	return &types.StatsJSON{
		Stats: types.Stats{
			CPUStats: types.CPUStats{
				CPUUsage: types.CPUUsage{
					PercpuUsage: []uint64{usageCoreA, usageCoreB},
				},
				ThrottlingData: types.ThrottlingData{
					Periods:          periods,
					ThrottledPeriods: throttledPeriods,
					ThrottledTime:    throttledTime,
				},
			},
			MemoryStats: types.MemoryStats{
				MaxUsage: maxUsage,
				Failcnt:  failCnt,
			},
			BlkioStats: types.BlkioStats{
				IoServiceBytesRecursive: ioServiceBytesRecursive,
			},
		},
		Networks: map[string]types.NetworkStats{
			testNetworkNameA: {
				RxErrors: rxErrors,
				TxErrors: txErrors,
			},
			testNetworkNameB: {
				RxErrors: rxErrors + 1,
				TxErrors: txErrors + 1,
			},
		},
	}
}
