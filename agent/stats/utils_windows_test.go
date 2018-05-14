// +build windows,unit

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerStatsToContainerStatsZeroCoresGeneratesError(t *testing.T) {
	numCores = uint64(0)
	jsonStat := fmt.Sprintf(`
		{
			"cpu_stats":{
				"cpu_usage":{
					"total_usage":%d
				}
			}
		}`, 100)
	dockerStat := &docker.Stats{}
	json.Unmarshal([]byte(jsonStat), dockerStat)
	_, err := dockerStatsToContainerStats(dockerStat)
	assert.Error(t, err, "expected error converting container stats with zero cpu cores")
}

func TestDockerStatsToContainerStatsCpuUsage(t *testing.T) {
	// doing this with json makes me sad, but is the easiest way to deal with
	// the inner structs

	// numCores is a global variable in package agent/stats
	// which denotes the number of cpu cores
	numCores = 4
	jsonStat := fmt.Sprintf(`
		{
			"cpu_stats":{
				"cpu_usage":{
					"total_usage":%d
				}
			}
		}`, 100)
	dockerStat := &docker.Stats{}
	json.Unmarshal([]byte(jsonStat), dockerStat)
	containerStats, err := dockerStatsToContainerStats(dockerStat)
	assert.NoError(t, err, "converting container stats failed")
	require.NotNil(t, containerStats, "containerStats should not be nil")
	assert.Equal(t, uint64(2500), containerStats.cpuUsage, "unexpected value for cpuUsage", containerStats.cpuUsage)
}
