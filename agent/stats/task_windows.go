//go:build windows

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
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"

	dockerstats "github.com/docker/docker/api/types"
	"github.com/pkg/errors"
)

const (
	receivedBroadcastPackets = "ReceivedBroadcastPackets"
	receivedMulticastPackets = "ReceivedMulticastPackets"
	receivedUnicastPackets   = "ReceivedUnicastPackets"
	sentBroadcastPackets     = "SentBroadcastPackets"
	sentMulticastPackets     = "SentMulticastPackets"
	sentUnicastPackets       = "SentUnicastPackets"
	receivedBytes            = "ReceivedBytes"
	receivedPacketErrors     = "ReceivedPacketErrors"
	receivedDiscardedPackets = "ReceivedDiscardedPackets"
	sentBytes                = "SentBytes"
	outboundPacketErrors     = "OutboundPacketErrors"
	outboundDiscardedPackets = "OutboundDiscardedPackets"
)

var (
	// Making it visible for unit testing
	execCommand = exec.Command
	// Fields to be extracted from the stats returned by cmdlet.
	networkStatKeys = []string{
		receivedBroadcastPackets,
		receivedMulticastPackets,
		receivedUnicastPackets,
		sentBroadcastPackets,
		sentMulticastPackets,
		sentUnicastPackets,
		receivedBytes,
		receivedPacketErrors,
		receivedDiscardedPackets,
		sentBytes,
		outboundDiscardedPackets,
		outboundPacketErrors,
	}
)

type StatsTask struct {
	*statsTaskCommon
}

func newStatsTaskContainer(taskARN, taskId, containerPID string, numberOfContainers int,
	resolver resolver.ContainerMetadataResolver, publishInterval time.Duration, taskENIs task.TaskENIs) (*StatsTask, error) {
	ctx, cancel := context.WithCancel(context.Background())

	devices := make([]string, len(taskENIs))
	for index, device := range taskENIs {
		devices[index] = device.LinkName
	}

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
	}, nil
}

func (taskStat *StatsTask) retrieveNetworkStatistics() (map[string]dockerstats.NetworkStats, error) {
	if len(taskStat.TaskMetadata.DeviceName) == 0 {
		return nil, errors.Errorf("unable to find any device name associated with the task %s", taskStat.TaskMetadata.TaskArn)
	}

	networkStats := make(map[string]dockerstats.NetworkStats, len(taskStat.TaskMetadata.DeviceName))
	for _, device := range taskStat.TaskMetadata.DeviceName {
		networkAdaptorStatistics, err := taskStat.getNetworkAdaptorStatistics(device)
		if err != nil {
			return nil, err
		}
		networkStats[device] = *networkAdaptorStatistics
	}

	return networkStats, nil
}

// getNetworkAdaptorStatistics returns the network statistics per container for the given network interface.
func (taskStat *StatsTask) getNetworkAdaptorStatistics(device string) (*dockerstats.NetworkStats, error) {
	// Ref: https://docs.microsoft.com/en-us/powershell/module/netadapter/get-netadapterstatistics?view=windowsserver2019-ps
	// The Get-NetAdapterStatistics cmdlet gets networking statistics from a network adapter.
	// The statistics include broadcast, multicast, discards, and errors.
	cmd := "Get-NetAdapterStatistics -Name \"" + device + "\" | Format-List -Property *"
	out, err := execCommand("powershell", "-Command", cmd).CombinedOutput()

	if err != nil {
		return nil, errors.Wrapf(err, "failed to run Get-NetAdapterStatistics for %s", device)
	}
	str := string(out)

	// Extract rawStats from the cmdlet output.
	lines := strings.Split(str, "\n")
	rawStats := make(map[string]string)
	for _, line := range lines {
		// populate all the network metrics in a map
		kv := strings.Split(line, ":")
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		rawStats[key] = value
	}

	// Parse the required fields from the generated map.
	parsedStats := make(map[string]uint64)
	for _, key := range networkStatKeys {
		value, err := taskStat.getMapValue(rawStats, key)
		if err != nil {
			return nil, err
		}
		parsedStats[key] = value
	}

	numberOfContainers := uint64(taskStat.TaskMetadata.NumberContainers)

	return &dockerstats.NetworkStats{
		RxBytes:   parsedStats[receivedBytes] / numberOfContainers,
		RxPackets: (parsedStats[receivedBroadcastPackets] + parsedStats[receivedMulticastPackets] + parsedStats[receivedUnicastPackets]) / numberOfContainers,
		RxErrors:  parsedStats[receivedPacketErrors] / numberOfContainers,
		RxDropped: parsedStats[receivedDiscardedPackets] / numberOfContainers,
		TxBytes:   parsedStats[sentBytes] / numberOfContainers,
		TxPackets: (parsedStats[sentBroadcastPackets] + parsedStats[sentMulticastPackets] + parsedStats[sentUnicastPackets]) / numberOfContainers,
		TxErrors:  parsedStats[outboundPacketErrors] / numberOfContainers,
		TxDropped: parsedStats[outboundDiscardedPackets] / numberOfContainers,
	}, nil
}

// getMapValue retrieves the value of the key from the given map.
func (taskStat *StatsTask) getMapValue(m map[string]string, key string) (uint64, error) {
	v, ok := m[key]
	if !ok {
		return 0, errors.Errorf("failed to find key: %s in output", key)
	}
	val, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, errors.Errorf("failed to parse network stats for %s with value: %s", key, v)
	}
	return val, nil
}
