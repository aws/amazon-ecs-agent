//go:build windows
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
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils"
	"github.com/docker/docker/api/types"
)

// aggregateOSDependentStats aggregates stats that are measured cumulatively against container start time and
// populated only for Windows OS.
func aggregateOSDependentStats(dockerStat, lastStatBeforeLastRestart *types.StatsJSON) *types.StatsJSON {
	// Memory stats.
	dockerStat.MemoryStats.CommitPeak = utils.MaxNum(dockerStat.MemoryStats.CommitPeak,
		lastStatBeforeLastRestart.MemoryStats.CommitPeak)

	// Disk I/O stats.
	dockerStat.StorageStats.ReadCountNormalized += lastStatBeforeLastRestart.StorageStats.ReadCountNormalized
	dockerStat.StorageStats.ReadSizeBytes += lastStatBeforeLastRestart.StorageStats.ReadSizeBytes
	dockerStat.StorageStats.WriteCountNormalized += lastStatBeforeLastRestart.StorageStats.WriteCountNormalized
	dockerStat.StorageStats.WriteSizeBytes += lastStatBeforeLastRestart.StorageStats.WriteSizeBytes

	return dockerStat
}
