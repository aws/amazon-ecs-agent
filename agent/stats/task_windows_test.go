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
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"

	dockerstats "github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const (
	networkAdapterStatisticsResult = `ifAlias                  : Ethernet 3
InterfaceAlias           : Ethernet 3
InterfaceDescription     : Amazon Elastic Network Adapter #2
Name                     : Ethernet 3
Source                   : 2
OutboundDiscardedPackets : 30
OutboundPacketErrors     : 20
ReceivedBroadcastBytes   : 247548
ReceivedBroadcastPackets : 5894
ReceivedBytes            : 249578
ReceivedDiscardedPackets : 10
ReceivedMulticastBytes   : 0
ReceivedMulticastPackets : 0
ReceivedPacketErrors     : 4
ReceivedUnicastBytes     : 2030
ReceivedUnicastPackets   : 8
SentBroadcastBytes       : 2858
SentBroadcastPackets     : 26
SentBytes                : 256478
SentMulticastBytes       : 5995
SentMulticastPackets     : 65
SentUnicastBytes         : 247624
SentUnicastPackets       : 5895
SupportedStatistics      : 4163583`
)

var expectedNetworkStats = dockerstats.NetworkStats{
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

// Supporting methods for network stats test.
func fakeExecCommandForNetworkStats(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestNetworkStatsProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// TestNetworkStatsProcess is invoked from fakeExecCommand to return the network stats.
func TestNetworkStatsProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintf(os.Stdout, networkAdapterStatisticsResult)
	os.Exit(0)
}

func TestTaskStatsCollection(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.TODO())
	resolver := mock_resolver.NewMockContainerMetadataResolver(ctrl)
	execCommand = fakeExecCommandForNetworkStats

	containerPID := "23"
	taskId := "task1"
	numberOfContainers := 2

	taskStats := &StatsTask{
		statsTaskCommon: &statsTaskCommon{
			TaskMetadata: &TaskMetadata{
				TaskArn:          taskId,
				ContainerPID:     containerPID,
				DeviceName:       []string{"Ethernet 3"},
				NumberContainers: numberOfContainers,
			},
			Ctx:                   ctx,
			Cancel:                cancel,
			Resolver:              resolver,
			metricPublishInterval: time.Second,
		},
	}

	testTask := &apitask.Task{
		Containers: []*apicontainer.Container{
			{Name: "c1"},
			{Name: "c2", Type: apicontainer.ContainerCNIPause},
		},
		KnownStatusUnsafe: apitaskstatus.TaskRunning,
	}
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(testTask, nil).AnyTimes()

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
