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

package stats

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/docker/docker/api/types"
)

type BlockStatKey struct {
	Major uint64
	Minor uint64
	Op    string
}

type BlockStatValue struct {
	HasBeenRetrieved bool
	Value            uint64
}

// aggregateOSDependentStats aggregates stats that are measured cumulatively against container start time and
// populated only for Linux OS.
func aggregateOSDependentStats(dockerStat, lastStatBeforeLastRestart *types.StatsJSON) *types.StatsJSON {
	// CPU stats.
	aggregateUsagePerCore(&dockerStat.CPUStats.CPUUsage.PercpuUsage,
		lastStatBeforeLastRestart.CPUStats.CPUUsage.PercpuUsage)
	dockerStat.CPUStats.ThrottlingData.Periods += lastStatBeforeLastRestart.CPUStats.ThrottlingData.Periods
	dockerStat.CPUStats.ThrottlingData.ThrottledPeriods +=
		lastStatBeforeLastRestart.CPUStats.ThrottlingData.ThrottledPeriods
	dockerStat.CPUStats.ThrottlingData.ThrottledTime += lastStatBeforeLastRestart.CPUStats.ThrottlingData.ThrottledTime

	// Memory stats.
	dockerStat.MemoryStats.MaxUsage = utils.MaxNum(dockerStat.MemoryStats.MaxUsage,
		lastStatBeforeLastRestart.MemoryStats.MaxUsage)
	dockerStat.MemoryStats.Failcnt += lastStatBeforeLastRestart.MemoryStats.Failcnt

	// Block I/O stats.
	aggregateBlockStat(&dockerStat.BlkioStats.IoServiceBytesRecursive,
		lastStatBeforeLastRestart.BlkioStats.IoServiceBytesRecursive)
	aggregateBlockStat(&dockerStat.BlkioStats.IoServicedRecursive,
		lastStatBeforeLastRestart.BlkioStats.IoServicedRecursive)
	aggregateBlockStat(&dockerStat.BlkioStats.IoServiceTimeRecursive,
		lastStatBeforeLastRestart.BlkioStats.IoServiceTimeRecursive)
	aggregateBlockStat(&dockerStat.BlkioStats.IoWaitTimeRecursive,
		lastStatBeforeLastRestart.BlkioStats.IoWaitTimeRecursive)
	aggregateBlockStat(&dockerStat.BlkioStats.IoMergedRecursive, lastStatBeforeLastRestart.BlkioStats.IoMergedRecursive)
	aggregateBlockStat(&dockerStat.BlkioStats.IoTimeRecursive, lastStatBeforeLastRestart.BlkioStats.IoTimeRecursive)
	aggregateBlockStat(&dockerStat.BlkioStats.SectorsRecursive, lastStatBeforeLastRestart.BlkioStats.SectorsRecursive)

	// Network stats.
	for key, dockerStatNetwork := range dockerStat.Networks {
		lastStatBeforeLastRestartNetwork, ok := lastStatBeforeLastRestart.Networks[key]
		if ok {
			dockerStatNetwork.RxErrors += lastStatBeforeLastRestartNetwork.RxErrors
			dockerStatNetwork.TxErrors += lastStatBeforeLastRestartNetwork.TxErrors
		}
		dockerStat.Networks[key] = dockerStatNetwork
	}

	return dockerStat
}

// aggregateUsagePerCore aggregates the total CPU time consumed per core.
func aggregateUsagePerCore(dockerStatUsageSlice *[]uint64, lastStatBeforeLastRestartStatUsageSlice []uint64) {
	if len(*dockerStatUsageSlice) == 0 && len(lastStatBeforeLastRestartStatUsageSlice) == 0 {
		return
	}

	var aggregatedUsageSlice []uint64
	peakNumCores := utils.MaxNum(len(*dockerStatUsageSlice), len(lastStatBeforeLastRestartStatUsageSlice))
	for i := 0; i < peakNumCores; i++ {
		coreUsage := uint64(0)
		if i < len(lastStatBeforeLastRestartStatUsageSlice) {
			coreUsage += lastStatBeforeLastRestartStatUsageSlice[i]
		}
		if i < len(*dockerStatUsageSlice) {
			coreUsage += (*dockerStatUsageSlice)[i]
		}
		aggregatedUsageSlice = append(aggregatedUsageSlice, coreUsage)
	}
	*dockerStatUsageSlice = aggregatedUsageSlice
}

// aggregateBlockStat aggregates block I/O stats for the specified I/O service stat.
func aggregateBlockStat(dockerStatBlockStatSlice *[]types.BlkioStatEntry,
	lastStatBeforeLastRestartStatBlockStatSlice []types.BlkioStatEntry) {
	if len(*dockerStatBlockStatSlice) == 0 && len(lastStatBeforeLastRestartStatBlockStatSlice) == 0 {
		return
	}

	var aggregatedBlockStatSlice []types.BlkioStatEntry
	blockStatsMap := make(map[BlockStatKey]BlockStatValue)

	// Add block stat entries from stats to map (merging duplicates).
	addBlockStatEntriesFromSliceToMap(lastStatBeforeLastRestartStatBlockStatSlice, blockStatsMap)
	addBlockStatEntriesFromSliceToMap(*dockerStatBlockStatSlice, blockStatsMap)

	// Get entries from map and add them to the result slice in the same order they were originally encountered.
	addCorrespondingMapEntriesOfSliceToAggregatedSlice(blockStatsMap,
		lastStatBeforeLastRestartStatBlockStatSlice, &aggregatedBlockStatSlice)
	addCorrespondingMapEntriesOfSliceToAggregatedSlice(blockStatsMap,
		*dockerStatBlockStatSlice, &aggregatedBlockStatSlice)

	*dockerStatBlockStatSlice = aggregatedBlockStatSlice
}

func addBlockStatEntriesFromSliceToMap(statSlice []types.BlkioStatEntry, statMap map[BlockStatKey]BlockStatValue) {
	for _, blockStat := range statSlice {
		blkStatKey := BlockStatKey{blockStat.Major, blockStat.Minor, blockStat.Op}
		blkStatVal, ok := statMap[blkStatKey]
		if ok {
			blkStatVal.Value += blockStat.Value
		} else {
			blkStatVal = BlockStatValue{false, blockStat.Value}
		}
		statMap[blkStatKey] = blkStatVal
	}
}

func addCorrespondingMapEntriesOfSliceToAggregatedSlice(statMap map[BlockStatKey]BlockStatValue,
	statSlice []types.BlkioStatEntry, aggregatedSlice *[]types.BlkioStatEntry) {
	for _, blockStat := range statSlice {
		blkStatKey := BlockStatKey{blockStat.Major, blockStat.Minor, blockStat.Op}
		blkStatVal, ok := statMap[blkStatKey]
		if ok && !blkStatVal.HasBeenRetrieved {
			*aggregatedSlice = append(*aggregatedSlice, types.BlkioStatEntry{
				Major: blockStat.Major, Minor: blockStat.Minor, Op: blockStat.Op, Value: blkStatVal.Value})
			blkStatVal.HasBeenRetrieved = true
			statMap[blkStatKey] = blkStatVal
		}
	}
}
