//+build unit

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
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"testing"
	"time"
	"errors"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/vishvananda/netlink"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"

	"github.com/golang/mock/gomock"
)

var statsDataTask = []*StatTestData{
	{parseNanoTime("2015-02-12T21:22:05.131117533Z"), 22400432, 1839104},
	{parseNanoTime("2015-02-12T21:22:05.232291187Z"), 116499979, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.333776335Z"), 248503503, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.434753595Z"), 372167097, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.535746779Z"), 502862518, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.638709495Z"), 638485801, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.739985398Z"), 780707806, 3649536},
	{parseNanoTime("2015-02-12T21:22:05.840941705Z"), 911624529, 3649536},
}

var statsNetworkData1 = map[string]types.NetworkStats{
	"eth0": types.NetworkStats{
		RxBytes:   uint64(100),
		RxPackets: uint64(10),
		RxErrors:  uint64(0),
		RxDropped: uint64(0),
		TxBytes:   uint64(0),
		TxPackets: uint64(0),
		TxErrors:  uint64(0),
		TxDropped: uint64(0),
	},
	"eth1": types.NetworkStats{
		RxBytes:   uint64(50),
		RxPackets: uint64(5),
		RxErrors:  uint64(0),
		RxDropped: uint64(0),
		TxBytes:   uint64(0),
		TxPackets: uint64(0),
		TxErrors:  uint64(0),
		TxDropped: uint64(0),
	},
}

var statsNetworkData2 = map[string]types.NetworkStats{
	"eth0": types.NetworkStats{
		RxBytes:   uint64(99),
		RxPackets: uint64(9),
		RxErrors:  uint64(0),
		RxDropped: uint64(0),
		TxBytes:   uint64(0),
		TxPackets: uint64(0),
		TxErrors:  uint64(0),
		TxDropped: uint64(0),
	},
	"eth1": types.NetworkStats{
		RxBytes:   uint64(49),
		RxPackets: uint64(4),
		RxErrors:  uint64(0),
		RxDropped: uint64(0),
		TxBytes:   uint64(0),
		TxPackets: uint64(0),
		TxErrors:  uint64(0),
		TxDropped: uint64(0),
	},
}

var statsNetworkData3 = map[string]types.NetworkStats{
	"eth0": types.NetworkStats{
		RxBytes:   uint64(98),
		RxPackets: uint64(8),
		RxErrors:  uint64(0),
		RxDropped: uint64(0),
		TxBytes:   uint64(0),
		TxPackets: uint64(0),
		TxErrors:  uint64(0),
		TxDropped: uint64(0),
	},
	"eth1": types.NetworkStats{
		RxBytes:   uint64(48),
		RxPackets: uint64(5),
		RxErrors:  uint64(0),
		RxDropped: uint64(0),
		TxBytes:   uint64(0),
		TxPackets: uint64(0),
		TxErrors:  uint64(0),
		TxDropped: uint64(0),
	},
}

var statsNetworkData = []map[string]types.NetworkStats{
	statsNetworkData1, statsNetworkData2, statsNetworkData3,
}

func TestTaskStatsCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)

	taskId := "task1"
	ctx, cancel := context.WithCancel(context.TODO())
	taskStats := &StatsTask{
		taskMetadata: &TaskMetadata{
			TaskArn:    taskId,
			DeviceName: []string{"device1"},
		},
		ctx:      ctx,
		cancel:   cancel,
		resolver: resolver,
	}
	testTask := &apitask.Task{Containers: []*apicontainer.Container{{Name: "c1",},},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,}
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()

	statChan := make(chan *types.StatsJSON)
	errC := make(chan error)

	getTaskStats = func(task *StatsTask) (<-chan *types.StatsJSON, <-chan error) {
		return statChan, errC
	}

	taskStats.StartStatsCollectionTask()
	time.Sleep(5 * time.Second)
	go func() {
		for index, networkDatum := range statsNetworkData {
			dockerStat := &types.StatsJSON{}
			dockerStat.Read = statsDataTask[index].timestamp
			dockerStat.Networks = networkDatum
			statChan <- dockerStat
		}
	}()
	time.Sleep(5 * time.Second)
	taskStats.StopStatsCollectionTask()
	networlStatsSet, err := taskStats.statsQueue.GetNetworkStatsSet()
	if err != nil {
		t.Fatal("Error getting cpu stats set:", err)
	}
	assert.EqualValues(t, 444, *networlStatsSet.RxBytes.Sum)
	assert.EqualValues(t, 41, *networlStatsSet.RxPackets.Sum)
	assert.EqualValues(t, 3, *networlStatsSet.RxPackets.SampleCount)
	assert.EqualValues(t, 3, *networlStatsSet.TxPackets.SampleCount)
}

func TestTaskStatsCollectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)

	taskId := "task1"
	testTask := &apitask.Task{Containers: []*apicontainer.Container{{Name: "c1",},},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,}

	ctx, cancel := context.WithCancel(context.TODO())
	taskStats := &StatsTask{
		taskMetadata: &TaskMetadata{
			TaskArn:    taskId,
			DeviceName: []string{"device1"},
		},
		ctx:      ctx,
		cancel:   cancel,
		resolver: resolver,
	}
	statChan := make(chan *types.StatsJSON)
	errC := make(chan error)
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()

	getTaskStats = func(task *StatsTask) (<-chan *types.StatsJSON, <-chan error) {
		return statChan, errC
	}

	taskStats.StartStatsCollectionTask()
	time.Sleep(5 * time.Second)
	go func() {
		for index, networkDatum := range statsNetworkData {
			dockerStat := &types.StatsJSON{}
			dockerStat.Read = statsDataTask[index].timestamp
			dockerStat.Networks = networkDatum
			statChan <- dockerStat
		}
		err := errors.New("emit macho dwarf: elf header corrupted")
		errC <- err
	}()
	time.Sleep(5 * time.Second)
	taskStats.StopStatsCollectionTask()
	networlStatsSet, err := taskStats.statsQueue.GetNetworkStatsSet()
	if err != nil {
		t.Fatal("Error getting cpu stats set:", err)
	}
	assert.EqualValues(t, 444, *networlStatsSet.RxBytes.Sum)
	assert.EqualValues(t, 41, *networlStatsSet.RxPackets.Sum)
	assert.EqualValues(t, 3, *networlStatsSet.RxPackets.SampleCount)
	assert.EqualValues(t, 3, *networlStatsSet.TxPackets.SampleCount)
}

func TestGetDeviceList(t *testing.T) {

	link1 := &netlink.GenericLink{
		LinkType: linkTypeDevice,
		LinkAttrs: netlink.LinkAttrs{
			Name: "link1device",
		},
	}
	link2 := &netlink.GenericLink{
		LinkType: linkTypeVlan,
		LinkAttrs: netlink.LinkAttrs{
			Name: "link2device",
		},
	}
	link3 := &netlink.GenericLink{
		LinkType: "randomLinkType",
		LinkAttrs: netlink.LinkAttrs{
			Name: "link3device",
		},
	}
	link4 := &netlink.GenericLink{
		LinkAttrs: netlink.LinkAttrs{
			EncapType: encapTypeLoopback,
			Name:      "link4device",
		},
		LinkType: linkTypeVlan,
	}
	linkList := []netlink.Link{link1, link2, link3, link4}

	deviceNames := getDevicesList(linkList)

	assert.ElementsMatch(t, []string{"link1device", "link2device"}, deviceNames)
}
