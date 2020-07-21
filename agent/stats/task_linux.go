// +build linux

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

	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/aws/amazon-ecs-agent/agent/utils/nswrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
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

// StatsTask abstracts methods to gather and aggregate network data for a task. Used only for AWSVPC mode.
type StatsTask struct {
	StatsQueue            *Queue
	TaskMetadata          *TaskMetadata
	Ctx                   context.Context
	Cancel                context.CancelFunc
	Resolver              resolver.ContainerMetadataResolver
	nswrapperinterface    nswrapper.NS
	netlinkinterface      netlinkwrapper.NetLink
	metricPublishInterval time.Duration
}

func newStatsTaskContainer(taskARN string, containerPID string, numberOfContainers int,
	resolver resolver.ContainerMetadataResolver, publishInterval time.Duration) (*StatsTask, error) {
	nsAgent := nswrapper.NewNS()
	netlinkclient := netlinkwrapper.New()

	ctx, cancel := context.WithCancel(context.Background())
	return &StatsTask{
		TaskMetadata: &TaskMetadata{
			TaskArn:          taskARN,
			ContainerPID:     containerPID,
			NumberContainers: numberOfContainers,
		},
		Ctx:                   ctx,
		Cancel:                cancel,
		Resolver:              resolver,
		netlinkinterface:      netlinkclient,
		nswrapperinterface:    nsAgent,
		metricPublishInterval: publishInterval,
	}, nil
}

func (task *StatsTask) StartStatsCollection() {
	queueSize := int(config.DefaultContainerMetricsPublishInterval.Seconds() * 4)
	task.StatsQueue = NewQueue(queueSize)
	task.StatsQueue.Reset()
	go task.collect()
}

func (task *StatsTask) StopStatsCollection() {
	task.Cancel()
}

func (taskStat *StatsTask) collect() {
	taskArn := taskStat.TaskMetadata.TaskArn
	backoff := retry.NewExponentialBackoff(time.Second*1, time.Second*10, 0.5, 2)

	for {
		err := taskStat.processStatsStream()
		select {
		case <-taskStat.Ctx.Done():
			seelog.Debugf("Stopping stats collection for taskStat %s", taskArn)
			return
		default:
			if err != nil {
				d := backoff.Duration()
				time.Sleep(d)
				seelog.Debugf("Error querying stats for task %s: %v", taskArn, err)
			}
			// We were disconnected from the stats stream.
			// Check if the task is terminal. If it is, stop collecting metrics.
			terminal, err := taskStat.terminal()
			if err != nil {
				// Error determining if the task is terminal. clean-up anyway.
				seelog.Warnf("Error determining if the task %s is terminal, stopping stats collection: %v",
					taskArn, err)
				taskStat.StopStatsCollection()
			} else if terminal {
				seelog.Infof("Task %s is terminal, stopping stats collection", taskArn)
				taskStat.StopStatsCollection()
			}
		}
	}
}

func (taskStat *StatsTask) processStatsStream() error {
	taskArn := taskStat.TaskMetadata.TaskArn
	awsvpcNetworkStats, errC := taskStat.getAWSVPCNetworkStats()

	returnError := false
	for {
		select {
		case <-taskStat.Ctx.Done():
			seelog.Info("task context is done")
			return nil
		case err := <-errC:
			seelog.Warnf("Error encountered processing metrics stream from host, this may affect "+
				"cloudwatch metric accuracy: %s", err)
			returnError = true
		case rawStat, ok := <-awsvpcNetworkStats:
			if !ok {
				if returnError {
					return fmt.Errorf("error encountered processing metrics stream from host")
				}
				return nil
			}
			if err := taskStat.StatsQueue.Add(rawStat); err != nil {
				seelog.Warnf("Task [%s]: error converting stats: %v", taskArn, err)
			}
		}

	}
}

func (taskStat *StatsTask) terminal() (bool, error) {
	resolvedTask, err := taskStat.Resolver.ResolveTaskByARN(taskStat.TaskMetadata.TaskArn)
	if err != nil {
		return false, err
	}
	return resolvedTask.GetKnownStatus() == apitaskstatus.TaskStopped, nil
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

func (taskStat *StatsTask) getAWSVPCNetworkStats() (<-chan *types.StatsJSON, <-chan error) {

	errC := make(chan error)
	statsC := make(chan *dockerstats.StatsJSON)
	if taskStat.TaskMetadata.NumberContainers > 0 {
		go func() {
			defer close(statsC)
			statPollTicker := time.NewTicker(taskStat.metricPublishInterval)
			defer statPollTicker.Stop()
			for range statPollTicker.C {
				if len(taskStat.TaskMetadata.DeviceName) == 0 {
					var err error
					taskStat.TaskMetadata.DeviceName, err = taskStat.populateNIDeviceList(taskStat.TaskMetadata.ContainerPID)
					if err != nil {
						errC <- err
						return
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
						errC <- err
						return
					}
					netLinkStats := link.Attrs().Statistics
					networkStats[link.Attrs().Name] = linkStatsToDockerStats(netLinkStats,
						uint64(taskStat.TaskMetadata.NumberContainers))
				}

				dockerStats := &types.StatsJSON{
					Networks: networkStats,
					Stats: types.Stats{
						Read: time.Now(),
					},
				}
				statsC <- dockerStats
			}
		}()
	}

	return statsC, errC
}
