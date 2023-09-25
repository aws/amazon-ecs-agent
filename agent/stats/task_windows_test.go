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
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/eni/networkutils"
	mock_networkutils "github.com/aws/amazon-ecs-agent/agent/eni/networkutils/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"

	dockerstats "github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	deviceName        = "Ethernet 3"
	ifaceLUID  uint64 = 1689399649632256
)

// Result from GetIfEntry2Ex Win32 API call.
var ifRowResult = &networkutils.MibIfRow2{
	InterfaceLUID: ifaceLUID,
	InOctets:      249578,
	InUcastPkts:   8,
	InNUcastPkts:  5894,
	InErrors:      4,
	InDiscards:    10,
	OutOctets:     256478,
	OutUcastPkts:  5895,
	OutNUcastPkts: 91,
	OutErrors:     20,
	OutDiscards:   30,
}

// Expected output from the stats collection module.
var expectedNetworkStats = &dockerstats.NetworkStats{
	RxBytes:    249578,
	RxPackets:  5902,
	RxErrors:   4,
	RxDropped:  10,
	TxBytes:    256478,
	TxPackets:  5986,
	TxErrors:   20,
	TxDropped:  30,
	EndpointID: "",
	InstanceID: "",
}

// TestTaskStatsCollection tests the network statistics collection.
func TestTaskStatsCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)
	mockNetUtils := mock_networkutils.NewMockNetworkUtils(ctrl)

	containerPID := "23"
	taskId := "task1"
	numberOfContainers := 2

	taskStats := &StatsTask{
		statsTaskCommon: &statsTaskCommon{
			TaskMetadata: &TaskMetadata{
				TaskArn:          taskId,
				ContainerPID:     containerPID,
				DeviceName:       []string{deviceName},
				NumberContainers: numberOfContainers,
			},
			Ctx:                   ctx,
			Cancel:                cancel,
			Resolver:              resolver,
			metricPublishInterval: time.Second,
		},
		netUtils:      mockNetUtils,
		interfaceLUID: []uint64{ifaceLUID},
	}

	testTask := &apitask.Task{
		Containers: []*apicontainer.Container{
			{Name: "c1"},
			{Name: "c2", Type: apicontainer.ContainerCNIPause},
		},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
	}
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()
	mockNetUtils.EXPECT().GetMIBIfEntryFromLUID(ifaceLUID).Return(ifRowResult, nil).AnyTimes()

	taskStats.StartStatsCollection()
	time.Sleep(checkPointSleep)
	taskStats.StopStatsCollection()

	networkStatsSet, err := taskStats.StatsQueue.GetNetworkStatsSet()
	assert.NoError(t, err)
	assert.NotNil(t, networkStatsSet)

	rxBytesSum := (*networkStatsSet.RxBytes.SampleCount * (int64(expectedNetworkStats.RxBytes))) / int64(numberOfContainers)
	assert.Equal(t, rxBytesSum, *networkStatsSet.RxBytes.Sum)
	txBytesSum := (*networkStatsSet.TxBytes.SampleCount * (int64(expectedNetworkStats.TxBytes))) / int64(numberOfContainers)
	assert.Equal(t, txBytesSum, *networkStatsSet.TxBytes.Sum)
	rxPacketsSum := (*networkStatsSet.RxPackets.SampleCount * (int64(expectedNetworkStats.RxPackets))) / int64(numberOfContainers)
	assert.Equal(t, rxPacketsSum, *networkStatsSet.RxPackets.Sum)
	txPacketsSum := (*networkStatsSet.TxPackets.SampleCount * (int64(expectedNetworkStats.TxPackets))) / int64(numberOfContainers)
	assert.Equal(t, txPacketsSum, *networkStatsSet.TxPackets.Sum)
	rxErrorSum := (*networkStatsSet.RxErrors.SampleCount * (int64(expectedNetworkStats.RxErrors))) / int64(numberOfContainers)
	assert.Equal(t, rxErrorSum, *networkStatsSet.RxErrors.Sum)
	txErrorSum := (*networkStatsSet.TxErrors.SampleCount * (int64(expectedNetworkStats.TxErrors))) / int64(numberOfContainers)
	assert.Equal(t, txErrorSum, *networkStatsSet.TxErrors.Sum)
	rxDroppedSum := (*networkStatsSet.RxDropped.SampleCount * (int64(expectedNetworkStats.RxDropped))) / int64(numberOfContainers)
	assert.Equal(t, rxDroppedSum, *networkStatsSet.RxDropped.Sum)
	txDroppedSum := (*networkStatsSet.TxDropped.SampleCount * (int64(expectedNetworkStats.TxDropped))) / int64(numberOfContainers)
	assert.Equal(t, txDroppedSum, *networkStatsSet.TxDropped.Sum)
}
