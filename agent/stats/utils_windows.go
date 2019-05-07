// +build windows
// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
)

// dockerStatsToContainerStats returns a new object of the ContainerStats object from docker stats.
func dockerStatsToContainerStats(dockerStats *types.StatsJSON) (*ContainerStats, error) {
	if numCores == uint64(0) {
		seelog.Error("Invalid number of cpu cores acquired from the system")
		return nil, fmt.Errorf("invalid number of cpu cores acquired from the system")
	}

	cpuUsage := (dockerStats.CPUStats.CPUUsage.TotalUsage * 100) / numCores
	memoryUsage := dockerStats.MemoryStats.PrivateWorkingSet
	networkStats := getNetworkStats(dockerStats)
	storageReadBytes, storageWriteBytes := getStorageStats(dockerStats)
	return &ContainerStats{
		cpuUsage:          cpuUsage,
		memoryUsage:       memoryUsage,
		timestamp:         dockerStats.Read,
		storageReadBytes:  storageReadBytes,
		storageWriteBytes: storageWriteBytes,
		networkStats:      networkStats,
	}, nil
}

// Safe return of storage stats based on Docker Stats (Windows)
func getStorageStats(dockerStats *types.StatsJSON) (uint64, uint64) {
	if dockerStats == nil {
		return uint64(0), uint64(0)
	}
	if dockerStats.StorageStats.ReadSizeBytes == 0 && dockerStats.StorageStats.WriteSizeBytes == 0 {
		seelog.Debug("storage stats (Windows) not reported for container")
	}
	// windows StorageStats are zero-initialized if the stats struct is not
	// null, so return is guaranteed to be a uint64
	storageReadBytes := dockerStats.StorageStats.ReadSizeBytes
	storageWriteBytes := dockerStats.StorageStats.WriteSizeBytes
	return storageReadBytes, storageWriteBytes
}
