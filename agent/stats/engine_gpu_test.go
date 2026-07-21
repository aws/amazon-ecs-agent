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

//go:build linux && unit
// +build linux,unit

package stats

// GPU metrics emission tests. Exercises GetInstanceMetrics GPU contract:
//
//  1. Reader seam: gpuMetricsReader injected via SetGPUMetricsReader.
//  2. 3-tick cadence: includeGPUMetrics=true every 3rd tick.
//  3. Staleness suppression: unchanged Timestamp suppresses GPU; CPU/memory flow.
//  4. Instance payload: dimensionless InstanceGPULimit + InstanceGPUUsageTotal.
//  5. Container payload: per-device AcceleratedDevice wrapper; unassigned GPUs excluded.

import (
	"context"
	"fmt"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	mock_resolver "github.com/aws/amazon-ecs-agent/agent/stats/resolver/mock"
	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDCGMMetricsReader is a test double. Mutate data.Timestamp between calls
// to simulate fresh vs stale snapshots; reads tracks query count.
type fakeDCGMMetricsReader struct {
	data  *gputypes.GPUMetricsFileData
	reads int
}

func (f *fakeDCGMMetricsReader) GetGPUMetrics() *gputypes.GPUMetricsFileData {
	f.reads++
	return f.data
}

// setupGPUStatsEngine builds an engine watching container "c1" (task "t1",
// bridge mode) with the given GPU IDs. t.Cleanup handles goroutine shutdown.
func setupGPUStatsEngine(t *testing.T, mockCtrl *gomock.Controller, gpuIDs []string) (*DockerStatsEngine, context.CancelFunc) {
	t.Helper()
	resolver := mock_resolver.NewMockContainerMetadataResolver(mockCtrl)
	mockDockerClient := mock_dockerapi.NewMockDockerClient(mockCtrl)
	t1 := &apitask.Task{Arn: "t1", Family: "f1", NetworkMode: "bridge"}
	resolver.EXPECT().ResolveTask("c1").AnyTimes().Return(t1, nil)
	resolver.EXPECT().ResolveContainer(gomock.Any()).AnyTimes().Return(&apicontainer.DockerContainer{
		Container: &apicontainer.Container{
			Name:   "test",
			GPUIDs: gpuIDs,
		},
	}, nil)
	mockDockerClient.EXPECT().Stats(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	resolver.EXPECT().ResolveTaskByARN(gomock.Any()).Return(t1, nil).AnyTimes()

	engine := NewDockerStatsEngine(&cfg, nil, eventStream(t.Name()), nil, nil, nil)
	ctx, cancel := context.WithCancel(context.TODO())
	engine.ctx = ctx
	engine.resolver = resolver
	engine.cluster = defaultCluster
	engine.containerInstanceArn = defaultContainerInstance
	engine.client = mockDockerClient
	engine.addAndStartStatsContainer("c1")
	// Stops the spawned collect() goroutine, whose context the returned
	// cancel cannot reach; mock-free, so safe after mockCtrl.Finish().
	t.Cleanup(engine.removeAll)
	return engine, cancel
}

// feedFakeStats loads two CPU/memory samples into every watched container's
// queue. Call before each GetInstanceMetrics (resetStatsUnsafe drains them).
func feedFakeStats(engine *DockerStatsEngine) {
	containerStats := createFakeContainerStats()
	for _, containers := range engine.tasksToContainers {
		for _, statsContainer := range containers {
			for i := 0; i < 2; i++ {
				statsContainer.statsQueue.add(containerStats[i])
			}
		}
	}
}

// requireInstanceGPUPayload asserts the instance-level GPU contract: exactly
// one dimensionless wrapper carrying InstanceGPULimit and
// InstanceGPUUsageTotal as MetricValueLong with unit Count.
func requireInstanceGPUPayload(t *testing.T, im *ecstcs.InstanceMetrics, wantLimit, wantUsage int64) {
	t.Helper()
	require.NotNil(t, im, "expected InstanceMetrics to be emitted")
	require.Len(t, im.GeneralMetricsPayload, 1, "expected exactly one instance GPU wrapper")
	wrapper := im.GeneralMetricsPayload[0]
	assert.Empty(t, wrapper.Dimensions, "instance GPU wrapper must be dimensionless; the backend stamps instance identity")
	require.Len(t, wrapper.GeneralMetrics, 2)

	var gotLimit, gotUsage *int64
	for _, gm := range wrapper.GeneralMetrics {
		require.NotNil(t, gm.MetricName)
		require.NotNil(t, gm.MetricValueLong, "instance GPU metrics are MetricValueLong")
		require.NotNil(t, gm.Unit)
		assert.Equal(t, "Count", *gm.Unit)
		switch *gm.MetricName {
		case "InstanceGPULimit":
			gotLimit = gm.MetricValueLong
		case "InstanceGPUUsageTotal":
			gotUsage = gm.MetricValueLong
		default:
			t.Errorf("unexpected instance GPU metric name: %s", *gm.MetricName)
		}
	}
	require.NotNil(t, gotLimit, "InstanceGPULimit not found")
	require.NotNil(t, gotUsage, "InstanceGPUUsageTotal not found")
	assert.Equal(t, wantLimit, *gotLimit, "InstanceGPULimit mismatch")
	assert.Equal(t, wantUsage, *gotUsage, "InstanceGPUUsageTotal mismatch")
}

// requireContainerGPUPayload asserts the container-level GPU contract: one
// AcceleratedDevice-dimensioned wrapper per assigned device, exactly the
// wanted devices, in reader order (wantUUIDs must match that order).
func requireContainerGPUPayload(t *testing.T, cm *ecstcs.ContainerMetric, wantUUIDs []string) {
	t.Helper()
	require.NotNil(t, cm)
	require.Len(t, cm.GeneralMetricsPayload, len(wantUUIDs),
		"expected one container GPU wrapper per assigned device")

	var gotUUIDs []string
	for _, wrapper := range cm.GeneralMetricsPayload {
		require.Len(t, wrapper.Dimensions, 1, "container GPU wrapper carries exactly one dimension")
		require.NotNil(t, wrapper.Dimensions[0].Key)
		require.NotNil(t, wrapper.Dimensions[0].Value)
		assert.Equal(t, "AcceleratedDevice", *wrapper.Dimensions[0].Key)
		gotUUIDs = append(gotUUIDs, *wrapper.Dimensions[0].Value)
		require.NotEmpty(t, wrapper.GeneralMetrics, "device wrapper must carry telemetry metrics")
		for _, gm := range wrapper.GeneralMetrics {
			require.NotNil(t, gm.MetricName)
			require.NotNil(t, gm.Unit)
			if *gm.MetricName == "GPUUtilization" {
				assert.NotNil(t, gm.MetricValueDouble, "GPUUtilization is MetricValueDouble")
				assert.Equal(t, "Percent", *gm.Unit)
			}
		}
	}
	assert.Equal(t, wantUUIDs, gotUUIDs, "container GPU wrapper devices mismatch")
}

// containerGPUPayloadDeviceIDs returns each wrapper's AcceleratedDevice
// dimension value, for explicit inclusion/exclusion assertions.
func containerGPUPayloadDeviceIDs(cm *ecstcs.ContainerMetric) []string {
	var ids []string
	for _, wrapper := range cm.GeneralMetricsPayload {
		for _, d := range wrapper.Dimensions {
			if d.Key != nil && *d.Key == "AcceleratedDevice" && d.Value != nil {
				ids = append(ids, *d.Value)
			}
		}
	}
	return ids
}

// requireNoContainerGPUPayload asserts the shape of a suppressed (stale or
// off-cadence) tick: no GPU payload on any container, while CPU/memory
// metrics still flow — GPU suppression must not drop the container itself.
func requireNoContainerGPUPayload(t *testing.T, taskMetrics []*ecstcs.TaskMetric) {
	t.Helper()
	require.NotEmpty(t, taskMetrics)
	for _, tm := range taskMetrics {
		require.NotEmpty(t, tm.ContainerMetrics)
		for _, cm := range tm.ContainerMetrics {
			assert.Empty(t, cm.GeneralMetricsPayload,
				"container %s must carry no GPU payload on a suppressed tick", aws.ToString(cm.ContainerName))
			assert.NotNil(t, cm.CpuStatsSet, "CPU metrics must keep flowing when GPU is suppressed")
			assert.NotNil(t, cm.MemoryStatsSet, "memory metrics must keep flowing when GPU is suppressed")
		}
	}
}

// TestGetInstanceMetricsEmitsInstanceGPUMetrics: an emitting tick with fresh
// data emits both scopes — the instance payload (limit = GPUs on host,
// usage = unique assigned GPU IDs) and a container payload restricted to the
// container's assigned devices.
func TestGetInstanceMetricsEmitsInstanceGPUMetrics(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// One of the host's two GPUs is assigned to the running container.
	engine, cancel := setupGPUStatsEngine(t, mockCtrl, []string{"GPU-1"})
	defer cancel()

	fake := &fakeDCGMMetricsReader{data: &gputypes.GPUMetricsFileData{
		Timestamp: "2026-07-19T00:00:00Z",
		Healthy:   true,
		GPUs: []gputypes.GPUMetric{
			{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
			{GPUUUID: "GPU-2", GPUUtilization: aws.Float64(10.0)},
		},
	}}
	engine.SetGPUMetricsReader(fake)

	feedFakeStats(engine)

	metadata, taskMetrics, instanceMetrics, err := engine.GetInstanceMetrics(false, true)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	require.Len(t, taskMetrics, 1)

	requireInstanceGPUPayload(t, instanceMetrics, 2 /* GPUs on host */, 1 /* unique assigned */)

	// Container scope: only assigned GPU-1 appears; CPU/memory ride along.
	require.Len(t, taskMetrics[0].ContainerMetrics, 1)
	cm := taskMetrics[0].ContainerMetrics[0]
	requireContainerGPUPayload(t, cm, []string{"GPU-1"})
	// Unassigned GPU-2 must not leak in.
	assert.NotContains(t, containerGPUPayloadDeviceIDs(cm), "GPU-2",
		"unassigned host GPU must not leak into the container payload")
	assert.NotNil(t, cm.CpuStatsSet)
	assert.NotNil(t, cm.MemoryStatsSet)

	assert.GreaterOrEqual(t, fake.reads, 1, "engine should have queried the DCGM metrics reader")
}

// TestGetInstanceMetricsSkipsStaleGPUMetrics: an unchanged reader Timestamp
// means stale data — suppressed at both scopes, resuming once the Timestamp
// changes. The stale tick asserts on containers too: checking only
// instanceMetrics would miss leaked container wrappers or a dropped
// container.
func TestGetInstanceMetricsSkipsStaleGPUMetrics(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Assign the GPU so a container payload exists to be suppressed.
	engine, cancel := setupGPUStatsEngine(t, mockCtrl, []string{"GPU-1"})
	defer cancel()

	fake := &fakeDCGMMetricsReader{data: &gputypes.GPUMetricsFileData{
		Timestamp: "2026-07-19T00:00:00Z",
		Healthy:   true,
		GPUs:      []gputypes.GPUMetric{{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(25.0)}},
	}}
	engine.SetGPUMetricsReader(fake)

	// Emitting tick #1: fresh data -> emitted at both scopes.
	feedFakeStats(engine)
	_, taskMetrics, instanceMetrics, err := engine.GetInstanceMetrics(false, true)
	require.NoError(t, err)
	requireInstanceGPUPayload(t, instanceMetrics, 1, 1)
	require.Len(t, taskMetrics, 1)
	require.Len(t, taskMetrics[0].ContainerMetrics, 1)
	requireContainerGPUPayload(t, taskMetrics[0].ContainerMetrics[0], []string{"GPU-1"})

	// Emitting tick #2: same timestamp -> stale. Both scopes suppressed
	// (tick #1's GPU-1 wrapper must not reappear); CPU/memory keep flowing.
	feedFakeStats(engine)
	_, taskMetrics, instanceMetrics, err = engine.GetInstanceMetrics(false, true)
	require.NoError(t, err)
	assert.Nil(t, instanceMetrics, "stale GPU data (unchanged timestamp) must not be re-emitted")
	requireNoContainerGPUPayload(t, taskMetrics)

	// Emitting tick #3: new snapshot -> emission resumes at both scopes.
	fake.data.Timestamp = "2026-07-19T00:01:00Z"
	feedFakeStats(engine)
	_, taskMetrics, instanceMetrics, err = engine.GetInstanceMetrics(false, true)
	require.NoError(t, err)
	requireInstanceGPUPayload(t, instanceMetrics, 1, 1)
	require.Len(t, taskMetrics, 1)
	require.Len(t, taskMetrics[0].ContainerMetrics, 1)
	requireContainerGPUPayload(t, taskMetrics[0].ContainerMetrics[0], []string{"GPU-1"})
}

// TestGPUMetricsEmittedOnlyWhenFlagSet verifies that GPU payloads are
// attached only when includeGPUMetrics is true (set by StartMetricsPublish
// every 3rd tick); CPU/memory metrics flow regardless.
func TestGPUMetricsEmittedOnlyWhenFlagSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	engine, cancel := setupGPUStatsEngine(t, mockCtrl, []string{"GPU-1"})
	defer cancel()

	fake := &fakeDCGMMetricsReader{data: &gputypes.GPUMetricsFileData{
		Timestamp: "2026-07-19T00:00:00Z",
		Healthy:   true,
		GPUs:      []gputypes.GPUMetric{{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(25.0)}},
	}}
	engine.SetGPUMetricsReader(fake)

	for tick := 1; tick <= 2*defaultPublishGPUMetricsTicker; tick++ {
		// Fresh data every round so staleness never interferes.
		fake.data.Timestamp = fmt.Sprintf("2026-07-19T00:00:%02dZ", tick)
		feedFakeStats(engine)

		// Simulate StartMetricsPublish: pass true on every 3rd tick.
		includeGPU := tick%defaultPublishGPUMetricsTicker == 0
		_, taskMetrics, instanceMetrics, err := engine.GetInstanceMetrics(false, includeGPU)
		require.NoError(t, err, "tick %d", tick)

		if includeGPU {
			requireInstanceGPUPayload(t, instanceMetrics, 1, 1)
			require.Len(t, taskMetrics, 1, "tick %d", tick)
			require.Len(t, taskMetrics[0].ContainerMetrics, 1, "tick %d", tick)
			requireContainerGPUPayload(t, taskMetrics[0].ContainerMetrics[0], []string{"GPU-1"})
		} else {
			assert.Nil(t, instanceMetrics, "GPU metrics must not be emitted when includeGPUMetrics=false (tick %d)",
				tick)
			requireNoContainerGPUPayload(t, taskMetrics)
		}
	}
}

// TestGetInstanceMetricsContainerGPUMetricsFiltering pins the container-scope
// filtering: one wrapper per assigned device in reader order; assigned
// devices absent from the snapshot are skipped silently (no empty wrapper).
func TestGetInstanceMetricsContainerGPUMetricsFiltering(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// GPU-9 is assigned but has no reading in the snapshot.
	engine, cancel := setupGPUStatsEngine(t, mockCtrl, []string{"GPU-1", "GPU-3", "GPU-9"})
	defer cancel()

	fake := &fakeDCGMMetricsReader{data: &gputypes.GPUMetricsFileData{
		Timestamp: "2026-07-19T00:00:00Z",
		Healthy:   true,
		GPUs: []gputypes.GPUMetric{
			{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
			{GPUUUID: "GPU-2", GPUUtilization: aws.Float64(10.0)}, // on host, not assigned
			{GPUUUID: "GPU-3", GPUUtilization: aws.Float64(75.0)},
		},
	}}
	engine.SetGPUMetricsReader(fake)

	feedFakeStats(engine)

	_, taskMetrics, instanceMetrics, err := engine.GetInstanceMetrics(false, true)
	require.NoError(t, err)
	require.Len(t, taskMetrics, 1)
	require.Len(t, taskMetrics[0].ContainerMetrics, 1)

	// Container scope: wrappers for GPU-1 and GPU-3 only, in reader order.
	cm := taskMetrics[0].ContainerMetrics[0]
	requireContainerGPUPayload(t, cm, []string{"GPU-1", "GPU-3"})

	// GPU-2 (unassigned) and GPU-9 (no reading) must not appear.
	gotDeviceIDs := containerGPUPayloadDeviceIDs(cm)
	assert.NotContains(t, gotDeviceIDs, "GPU-2",
		"unassigned host GPU must not leak into the container payload")
	assert.NotContains(t, gotDeviceIDs, "GPU-9",
		"assigned device with no reading must not produce a wrapper")

	// Instance scope: limit = the reader's 3 devices; usage = 3 unique
	// assigned IDs (incl. GPU-9 — assignment counting comes from container
	// state, not the snapshot).
	requireInstanceGPUPayload(t, instanceMetrics, 3, 3)
}

// TestGetInstanceMetricsNoContainerGPUPayloadWithoutAssignment: a container
// with no assigned GPUs carries no payload even when the host has GPUs.
func TestGetInstanceMetricsNoContainerGPUPayloadWithoutAssignment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	engine, cancel := setupGPUStatsEngine(t, mockCtrl, nil)
	defer cancel()

	fake := &fakeDCGMMetricsReader{data: &gputypes.GPUMetricsFileData{
		Timestamp: "2026-07-19T00:00:00Z",
		Healthy:   true,
		GPUs:      []gputypes.GPUMetric{{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(25.0)}},
	}}
	engine.SetGPUMetricsReader(fake)

	feedFakeStats(engine)

	_, taskMetrics, instanceMetrics, err := engine.GetInstanceMetrics(false, true)
	require.NoError(t, err)

	// Instance scope still emits: the host has a GPU, none assigned.
	requireInstanceGPUPayload(t, instanceMetrics, 1, 0)
	// No container payload; CPU/memory unaffected.
	requireNoContainerGPUPayload(t, taskMetrics)
}

// TestGetInstanceMetricsConnectionLost verifies GPU emission behavior when
// the DCGM metrics reader reports ConnectionLost. Both subtests are emitting
// ticks — ConnectionLost is toggled to verify suppression vs emission.
func TestGetInstanceMetricsConnectionLost(t *testing.T) {
	testCases := []struct {
		name           string
		connectionLost bool
		expectGPUEmit  bool
	}{
		{
			name:           "connection lost suppresses GPU at both scopes",
			connectionLost: true,
			expectGPUEmit:  false,
		},
		{
			name:           "connection healthy emits GPU normally",
			connectionLost: false,
			expectGPUEmit:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			engine, cancel := setupGPUStatsEngine(t, mockCtrl, []string{"GPU-1"})
			defer cancel()

			fake := &fakeDCGMMetricsReader{data: &gputypes.GPUMetricsFileData{
				Timestamp:      "2026-07-19T00:00:00Z",
				Healthy:        true,
				ConnectionLost: tc.connectionLost,
				GPUs:           []gputypes.GPUMetric{{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)}},
			}}
			engine.SetGPUMetricsReader(fake)

			feedFakeStats(engine)
			_, taskMetrics, instanceMetrics, err := engine.GetInstanceMetrics(false, true)
			require.NoError(t, err)

			if tc.expectGPUEmit {
				requireInstanceGPUPayload(t, instanceMetrics, 1, 1)
				require.Len(t, taskMetrics, 1)
				require.Len(t, taskMetrics[0].ContainerMetrics, 1)
				requireContainerGPUPayload(t, taskMetrics[0].ContainerMetrics[0], []string{"GPU-1"})
			} else {
				assert.Nil(t, instanceMetrics, "instance GPU must not be emitted when ConnectionLost=true")
				requireNoContainerGPUPayload(t, taskMetrics)
			}

			// CPU/memory must keep flowing regardless of ConnectionLost.
			require.NotEmpty(t, taskMetrics)
			require.NotEmpty(t, taskMetrics[0].ContainerMetrics)
			assert.NotNil(t, taskMetrics[0].ContainerMetrics[0].CpuStatsSet,
				"CPU metrics must keep flowing")
			assert.NotNil(t, taskMetrics[0].ContainerMetrics[0].MemoryStatsSet,
				"memory metrics must keep flowing")
		})
	}
}
