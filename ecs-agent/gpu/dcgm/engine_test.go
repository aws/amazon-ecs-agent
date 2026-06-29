//go:build unit && linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package dcgm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEngine(client Client, outputPath string, collectionFreq time.Duration, oneShot bool) *Engine {
	return &Engine{
		client:         client,
		outputPath:     outputPath,
		collectionFreq: collectionFreq,
		oneShot:        oneShot,
	}
}

func TestRunExitsOnContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetReconcileJustInit(true)
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{GPUUUID: "GPU-test-001"},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := eng.run(ctx)

	assert.NoError(t, err, "run() should return nil on context cancellation")
}

func TestRunOneShotCollectsAndExits(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetReconcileJustInit(true)
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{GPUUUID: "GPU-test-001"},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, true)

	ctx := context.Background()

	err := eng.run(ctx)

	assert.NoError(t, err, "run() in one-shot mode should complete without error")

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var output metricsOutput
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)
	assert.Len(t, output.GPUs, 1)
	assert.Equal(t, "GPU-test-001", output.GPUs[0].GPUUUID)
}

func TestRunReconcileFailureReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetReconcileError(assert.AnError)

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	ctx := context.Background()

	err := eng.run(ctx)

	assert.Error(t, err, "run() should return error when initial reconciliation fails")
	assert.Contains(t, err.Error(), "initial DCGM reconciliation failed")
}

func TestStopReturnsNil(t *testing.T) {
	eng := &Engine{}
	err := eng.Stop()
	assert.NoError(t, err, "Stop() should always return nil")
}

func TestCollectAndWriteCreatesValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	utilization := 85.0
	memUtil := 50.0
	memTotal := uint64(16106127360)
	memUsed := uint64(8053063680)
	power := 250.5
	temp := 72.0

	mockClient := NewMockClient()
	mockClient.SetInitialized(true)
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{
			GPUUUID:            "GPU-abc-123",
			GPUUtilization:     &utilization,
			MemoryUtilization:  &memUtil,
			MemoryTotal:        &memTotal,
			MemoryUsed:         &memUsed,
			PowerDraw:          &power,
			Temperature:        &temp,
			RestartAppXidCount: 0,
		},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	err := eng.collectAndWrite(context.Background())
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var output metricsOutput
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)

	assert.NotEmpty(t, output.Timestamp)
	require.Len(t, output.GPUs, 1)
	assert.Equal(t, "GPU-abc-123", output.GPUs[0].GPUUUID)
	assert.Equal(t, 85.0, *output.GPUs[0].GPUUtilization)
	assert.Equal(t, 50.0, *output.GPUs[0].MemoryUtilization)
	assert.Equal(t, uint64(16106127360), *output.GPUs[0].MemoryTotal)
	assert.Equal(t, uint64(8053063680), *output.GPUs[0].MemoryUsed)
	assert.Equal(t, 250.5, *output.GPUs[0].PowerDraw)
	assert.Equal(t, 72.0, *output.GPUs[0].Temperature)
	assert.Equal(t, int64(0), output.GPUs[0].RestartAppXidCount)
}

func TestCollectAndWriteAtomicRename(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetInitialized(true)
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{GPUUUID: "GPU-test-001"},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	err := eng.collectAndWrite(context.Background())
	require.NoError(t, err)

	// Final file should exist
	_, err = os.Stat(outputPath)
	assert.NoError(t, err, "Output file should exist after collectAndWrite")

	// Staging file should NOT exist (renamed away)
	_, err = os.Stat(outputPath + ".staging")
	assert.True(t, os.IsNotExist(err), "Staging file should not exist after rename")
}

func TestCollectAndWriteReportsHealthyStatus(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetInitialized(true)
	mockClient.SetHealthy(true)
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{GPUUUID: "GPU-healthy-001"},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	err := eng.collectAndWrite(context.Background())
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var output metricsOutput
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)

	assert.True(t, output.Healthy, "Healthy GPU should report healthy=true")
	assert.Empty(t, output.UnhealthyReason, "Healthy GPU should have no unhealthy reason")
}

func TestCollectAndWriteReportsUnhealthyStatus(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetInitialized(true)
	mockClient.SetHealthy(false)
	mockClient.SetUnhealthyReason("XID_48")
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{GPUUUID: "GPU-unhealthy-001"},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	err := eng.collectAndWrite(context.Background())
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var output metricsOutput
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)

	assert.False(t, output.Healthy, "Unhealthy GPU should report healthy=false")
	assert.Equal(t, "XID_48", output.UnhealthyReason, "Should report the XID error code")
}

func TestCollectAndWriteReportsUnhealthyWhenNotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "gpu-metrics.json")

	mockClient := NewMockClient()
	mockClient.SetInitialized(true)
	mockClient.SetHealthy(false)
	mockClient.SetMetrics([]gputypes.GPUMetric{
		{GPUUUID: "GPU-disconnected-001"},
	})

	eng := newTestEngine(mockClient, outputPath, 60*time.Second, false)

	err := eng.collectAndWrite(context.Background())
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var output metricsOutput
	err = json.Unmarshal(data, &output)
	require.NoError(t, err)

	assert.False(t, output.Healthy, "Should report unhealthy when client is not healthy")
}
