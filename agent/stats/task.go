package stats

import (
	"context"
	"fmt"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/nswrapper"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	dockerstats "github.com/docker/docker/api/types"
	"github.com/pkg/errors"
	"math/rand"
	"time"
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

func newStatsTaskContainer(taskARN string, containerPID string,
	resolver resolver.ContainerMetadataResolver) (*StatsTask, error) {

	ctx, cancel := context.WithCancel(context.Background())
	return &StatsTask{
		taskMetadata: &TaskMetadata{
			TaskArn:      taskARN,
			ContainerPID: containerPID,
		},
		ctx:      ctx,
		cancel:   cancel,
		resolver: resolver,
	}, nil

}

func (task *StatsTask) StartStatsCollectionTask() {
	// Create the queue to store utilization data from docker stats
	seelog.Infof("Starting stats collection for task: %s", task.taskMetadata.TaskArn)
	queueSize := int(config.DefaultContainerMetricsPublishInterval.Seconds() * 4)
	task.statsQueue = NewQueue(queueSize)
	task.statsQueue.Reset()
	go task.collect()
}

func (task *StatsTask) StopStatsCollectionTask() {
	task.cancel()
}

func (taskStat *StatsTask) collect() {
	taskArn := taskStat.taskMetadata.TaskArn
	for {
		select {
		case <-taskStat.ctx.Done():
			seelog.Debugf("Stopping stats collection for taskStat %s", taskArn)
			return
		default:
			// Function that is continually collecting stats stream
			err := taskStat.processStatsStream()
			if err != nil {
				seelog.Debugf("Error querying stats for task %s: %v", taskArn, err)
			}
			// We were disconnected from the stats stream.
			// Check if the task is terminal. If it is, stop collecting metrics.
			terminal, err := taskStat.terminal()
			if err != nil {
				// Error determining if the task is terminal. clean-up anyway.
				seelog.Warnf("Error determining if the task %s is terminal, stopping stats collection: %v",
					taskArn, err)
				taskStat.StopStatsCollectionTask()
			} else if terminal {
				seelog.Infof("Task %s is terminal, stopping stats collection", taskArn)
				taskStat.StopStatsCollectionTask()
			}
		}
	}
}

func (taskStat *StatsTask) processStatsStream() error {
	taskArn := taskStat.taskMetadata.TaskArn
	// Getting stats from host
	dockerStats, errC := getTaskStats(taskStat)

	returnError := false
	for {
		select {
		case <-taskStat.ctx.Done():
			return nil
		case err := <-errC:
			seelog.Warnf("Error encountered processing metrics stream from host, this may affect "+
				"cloudwatch metric accuracy: %s", err)
			returnError = true
		case rawStat, ok := <-dockerStats:
			if !ok {
				if returnError {
					return fmt.Errorf("error encountered processing metrics stream from host")
				}
				return nil
			}
			if err := taskStat.statsQueue.Add(rawStat); err != nil {
				seelog.Warnf("Task [%s]: error converting stats: %v", taskArn, err)
			}
		}
	}

}

func getDevicesList(linkList []netlink.Link) []string {
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

// If the device list for task has already been populated, do no populate again as it will stay same for a task
func (taskStat *StatsTask) populateDeviceList(netlinkClient netlinkwrapper.NetLink, nsClient nswrapper.NS) error {
	var err error
	if len(taskStat.taskMetadata.DeviceName) == 0 {
		netNSPath := fmt.Sprintf(ecscni.NetnsFormat, taskStat.taskMetadata.ContainerPID)
		err = nsClient.WithNetNSPath(netNSPath, func(ns.NetNS) error {
			var linkErr error
			linksInTaskNetNS, linkErr := netlinkClient.LinkList()
			deviceNames := getDevicesList(linksInTaskNetNS)
			taskStat.taskMetadata.DeviceName = append(taskStat.taskMetadata.DeviceName, deviceNames...)
			return linkErr
		})
	}
	return err
}

var getTaskStats = func(task *StatsTask) (<-chan *types.StatsJSON, <-chan error) {
	errC := make(chan error)
	statsC := make(chan *dockerstats.StatsJSON)
	ns_agent := nswrapper.NewNS()
	netlinkclient := netlinkwrapper.New()
	time.Sleep(time.Second * time.Duration(rand.Intn(int(config.DefaultPollingMetricsWaitDuration))))

	err := task.populateDeviceList(netlinkclient, ns_agent)
	if err != nil {
		errC <- err
		return statsC, errC
	}

	networkStats := make(map[string]dockerstats.NetworkStats)
	for _, device := range task.taskMetadata.DeviceName {
		var link netlink.Link
		err := ns_agent.WithNetNSPath(fmt.Sprintf(ecscni.NetnsFormat, task.taskMetadata.ContainerPID),
			func(ns.NetNS) error {
				var linkErr error
				if link, linkErr = netlinkclient.LinkByName(device); linkErr != nil {
					return errors.Wrapf(linkErr, "failed to collect links in task net namespace of device %v", device)
				}
				return nil
			})
		if err != nil {
			errC <- err
			return statsC, errC
		}

		netLinkStats := link.Attrs().Statistics
		networkStats[link.Attrs().Name] = dockerstats.NetworkStats{
			RxBytes:   netLinkStats.RxBytes,
			RxPackets: netLinkStats.RxPackets,
			RxErrors:  netLinkStats.RxErrors,
			RxDropped: netLinkStats.RxDropped,
			TxBytes:   netLinkStats.TxBytes,
			TxPackets: netLinkStats.TxPackets,
			TxErrors:  netLinkStats.TxErrors,
			TxDropped: netLinkStats.TxDropped,
		}
	}
	dockerStats := &types.StatsJSON{
		Networks: networkStats,
	}
	statsC <- dockerStats
	return statsC, errC
}

func (task *StatsTask) terminal() (bool, error) {
	resolvedTask, err := task.resolver.ResolveTaskByARN(task.taskMetadata.TaskArn)
	if err != nil {
		return false, err
	}
	return resolvedTask.GetKnownStatus() == apitaskstatus.TaskStopped, nil
}
