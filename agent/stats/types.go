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

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
)

// ContainerStats encapsulates the raw CPU and memory utilization from cgroup fs.
type ContainerStats struct {
	cpuUsage          uint64
	memoryUsage       uint64
	storageReadBytes  uint64
	storageWriteBytes uint64
	restartCount      *int64
	networkStats      *NetworkStats
	timestamp         time.Time
}

// NonDockerContainerStats contains stats for a container that are not gotten from docker.
// These are amended to the docker stats and added to the stats queue if they are
// available.
type NonDockerContainerStats struct {
	restartCount *int64
}

// NetworkStats contains the network stats information for a container
type NetworkStats struct {
	RxBytes          uint64  `json:"rxBytes"`
	RxDropped        uint64  `json:"rxDropped"`
	RxErrors         uint64  `json:"rxErrors"`
	RxPackets        uint64  `json:"rxPackets"`
	TxBytes          uint64  `json:"txBytes"`
	TxDropped        uint64  `json:"txDropped"`
	TxErrors         uint64  `json:"txErrors"`
	TxPackets        uint64  `json:"txPackets"`
	RxBytesPerSecond float32 `json:"rxBytesPerSecond"`
	TxBytesPerSecond float32 `json:"txBytesPerSecond"`
}

// UsageStats abstracts the format in which the queue stores data.
type UsageStats struct {
	CPUUsagePerc      float32       `json:"cpuUsagePerc"`
	MemoryUsageInMegs uint32        `json:"memoryUsageInMegs"`
	StorageReadBytes  uint64        `json:"storageReadBytes"`
	StorageWriteBytes uint64        `json:"storageWriteBytes"`
	NetworkStats      *NetworkStats `json:"networkStats"`
	RestartCount      *int64        `json:"restartCount"`
	Timestamp         time.Time     `json:"timestamp"`
	cpuUsage          uint64
	// sent indicates if the stat has been sent to TACS already.
	sent bool
}

// ContainerMetadata contains meta-data information for a container.
type ContainerMetadata struct {
	DockerID    string    `json:"-"`
	Name        string    `json:"-"`
	NetworkMode string    `json:"-"`
	StartedAt   time.Time `json:"-"`
}

// TaskMetadata contains meta-data information for a task.
type TaskMetadata struct {
	TaskArn string `json:"-"`
	TaskId  string `json:"-"`
	// ContainerPID is the PID of the pause container in the awsvpc task.
	ContainerPID     string   `json:"-"`
	DeviceName       []string `json:"-"`
	NumberContainers int      `json:"-"`
}

// StatsContainer abstracts methods to gather and aggregate utilization data for a container.
type StatsContainer struct {
	containerMetadata      *ContainerMetadata
	ctx                    context.Context
	cancel                 context.CancelFunc
	client                 dockerapi.DockerClient
	statsQueue             *Queue
	resolver               resolver.ContainerMetadataResolver
	config                 *config.Config
	restartAggregationData apicontainer.ContainerRestartAggregationDataForStats
	dataClient             data.Client
}

// taskDefinition encapsulates family and version strings for a task definition
type taskDefinition struct {
	family  string
	version string
}
