//go:build linux && unit
// +build linux,unit

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
	"errors"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_netlink "github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	mock_nswrapper "github.com/aws/amazon-ecs-agent/agent/utils/nswrapper/mocks"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestTaskStatsCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)
	mockNS := mock_nswrapper.NewMockNS(ctrl)
	mockNetLink := mock_netlink.NewMockNetLink(ctrl)

	containerPID := "23"
	taskId := "task1"
	ctx, cancel := context.WithCancel(context.TODO())
	numberOfContainers := 2
	taskStats := &StatsTask{
		statsTaskCommon: &statsTaskCommon{
			TaskMetadata: &TaskMetadata{
				TaskArn:          taskId,
				ContainerPID:     containerPID,
				DeviceName:       []string{"device1", "device2"},
				NumberContainers: numberOfContainers,
			},
			Ctx:                   ctx,
			Cancel:                cancel,
			Resolver:              resolver,
			metricPublishInterval: time.Second,
		},
		netlinkinterface:   mockNetLink,
		nswrapperinterface: mockNS,
	}

	testTask := &apitask.Task{
		Containers: []*apicontainer.Container{
			{Name: "c1"},
			{Name: "c2", Type: apicontainer.ContainerCNIPause},
		},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
	}
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()
	mockNS.EXPECT().WithNetNSPath(gomock.Any(),
		gomock.Any()).Do(func(nsPath interface{}, toRun func(n ns.NetNS) error) error {
		return toRun(nil)
	}).AnyTimes()
	mockNetLink.EXPECT().LinkByName(gomock.Any()).Return(&netlink.Device{
		LinkAttrs: netlink.LinkAttrs{
			Name: "name",
			Statistics: &netlink.LinkStatistics{
				RxPackets: uint64(2),
				RxBytes:   uint64(50),
			},
		},
	}, nil).AnyTimes()

	taskStats.StartStatsCollection()
	time.Sleep(checkPointSleep)
	taskStats.StopStatsCollection()

	networkStatsSet, err := taskStats.StatsQueue.GetNetworkStatsSet()

	assert.NoError(t, err)
	assert.NotNil(t, networkStatsSet)
	rxSum := (*networkStatsSet.RxBytes.SampleCount * (int64(50))) / int64(numberOfContainers)
	assert.EqualValues(t, rxSum, *networkStatsSet.RxBytes.Sum)
}

func TestTaskStatsCollectionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)
	mockNS := mock_nswrapper.NewMockNS(ctrl)
	mockNetLink := mock_netlink.NewMockNetLink(ctrl)

	containerPID := "23"
	taskId := "task1"
	ctx, cancel := context.WithCancel(context.TODO())

	taskStats := &StatsTask{
		statsTaskCommon: &statsTaskCommon{
			TaskMetadata: &TaskMetadata{
				TaskArn:          taskId,
				ContainerPID:     containerPID,
				DeviceName:       []string{"device1", "device2"},
				NumberContainers: 2,
			},
			Ctx:                   ctx,
			Cancel:                cancel,
			Resolver:              resolver,
			metricPublishInterval: time.Second,
		},
		netlinkinterface:   mockNetLink,
		nswrapperinterface: mockNS,
	}

	testTask := &apitask.Task{
		Containers: []*apicontainer.Container{
			{Name: "c1"},
			{Name: "c2", Type: apicontainer.ContainerCNIPause},
		},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
	}
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()
	err := errors.New("emit macho dwarf: elf header corrupted")

	mockNS.EXPECT().WithNetNSPath(gomock.Any(), gomock.Any()).Return(err)
	mockNS.EXPECT().WithNetNSPath(gomock.Any(),
		gomock.Any()).Do(func(nsPath interface{}, toRun func(n ns.NetNS) error) error {
		return toRun(nil)
	}).AnyTimes()
	mockNetLink.EXPECT().LinkByName(gomock.Any()).Return(&netlink.Device{
		LinkAttrs: netlink.LinkAttrs{
			Name: "name",
			Statistics: &netlink.LinkStatistics{
				RxPackets: uint64(2),
				RxBytes:   uint64(50),
			},
		},
	}, nil).AnyTimes()

	taskStats.StartStatsCollection()
	time.Sleep(checkPointSleep)
	taskStats.StopStatsCollection()

	networkStatsSet, err := taskStats.StatsQueue.GetNetworkStatsSet()
	assert.NoError(t, err)
	assert.EqualValues(t, 50, *networkStatsSet.RxBytes.Sum)
	assert.EqualValues(t, 2, *networkStatsSet.RxPackets.Sum)
	assert.EqualValues(t, 2, *networkStatsSet.RxPackets.SampleCount)
	assert.EqualValues(t, 2, *networkStatsSet.TxPackets.SampleCount)
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
