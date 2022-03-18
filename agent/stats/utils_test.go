//go:build unit
// +build unit

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
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

const (
	// below is the sum of each field in each network interface json in unix_test_stats.json
	expectedRxBytes   = uint64(1096)
	expectedRxPackets = uint64(14)
	expectedRxDropped = uint64(1)
	expectedRxErrors  = uint64(0)
	expectedTxBytes   = uint64(8992)
	expectedTxPackets = uint64(123)
	expectedTxDropped = uint64(10)
	expectedTxErrors  = uint64(0)
)

func TestDockerStatsToContainerStatsMemUsage(t *testing.T) {
	jsonStat := fmt.Sprintf(`
		{
			"cpu_stats":{
				"cpu_usage":{
					"percpu_usage":[%d, %d, %d, %d],
					"total_usage":%d
				}
			},
			"memory_stats":{
				"usage": %d,
				"max_usage": %d,
				"stats": {
					"cache": %d,
					"rss": %d
				},
				"privateworkingset": %d
			}
		}`, 1, 2, 3, 4, 100, 30, 100, 20, 10, 10)
	dockerStat := &types.StatsJSON{}
	json.Unmarshal([]byte(jsonStat), dockerStat)
	containerStats, err := dockerStatsToContainerStats(dockerStat)
	if err != nil {
		t.Errorf("Error converting container stats: %v", err)
	}
	if containerStats == nil {
		t.Fatal("containerStats should not be nil")
	}
	if containerStats.memoryUsage != 10 {
		t.Error("Unexpected value for memoryUsage", containerStats.memoryUsage)
	}
}

func validateNetworkMetrics(t *testing.T, netStats *NetworkStats) {
	assert.Equal(t, expectedRxBytes, netStats.RxBytes)
	assert.Equal(t, expectedRxPackets, netStats.RxPackets)
	assert.Equal(t, expectedRxDropped, netStats.RxDropped)
	assert.Equal(t, expectedRxErrors, netStats.RxErrors)
	assert.Equal(t, expectedTxBytes, netStats.TxBytes)
	assert.Equal(t, expectedTxPackets, netStats.TxPackets)
	assert.Equal(t, expectedTxDropped, netStats.TxDropped)
	assert.Equal(t, expectedTxErrors, netStats.TxErrors)
	assert.True(t, math.IsNaN(float64(netStats.RxBytesPerSecond)))
	assert.True(t, math.IsNaN(float64(netStats.TxBytesPerSecond)))
}
