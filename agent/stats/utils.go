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
	"math"
	"runtime"

	"github.com/docker/docker/api/types"
)

var numCores = uint64(runtime.NumCPU())

// nan32 returns a 32bit NaN.
func nan32() float32 {
	return (float32)(math.NaN())
}

func getNetworkStats(dockerStats *types.StatsJSON) *NetworkStats {
	if dockerStats.Networks == nil {
		return nil
	}
	networkStats := &NetworkStats{}
	for _, netStats := range dockerStats.Networks {
		networkStats.RxBytes += netStats.RxBytes
		networkStats.RxDropped += netStats.RxDropped
		networkStats.RxErrors += netStats.RxErrors
		networkStats.RxPackets += netStats.RxPackets

		networkStats.TxBytes += netStats.TxBytes
		networkStats.TxDropped += netStats.TxDropped
		networkStats.TxErrors += netStats.TxErrors
		networkStats.TxPackets += netStats.TxPackets
	}
	networkStats.RxBytesPerSecond = float32(nan32())
	networkStats.TxBytesPerSecond = float32(nan32())
	return networkStats
}
