// +build windows
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
	"fmt"

	"github.com/docker/docker/api/types"
)

// dockerStatsToContainerStats returns a new object of the ContainerStats object from docker stats.
func dockerStatsToContainerStats(dockerStats *types.StatsJSON) (*ContainerStats, error) {
	cpuUsage := (dockerStats.CPUStats.CPUUsage.TotalUsage * 100) / numCores
	memoryUsage := dockerStats.MemoryStats.PrivateWorkingSet
	networkStats := getNetworkStats(dockerStats)
	storageReadBytes := dockerStats.StorageStats.ReadSizeBytes
	storageWriteBytes := dockerStats.StorageStats.WriteSizeBytes
	return &ContainerStats{
		cpuUsage:          cpuUsage,
		memoryUsage:       memoryUsage,
		timestamp:         dockerStats.Read,
		storageReadBytes:  storageReadBytes,
		storageWriteBytes: storageWriteBytes,
		networkStats:      networkStats,
	}, nil
}

func validateDockerStats(dockerStats *types.StatsJSON) error {
	if numCores == uint64(0) {
		return fmt.Errorf("invalid container statistics reported, no cpu core usage reported")
	}
	return nil
}
