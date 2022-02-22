//go:build linux

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
	"fmt"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/aws/amazon-ecs-agent/agent/utils/nswrapper"

	"github.com/containernetworking/plugins/pkg/ns"
	dockerstats "github.com/docker/docker/api/types"
	netlinklib "github.com/vishvananda/netlink"
)

const (
	// linkTypeDevice defines the string that's expected to be the output of
	// netlink.Link.Type() method for netlink.Device type.
	linkTypeDevice = "device"
	linkTypeVlan   = "vlan"
	// encapTypeLoopback defines the string that's set for the link.Attrs.EncapType
	// field for localhost devices. The EncapType field defines the link
	// encapsulation method. For localhost, it's set to "loopback".
	encapTypeLoopback = "loopback"
)

type StatsTask struct {
	*statsTaskCommon
	nswrapperinterface nswrapper.NS
	netlinkinterface   netlinkwrapper.NetLink
}

func newStatsTaskContainer(taskARN, taskId, containerPID string, numberOfContainers int,
	resolver resolver.ContainerMetadataResolver, publishInterval time.Duration, _ task.TaskENIs) (*StatsTask, error) {
	nsAgent := nswrapper.NewNS()
	netlinkclient := netlinkwrapper.New()

	ctx, cancel := context.WithCancel(context.Background())
	return &StatsTask{
		statsTaskCommon: &statsTaskCommon{
			TaskMetadata: &TaskMetadata{
				TaskArn:          taskARN,
				TaskId:           taskId,
				ContainerPID:     containerPID,
				NumberContainers: numberOfContainers,
			},
			Ctx:                   ctx,
			Cancel:                cancel,
			Resolver:              resolver,
			metricPublishInterval: publishInterval,
		},
		netlinkinterface:   netlinkclient,
		nswrapperinterface: nsAgent,
	}, nil
}

func getDevicesList(linkList []netlinklib.Link) []string {
	var deviceNames []string
	for _, link := range linkList {
		if link.Type() != linkTypeDevice && link.Type() != linkTypeVlan {
			// We only care about netlink.Device/netlink.Vlan types. Ignore other link types.
			continue
		}
		if link.Attrs().EncapType == encapTypeLoopback {
			// Ignore localhost
			continue
		}
		deviceNames = append(deviceNames, link.Attrs().Name)
	}
	return deviceNames
}

func (taskStat *StatsTask) populateNIDeviceList(containerPID string) ([]string, error) {
	var err error
	var deviceList []string
	netNSPath := fmt.Sprintf(ecscni.NetnsFormat, containerPID)
	err = taskStat.nswrapperinterface.WithNetNSPath(netNSPath, func(ns.NetNS) error {
		linksInTaskNetNS, linkErr := taskStat.netlinkinterface.LinkList()
		deviceNames := getDevicesList(linksInTaskNetNS)
		deviceList = append(deviceList, deviceNames...)
		return linkErr
	})
	return deviceList, err
}

func linkStatsToDockerStats(netLinkStats *netlinklib.LinkStatistics, numberOfContainers uint64) dockerstats.NetworkStats {
	networkStats := dockerstats.NetworkStats{
		RxBytes:   netLinkStats.RxBytes / numberOfContainers,
		RxPackets: netLinkStats.RxPackets / numberOfContainers,
		RxErrors:  netLinkStats.RxErrors / numberOfContainers,
		RxDropped: netLinkStats.RxDropped / numberOfContainers,
		TxBytes:   netLinkStats.TxBytes / numberOfContainers,
		TxPackets: netLinkStats.TxPackets / numberOfContainers,
		TxErrors:  netLinkStats.TxErrors / numberOfContainers,
		TxDropped: netLinkStats.TxDropped / numberOfContainers,
	}
	return networkStats
}

func (taskStat *StatsTask) retrieveNetworkStatistics() (map[string]dockerstats.NetworkStats, error) {
	if len(taskStat.TaskMetadata.DeviceName) == 0 {
		var err error
		taskStat.TaskMetadata.DeviceName, err = taskStat.populateNIDeviceList(taskStat.TaskMetadata.ContainerPID)
		if err != nil {
			return nil, err
		}
	}

	networkStats := make(map[string]dockerstats.NetworkStats, len(taskStat.TaskMetadata.DeviceName))
	for _, device := range taskStat.TaskMetadata.DeviceName {
		var link netlinklib.Link
		err := taskStat.nswrapperinterface.WithNetNSPath(fmt.Sprintf(ecscni.NetnsFormat,
			taskStat.TaskMetadata.ContainerPID),
			func(ns.NetNS) error {
				var linkErr error
				if link, linkErr = taskStat.netlinkinterface.LinkByName(device); linkErr != nil {
					return linkErr
				}
				return nil
			})

		if err != nil {
			return nil, err
		}
		netLinkStats := link.Attrs().Statistics
		networkStats[link.Attrs().Name] = linkStatsToDockerStats(netLinkStats,
			uint64(taskStat.TaskMetadata.NumberContainers))
	}

	return networkStats, nil
}
