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

package gpu

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants to avoid hardcoding values throughout.
const (
	testGPUUUID0          = "GPU-abc-123"
	testGPUUUIDFractional = "GPU-fractional-raw"
	testGPUUUID1          = "GPU-0"
	testGPUUUID2          = "GPU-1"
	testGPUUUID3          = "GPU-2"
	testGPUUUID4          = "GPU-3"

	testUtilization0 = 85.0
	testUtilization1 = 100.0
	testUtilization2 = 75.0
	testUtilization3 = 50.0
	testUtilization4 = 25.0

	testMemUtil0 = 50.0

	testMemTotal0     = uint64(16106127360) // 15 GiB (Tesla T4)
	testMemUsed0      = uint64(8053063680)  // ~7.5 GiB
	testFractionalMem = uint64(6442450944)  // 6 GiB (L4-6Q fractional)

	testPowerDraw0 = 250.5
	testTemp0      = 72.0
	testTemp1      = 55.0
	testTemp2      = 50.0
	testTemp3      = 45.0
	testTemp4      = 40.0

	testFractionalUtil    = 0.0
	testFractionalMemUtil = 7.5

	testStaleTimestamp = "2026-01-01T00:00:00Z"
)

func TestDCGMHandlerValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	data := GPUMetricsFileData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		GPUs: []GPUMetric{
			{
				GPUUUID:            testGPUUUID0,
				GPUUtilization:     aws.Float64(testUtilization0),
				MemoryUtilization:  aws.Float64(testMemUtil0),
				MemoryTotal:        aws.Uint64(testMemTotal0),
				MemoryUsed:         aws.Uint64(testMemUsed0),
				PowerDraw:          aws.Float64(testPowerDraw0),
				Temperature:        aws.Float64(testTemp0),
				RestartAppXidCount: 0,
			},
		},
	}

	writeMetricsFile(t, filePath, data)

	handler := NewDCGMHandler(filePath)
	result := handler.GetGPUMetrics()

	require.NotNil(t, result)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, testGPUUUID0, result.Metrics[0].GPUUUID)
	assert.Equal(t, testUtilization0, *result.Metrics[0].GPUUtilization)
	assert.Equal(t, testMemUtil0, *result.Metrics[0].MemoryUtilization)
	assert.Equal(t, testMemTotal0, *result.Metrics[0].MemoryTotal)
	assert.Equal(t, testMemUsed0, *result.Metrics[0].MemoryUsed)
	assert.Equal(t, testPowerDraw0, *result.Metrics[0].PowerDraw)
	assert.Equal(t, testTemp0, *result.Metrics[0].Temperature)
	assert.Equal(t, int64(0), result.Metrics[0].RestartAppXidCount)
}

func TestDCGMHandlerMultipleGPUs(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	expectedGPUs := []GPUMetric{
		{
			GPUUUID:            testGPUUUID1,
			GPUUtilization:     aws.Float64(testUtilization1),
			MemoryUtilization:  aws.Float64(60.0),
			MemoryTotal:        aws.Uint64(testMemTotal0),
			MemoryUsed:         aws.Uint64(testMemUsed0),
			PowerDraw:          aws.Float64(70.0),
			Temperature:        aws.Float64(testTemp1),
			RestartAppXidCount: 0,
		},
		{
			GPUUUID:            testGPUUUID2,
			GPUUtilization:     aws.Float64(testUtilization2),
			MemoryUtilization:  aws.Float64(45.0),
			MemoryTotal:        aws.Uint64(testMemTotal0),
			MemoryUsed:         aws.Uint64(4026531840),
			PowerDraw:          aws.Float64(65.0),
			Temperature:        aws.Float64(testTemp2),
			RestartAppXidCount: 1,
		},
		{
			GPUUUID:            testGPUUUID3,
			GPUUtilization:     aws.Float64(testUtilization3),
			MemoryUtilization:  aws.Float64(30.0),
			MemoryTotal:        aws.Uint64(testMemTotal0),
			MemoryUsed:         aws.Uint64(2013265920),
			PowerDraw:          aws.Float64(50.0),
			Temperature:        aws.Float64(testTemp3),
			RestartAppXidCount: 0,
		},
		{
			GPUUUID:            testGPUUUID4,
			GPUUtilization:     aws.Float64(testUtilization4),
			MemoryUtilization:  aws.Float64(10.0),
			MemoryTotal:        aws.Uint64(testMemTotal0),
			MemoryUsed:         aws.Uint64(0),
			PowerDraw:          aws.Float64(9.0),
			Temperature:        aws.Float64(testTemp4),
			RestartAppXidCount: 0,
		},
	}

	data := GPUMetricsFileData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		GPUs:      expectedGPUs,
	}

	writeMetricsFile(t, filePath, data)

	handler := NewDCGMHandler(filePath)
	result := handler.GetGPUMetrics()

	require.NotNil(t, result)
	require.Len(t, result.Metrics, len(expectedGPUs))

	for i, expected := range expectedGPUs {
		actual := result.Metrics[i]
		assert.Equal(t, expected.GPUUUID, actual.GPUUUID, "GPU %d UUID mismatch", i)
		assert.Equal(t, *expected.GPUUtilization, *actual.GPUUtilization, "GPU %d utilization mismatch", i)
		assert.Equal(t, *expected.MemoryUtilization, *actual.MemoryUtilization, "GPU %d memory utilization mismatch", i)
		assert.Equal(t, *expected.MemoryTotal, *actual.MemoryTotal, "GPU %d memory total mismatch", i)
		assert.Equal(t, *expected.MemoryUsed, *actual.MemoryUsed, "GPU %d memory used mismatch", i)
		assert.Equal(t, *expected.PowerDraw, *actual.PowerDraw, "GPU %d power draw mismatch", i)
		assert.Equal(t, *expected.Temperature, *actual.Temperature, "GPU %d temperature mismatch", i)
		assert.Equal(t, expected.RestartAppXidCount, actual.RestartAppXidCount, "GPU %d XID count mismatch", i)
	}
}

func TestDCGMHandlerFileNotFound(t *testing.T) {
	handler := NewDCGMHandler("/nonexistent/path/gpu-metrics.json")
	result := handler.GetGPUMetrics()
	assert.Nil(t, result)
}

func TestDCGMHandlerInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	os.WriteFile(filePath, []byte("not valid json{{{"), 0644)

	handler := NewDCGMHandler(filePath)
	result := handler.GetGPUMetrics()
	assert.Nil(t, result)
}

func TestDCGMHandlerEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	os.WriteFile(filePath, []byte(""), 0644)

	handler := NewDCGMHandler(filePath)
	result := handler.GetGPUMetrics()
	assert.Nil(t, result, "Empty file should return nil (JSON parsing fails gracefully)")
}

// TestDCGMHandler_GetGPUMetrics_FractionalGPU_MissingFieldsInJSON verifies that
// when dcgm-init writes JSON without power_draw_watts and temperature_celsius fields
// (as happens on g6f fractional vGPU instances), the handler correctly parses them
// as nil rather than zero values.
func TestDCGMHandlerFractionalGPUMissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	// Write raw JSON exactly as dcgm-init would on a g6f instance —
	// power_draw_watts and temperature_celsius are completely absent (not null).
	rawJSON := fmt.Sprintf(`{
  "timestamp": "%s",
  "gpus": [
    {
      "gpu_uuid": "%s",
      "gpu_utilization_percent": %v,
      "memory_utilization_percent": %v,
      "memory_total_bytes": %d,
      "memory_used_bytes": 0,
      "restart_app_xid_count": 0
    }
  ]
}`, time.Now().UTC().Format(time.RFC3339), testGPUUUIDFractional, testFractionalUtil, testFractionalMemUtil, testFractionalMem)

	err := os.WriteFile(filePath, []byte(rawJSON), 0644)
	require.NoError(t, err)

	handler := NewDCGMHandler(filePath)
	result := handler.GetGPUMetrics()

	require.NotNil(t, result)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, testGPUUUIDFractional, result.Metrics[0].GPUUUID)
	assert.NotNil(t, result.Metrics[0].GPUUtilization)
	assert.Equal(t, testFractionalUtil, *result.Metrics[0].GPUUtilization)
	assert.NotNil(t, result.Metrics[0].MemoryUtilization)
	assert.Equal(t, testFractionalMemUtil, *result.Metrics[0].MemoryUtilization)
	assert.NotNil(t, result.Metrics[0].MemoryTotal)
	assert.Equal(t, testFractionalMem, *result.Metrics[0].MemoryTotal)
	assert.Nil(t, result.Metrics[0].PowerDraw,
		"PowerDraw should be nil when field is absent from JSON (not zero)")
	assert.Nil(t, result.Metrics[0].Temperature,
		"Temperature should be nil when field is absent from JSON (not zero)")
	assert.Equal(t, int64(0), result.Metrics[0].RestartAppXidCount)
}

// TestDCGMHandlerReturnsTimestamp verifies that the handler returns the timestamp
// from the file so callers can detect stale data.
func TestDCGMHandlerReturnsTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	data := GPUMetricsFileData{
		Timestamp: testStaleTimestamp,
		GPUs: []GPUMetric{
			{GPUUUID: testGPUUUID0, GPUUtilization: aws.Float64(testUtilization0)},
		},
	}
	writeMetricsFile(t, filePath, data)

	handler := NewDCGMHandler(filePath)

	// Handler always returns data with the timestamp — no staleness logic
	result1 := handler.GetGPUMetrics()
	require.NotNil(t, result1)
	assert.Equal(t, testStaleTimestamp, result1.Timestamp)
	require.Len(t, result1.Metrics, 1)

	// Second call returns the same data (handler doesn't track staleness)
	result2 := handler.GetGPUMetrics()
	require.NotNil(t, result2)
	assert.Equal(t, result1.Timestamp, result2.Timestamp)
	assert.Equal(t, result1.Metrics[0].GPUUUID, result2.Metrics[0].GPUUUID)
}

func TestDCGMHandlerDefaultFilePath(t *testing.T) {
	handler := NewDCGMHandler("")
	assert.Equal(t, DefaultGPUMetricsFilePath, handler.filePath)
}

func writeMetricsFile(t *testing.T, path string, data GPUMetricsFileData) {
	t.Helper()
	bytes, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, bytes, 0644)
	require.NoError(t, err)
}
