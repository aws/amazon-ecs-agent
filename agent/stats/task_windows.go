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
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"

	dockerstats "github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

type StatsTask struct {
	*statsTaskCommon
	interfaceLUID []uint64
	netUtils      networkutils.NetworkUtils
}

func newStatsTaskContainer(taskARN, taskId, containerPID string, numberOfContainers int,
	resolver resolver.ContainerMetadataResolver, publishInterval time.Duration, taskENIs task.TaskENIs) (*StatsTask, error) {

	// Instantiate an instance of network utils.
	// This interface would be used to invoke Windows networking APIs.
	netUtils := networkutils.New()

	devices := make([]string, len(taskENIs))
	ifaceLUID := make([]uint64, len(taskENIs))
	// Find and store the device name along with the interface LUID.
	for index, device := range taskENIs {
		devices[index] = device.LinkName

		interfaceLUID, err := netUtils.ConvertInterfaceAliasToLUID(device.LinkName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialise stats task container")
		}
		ifaceLUID[index] = interfaceLUID
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &StatsTask{
		statsTaskCommon: &statsTaskCommon{
			TaskMetadata: &TaskMetadata{
				TaskArn:          taskARN,
				ContainerPID:     containerPID,
				DeviceName:       devices,
				NumberContainers: numberOfContainers,
			},
			Ctx:                   ctx,
			Cancel:                cancel,
			Resolver:              resolver,
			metricPublishInterval: publishInterval,
		},
		interfaceLUID: ifaceLUID,
		netUtils:      netUtils,
	}, nil
}

// retrieveNetworkStatistics retrieves the network statistics for the task devices by querying
// the Windows networking APIs.
func (taskStat *StatsTask) retrieveNetworkStatistics() (map[string]dockerstats.NetworkStats, error) {
	if len(taskStat.TaskMetadata.DeviceName) == 0 {
		return nil, errors.Errorf("unable to find any device name associated with the task %s", taskStat.TaskMetadata.TaskArn)
	}

	networkStats := make(map[string]dockerstats.NetworkStats, len(taskStat.TaskMetadata.DeviceName))
	for index, device := range taskStat.TaskMetadata.DeviceName {
		numberOfContainers := taskStat.TaskMetadata.NumberContainers

		// Query the MIB_IF_ROW2 for the given interface LUID which would contain network statistics.
		ifaceLUID := taskStat.interfaceLUID[index]
		ifRow, err := taskStat.netUtils.GetMIBIfEntryFromLUID(ifaceLUID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to retrieve network stats")
		}

		// Parse the retrieved network statistics.
		networkAdaptorStatistics := taskStat.parseNetworkStatsPerContainerFromIfRow(ifRow, numberOfContainers)
		networkStats[device] = *networkAdaptorStatistics
	}

	return networkStats, nil
}

// parseNetworkStatsPerContainerFromIfRow parses the network statistics from MibIfRow2 row into
// docker network stats. The stats are averaged over all the task containers.
func (taskStat *StatsTask) parseNetworkStatsPerContainerFromIfRow(
	iface *networkutils.MibIfRow2,
	numberOfContainers int,
) *dockerstats.NetworkStats {

	stats := &dockerstats.NetworkStats{}
	stats.RxBytes = iface.InOctets / uint64(numberOfContainers)
	stats.RxPackets = (iface.InNUcastPkts + iface.InUcastPkts) / uint64(numberOfContainers)
	stats.RxErrors = iface.InErrors / uint64(numberOfContainers)
	stats.RxDropped = iface.InDiscards / uint64(numberOfContainers)
	stats.TxBytes = iface.OutOctets / uint64(numberOfContainers)
	stats.TxPackets = (iface.OutNUcastPkts + iface.OutUcastPkts) / uint64(numberOfContainers)
	stats.TxErrors = iface.OutErrors / uint64(numberOfContainers)
	stats.TxDropped = iface.OutDiscards / uint64(numberOfContainers)

	return stats
}
