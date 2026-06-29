//go:build unit && linux

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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPUMetricsNotEmittedWhenTimestampStale(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	// Write initial GPU metrics file
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:00:00Z", 85.0)

	telemetryMessages := make(chan ecstcs.TelemetryMessage, 10)
	healthMessages := make(chan ecstcs.HealthMessage, 10)

	engine := NewDockerStatsEngine(&cfg, nil, nil, telemetryMessages, healthMessages, nil)
	engine.ctx, _ = context.WithCancel(context.Background())
	engine.dcgmHandler = gpu.NewDCGMHandler(filePath)

	// Simulate 3 ticks to trigger GPU emission (gpuMetricsPublishCount >= 3)
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)

	// First emission should have instance metrics (new timestamp)
	assert.NotNil(t, engine.currentGPUMetrics, "First tick with new timestamp should populate currentGPUMetrics")
	assert.Equal(t, "2026-01-01T00:00:00Z", engine.lastGPUTimestamp)

	// Simulate 3 more ticks with the SAME file (timestamp unchanged)
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)

	// Second emission should NOT have GPU metrics (stale timestamp)
	assert.Nil(t, engine.currentGPUMetrics, "Second tick with same timestamp should not populate currentGPUMetrics")
	assert.Equal(t, "2026-01-01T00:00:00Z", engine.lastGPUTimestamp, "Timestamp should not change")
}

func TestGPUMetricsEmittedWhenTimestampChanges(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	// Write initial GPU metrics
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:00:00Z", 50.0)

	telemetryMessages := make(chan ecstcs.TelemetryMessage, 10)
	healthMessages := make(chan ecstcs.HealthMessage, 10)

	engine := NewDockerStatsEngine(&cfg, nil, nil, telemetryMessages, healthMessages, nil)
	engine.ctx, _ = context.WithCancel(context.Background())
	engine.dcgmHandler = gpu.NewDCGMHandler(filePath)

	// First emission
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.NotNil(t, engine.currentGPUMetrics)
	assert.Equal(t, "2026-01-01T00:00:00Z", engine.lastGPUTimestamp)

	// Same timestamp — no emission
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.Nil(t, engine.currentGPUMetrics)

	// Update file with new timestamp (simulates dcgm-init writing next tick)
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:01:00Z", 99.0)

	// New timestamp — should emit again
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.NotNil(t, engine.currentGPUMetrics, "New timestamp should trigger emission")
	assert.Equal(t, "2026-01-01T00:01:00Z", engine.lastGPUTimestamp)
	require.Len(t, engine.currentGPUMetrics, 1)
	assert.Equal(t, 99.0, *engine.currentGPUMetrics[0].GPUUtilization)
}

func TestGPUMetricsNotEmittedBeforeThirdTick(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	writeGPUMetricsFile(t, filePath, "2026-01-01T00:00:00Z", 75.0)

	telemetryMessages := make(chan ecstcs.TelemetryMessage, 10)
	healthMessages := make(chan ecstcs.HealthMessage, 10)

	engine := NewDockerStatsEngine(&cfg, nil, nil, telemetryMessages, healthMessages, nil)
	engine.ctx, _ = context.WithCancel(context.Background())
	engine.dcgmHandler = gpu.NewDCGMHandler(filePath)

	// First tick (count goes to 1) — should not emit
	engine.gpuMetricsPublishCount = 0
	engine.publishMetrics(false)
	assert.Nil(t, engine.currentGPUMetrics, "Should not emit on first tick")

	// Second tick (count goes to 2) — should not emit
	engine.publishMetrics(false)
	assert.Nil(t, engine.currentGPUMetrics, "Should not emit on second tick")

	// Third tick (count goes to 3, resets to 0) — should emit
	engine.publishMetrics(false)
	assert.NotNil(t, engine.currentGPUMetrics, "Should emit on third tick")
}

func TestGPUMetricsNotEmittedWhenTimestampGoesBackward(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	// Write metrics with a recent timestamp
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:05:00Z", 90.0)

	telemetryMessages := make(chan ecstcs.TelemetryMessage, 10)
	healthMessages := make(chan ecstcs.HealthMessage, 10)

	engine := NewDockerStatsEngine(&cfg, nil, nil, telemetryMessages, healthMessages, nil)
	engine.ctx, _ = context.WithCancel(context.Background())
	engine.dcgmHandler = gpu.NewDCGMHandler(filePath)

	// First emission succeeds (new timestamp)
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.NotNil(t, engine.currentGPUMetrics)
	assert.Equal(t, "2026-01-01T00:05:00Z", engine.lastGPUTimestamp)

	// Write metrics with an OLDER timestamp (clock skew, file corruption, etc.)
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:03:00Z", 10.0)

	// Should NOT emit because timestamp is older than lastGPUTimestamp
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.Nil(t, engine.currentGPUMetrics,
		"Should not emit metrics when file timestamp is older than last emitted timestamp")
	assert.Equal(t, "2026-01-01T00:05:00Z", engine.lastGPUTimestamp,
		"lastGPUTimestamp should remain at the newer value")

	// Write metrics with the same timestamp as the last emitted (equal, not greater)
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:05:00Z", 50.0)

	// Should NOT emit because timestamp is equal (not strictly greater)
	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.Nil(t, engine.currentGPUMetrics,
		"Should not emit metrics when file timestamp equals last emitted timestamp")

	// Write metrics with a newer timestamp — should emit
	writeGPUMetricsFile(t, filePath, "2026-01-01T00:06:00Z", 100.0)

	engine.gpuMetricsPublishCount = 2
	engine.publishMetrics(false)
	assert.NotNil(t, engine.currentGPUMetrics,
		"Should emit metrics when file timestamp is strictly newer")
	assert.Equal(t, "2026-01-01T00:06:00Z", engine.lastGPUTimestamp)
	assert.Equal(t, 100.0, *engine.currentGPUMetrics[0].GPUUtilization)
}

func writeGPUMetricsFile(t *testing.T, path string, timestamp string, utilization float64) {
	t.Helper()
	data := gpu.GPUMetricsFileData{
		Timestamp: timestamp,
		GPUs: []gpu.GPUMetric{
			{
				GPUUUID:        "GPU-test-001",
				GPUUtilization: aws.Float64(utilization),
			},
		},
	}
	bytes, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, bytes, 0644)
	require.NoError(t, err)
}
