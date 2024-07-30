//go:build windows && unit
// +build windows,unit

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
	dockerStat := getTestStatsJSONForOSDependentStats(1, 2, 3, 4, 5)
	lastStatBeforeLastRestart := getTestStatsJSONForOSDependentStats(5, 4, 3, 2, 1)
	expectedAggregatedStat := types.StatsJSON{
		Stats: types.Stats{
			MemoryStats: types.MemoryStats{
				CommitPeak: utils.MaxNum(dockerStat.MemoryStats.CommitPeak,
					lastStatBeforeLastRestart.MemoryStats.CommitPeak),
			},
			StorageStats: types.StorageStats{
				ReadCountNormalized: dockerStat.StorageStats.ReadCountNormalized +
					lastStatBeforeLastRestart.StorageStats.ReadCountNormalized,
				ReadSizeBytes: dockerStat.StorageStats.ReadSizeBytes +
					lastStatBeforeLastRestart.StorageStats.ReadSizeBytes,
				WriteCountNormalized: dockerStat.StorageStats.WriteCountNormalized +
					lastStatBeforeLastRestart.StorageStats.WriteCountNormalized,
				WriteSizeBytes: dockerStat.StorageStats.WriteSizeBytes +
					lastStatBeforeLastRestart.StorageStats.WriteSizeBytes,
			},
		},
	}

	dockerStat = aggregateOSDependentStats(dockerStat, lastStatBeforeLastRestart)
	require.Equal(t, expectedAggregatedStat, *dockerStat)
}

func getTestStatsJSONForOSDependentStats(commitPeak, readCountNormalized, readSizeBytes, writeCountNormalized,
	writeSizeBytes uint64) *types.StatsJSON {
	return &types.StatsJSON{
		Stats: types.Stats{
			MemoryStats: types.MemoryStats{
				CommitPeak: commitPeak,
			},
			StorageStats: types.StorageStats{
				ReadCountNormalized:  readCountNormalized,
				ReadSizeBytes:        readSizeBytes,
				WriteCountNormalized: writeCountNormalized,
				WriteSizeBytes:       writeSizeBytes,
			},
		},
	}
}
