//go:build !windows && unit
// +build !windows,unit

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
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerStatsToContainerStats(t *testing.T) {
	// numCores is a global variable in package agent/stats
	// which denotes the number of cpu cores
	// TODO should we take this from the docker stats `online_cpus`?
	numCores = 4
	inputJsonFile, _ := filepath.Abs("./unix_test_stats.json")
	jsonBytes, _ := ioutil.ReadFile(inputJsonFile)
	dockerStat := &types.StatsJSON{}
	json.Unmarshal([]byte(jsonBytes), dockerStat)
	containerStats, err := dockerStatsToContainerStats(dockerStat)
	assert.NoError(t, err, "converting container stats failed")
	require.NotNil(t, containerStats, "containerStats should not be nil")
	assert.Equal(t, uint64(65714455379), containerStats.cpuUsage, "unexpected value for cpuUsage", containerStats.cpuUsage)
	// storage bytes check
	assert.Equal(t, uint64(3), containerStats.storageReadBytes, "unexpected value for storageReadBytes", containerStats.storageReadBytes)
	assert.Equal(t, uint64(15), containerStats.storageWriteBytes, "Unexpected value for storageWriteBytes", containerStats.storageWriteBytes)
	// network stats check
	netStats := containerStats.networkStats
	assert.NotNil(t, netStats, "networkStats should not be nil")
	validateNetworkMetrics(t, netStats)
}

func TestDockerStatsToContainerStatsEmptyCpuUsageGeneratesError(t *testing.T) {
	inputJsonFile, _ := filepath.Abs("./unix_test_stats.json")
	jsonBytes, _ := ioutil.ReadFile(inputJsonFile)
	dockerStat := &types.StatsJSON{}
	json.Unmarshal([]byte(jsonBytes), dockerStat)
	prevNumCores := numCores
	numCores = uint64(0)
	err := validateDockerStats(dockerStat)
	assert.Error(t, err, "expected error converting container stats with numCores=0")
	numCores = prevNumCores
}
