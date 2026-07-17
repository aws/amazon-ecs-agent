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
	"os"
	"path/filepath"
	"testing"
	"time"

	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
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

	testMemUtil1 = 60.0
	testMemUtil2 = 45.0
	testMemUtil3 = 30.0
	testMemUtil4 = 10.0

	testMemTotal1     = uint64(16106127360) // 15 GiB (Tesla T4)
	testMemTotal2     = uint64(16106127360)
	testMemTotal3     = uint64(16106127360)
	testMemTotal4     = uint64(16106127360)
	testMemUsed1      = uint64(8053063680) // ~7.5 GiB
	testMemUsed2      = uint64(4026531840) // ~3.75 GiB
	testMemUsed3      = uint64(2013265920) // ~1.875 GiB
	testMemUsed4      = uint64(1006632960) // ~0.9 GiB
	testFractionalMem = uint64(6442450944) // 6 GiB (L4-6Q fractional)

	testPowerDraw1 = 70.0
	testPowerDraw2 = 65.0
	testPowerDraw3 = 50.0
	testPowerDraw4 = 9.0

	testTemp1 = 55.0
	testTemp2 = 50.0
	testTemp3 = 45.0
	testTemp4 = 40.0

	testRestartXidCount = int64(1)

	testFractionalUtil    = 0.0
	testFractionalMemUtil = 7.5

	testStaleTimestamp  = "2026-01-01T00:00:00Z"
	testUnhealthyReason = "XID_48" // mirrors dcgm/client's fmt.Sprintf("XID_%d")

	testT4GPUUUID   = "GPU-2a95d2f5-c87a-9cb4-52d4-393d7059e8f5"
	testT4Timestamp = "2026-07-14T19:13:51Z"
	testT4Util      = 42.0
	testT4MemUtil   = 25.0
	testT4MemTotal  = uint64(16106127360) // 15 GiB (Tesla T4)
	testT4MemUsed   = uint64(5368709120)  // ~5 GiB
	testT4PowerDraw = 9.887
	testT4Temp      = 36.0
	testT4Xid       = int64(2)
)

// TestDCGMMetricsReaderValidFile parses a healthy single-GPU snapshot and checks every
// field round-trips.
func TestDCGMMetricsReaderValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	writeMetricsFile(t, filePath, gputypes.GPUMetricsFileData{
		Timestamp: testT4Timestamp,
		Healthy:   true,
		GPUs: []gputypes.GPUMetric{
			{
				GPUUUID:            testT4GPUUUID,
				GPUUtilization:     aws.Float64(testT4Util),
				MemoryUtilization:  aws.Float64(testT4MemUtil),
				MemoryTotal:        aws.Uint64(testT4MemTotal),
				MemoryUsed:         aws.Uint64(testT4MemUsed),
				PowerDraw:          aws.Float64(testT4PowerDraw),
				Temperature:        aws.Float64(testT4Temp),
				RestartAppXidCount: testT4Xid,
			},
		},
	})

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()

	require.NotNil(t, result)
	assert.Equal(t, testT4Timestamp, result.Timestamp)
	assert.True(t, result.Healthy)
	require.Len(t, result.GPUs, 1)
	assert.Equal(t, testT4GPUUUID, result.GPUs[0].GPUUUID)
	assert.Equal(t, testT4Util, *result.GPUs[0].GPUUtilization)
	assert.Equal(t, testT4MemUtil, *result.GPUs[0].MemoryUtilization)
	assert.Equal(t, testT4MemTotal, *result.GPUs[0].MemoryTotal)
	assert.Equal(t, testT4MemUsed, *result.GPUs[0].MemoryUsed)
	assert.Equal(t, testT4PowerDraw, *result.GPUs[0].PowerDraw)
	assert.Equal(t, testT4Temp, *result.GPUs[0].Temperature)
	assert.Equal(t, testT4Xid, result.GPUs[0].RestartAppXidCount)
}

func TestDCGMMetricsReaderMultipleGPUs(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	expectedGPUs := []gputypes.GPUMetric{
		{
			GPUUUID:            testGPUUUID1,
			GPUUtilization:     aws.Float64(testUtilization1),
			MemoryUtilization:  aws.Float64(testMemUtil1),
			MemoryTotal:        aws.Uint64(testMemTotal1),
			MemoryUsed:         aws.Uint64(testMemUsed1),
			PowerDraw:          aws.Float64(testPowerDraw1),
			Temperature:        aws.Float64(testTemp1),
			RestartAppXidCount: 0,
		},
		{
			GPUUUID:            testGPUUUID2,
			GPUUtilization:     aws.Float64(testUtilization2),
			MemoryUtilization:  aws.Float64(testMemUtil2),
			MemoryTotal:        aws.Uint64(testMemTotal2),
			MemoryUsed:         aws.Uint64(testMemUsed2),
			PowerDraw:          aws.Float64(testPowerDraw2),
			Temperature:        aws.Float64(testTemp2),
			RestartAppXidCount: testRestartXidCount,
		},
		{
			GPUUUID:            testGPUUUID3,
			GPUUtilization:     aws.Float64(testUtilization3),
			MemoryUtilization:  aws.Float64(testMemUtil3),
			MemoryTotal:        aws.Uint64(testMemTotal3),
			MemoryUsed:         aws.Uint64(testMemUsed3),
			PowerDraw:          aws.Float64(testPowerDraw3),
			Temperature:        aws.Float64(testTemp3),
			RestartAppXidCount: 0,
		},
		{
			GPUUUID:            testGPUUUID4,
			GPUUtilization:     aws.Float64(testUtilization4),
			MemoryUtilization:  aws.Float64(testMemUtil4),
			MemoryTotal:        aws.Uint64(testMemTotal4),
			MemoryUsed:         aws.Uint64(testMemUsed4),
			PowerDraw:          aws.Float64(testPowerDraw4),
			Temperature:        aws.Float64(testTemp4),
			RestartAppXidCount: 0,
		},
	}

	data := gputypes.GPUMetricsFileData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		GPUs:      expectedGPUs,
	}

	writeMetricsFile(t, filePath, data)

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()

	require.NotNil(t, result)
	require.Len(t, result.GPUs, len(expectedGPUs))

	for i, expected := range expectedGPUs {
		actual := result.GPUs[i]
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

// TestDCGMMetricsReaderParentDirMissing covers a missing containing directory: the
// read fails with os.IsNotExist, so the reader returns nil. The
// dir-exists/file-missing case is covered by TestDCGMMetricsReaderFileMissingInExistingDir.
func TestDCGMMetricsReaderParentDirMissing(t *testing.T) {
	reader := NewDCGMMetricsReader("/nonexistent/path/gpu-metrics.json")
	result := reader.GetGPUMetrics()
	assert.Nil(t, result, "a missing parent directory should return nil")
}

func TestDCGMMetricsReaderInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	require.NoError(t, os.WriteFile(filePath, []byte("not valid json{{{"), 0644))

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	assert.Nil(t, result)
}

// TestDCGMMetricsReaderEmptyFile covers an empty or whitespace-only file, which
// dcgm-init creates before its first write. json.Unmarshal rejects it with
// "unexpected end of JSON input", so it returns nil via the parse-error branch.
func TestDCGMMetricsReaderEmptyFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"zero bytes", ""},
		{"whitespace only", "   \n\t  \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "gpu-metrics.json")
			require.NoError(t, os.WriteFile(filePath, []byte(tc.content), 0644))

			result := NewDCGMMetricsReader(filePath).GetGPUMetrics()
			assert.Nil(t, result, "empty/whitespace file should return nil (pre-first-write state)")
		})
	}
}

// TestDCGMMetricsReaderUnreadableFile verifies that when the metrics file exists but
// the agent process cannot read it (no read permission), the reader treats it
// as no metrics yet and returns nil rather than erroring. Root bypasses Unix
// permission checks, so the test is skipped when running as root.
func TestDCGMMetricsReaderUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks; cannot exercise unreadable path")
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	require.NoError(t, os.WriteFile(filePath, []byte(`{"timestamp":"2026-01-01T00:00:00Z","gpus":[]}`), 0000))

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	assert.Nil(t, result, "Unreadable file should return nil (no read permission)")
}

func TestDCGMMetricsReaderConnectionLost(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	writeMetricsFile(t, filePath, gputypes.GPUMetricsFileData{
		Timestamp:      testStaleTimestamp,
		Healthy:        true,
		ConnectionLost: true,
		GPUs:           []gputypes.GPUMetric{},
	})

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	require.NotNil(t, result)
	assert.True(t, result.ConnectionLost, "connection_lost should be parsed and propagated")
	assert.True(t, result.Healthy, "healthy stays true when disconnected; caller uses ConnectionLost")
}

// TestDCGMMetricsReaderUnhealthyReason covers an unhealthy GPU: healthy=false with a
// fault code. "XID_48" mirrors dcgm/client's fmt.Sprintf("XID_%d").
func TestDCGMMetricsReaderUnhealthyReason(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	writeMetricsFile(t, filePath, gputypes.GPUMetricsFileData{
		Timestamp:       testStaleTimestamp,
		Healthy:         false,
		UnhealthyReason: testUnhealthyReason,
		GPUs:            []gputypes.GPUMetric{},
	})

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	require.NotNil(t, result)
	assert.False(t, result.Healthy, "healthy should be parsed as false")
	assert.Equal(t, testUnhealthyReason, result.UnhealthyReason, "unhealthy_reason should be parsed and propagated")
	assert.False(t, result.ConnectionLost, "connection_lost absent -> false")
}

// TestDCGMMetricsReaderFractionalGPUMissingFields verifies that absent power_draw_watts
// and temperature_celsius fields (as on g6f fractional vGPU instances) parse as
// nil, not zero values.
func TestDCGMMetricsReaderFractionalGPUMissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	// Leave PowerDraw and Temperature nil: their omitempty tags drop them from the
	// marshaled JSON, exactly as dcgm-init emits on a g6f instance (fields absent).
	writeMetricsFile(t, filePath, gputypes.GPUMetricsFileData{
		Timestamp: testStaleTimestamp,
		GPUs: []gputypes.GPUMetric{
			{
				GPUUUID:           testGPUUUIDFractional,
				GPUUtilization:    aws.Float64(testFractionalUtil),
				MemoryUtilization: aws.Float64(testFractionalMemUtil),
				MemoryTotal:       aws.Uint64(testFractionalMem),
				MemoryUsed:        aws.Uint64(0),
			},
		},
	})

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()

	require.NotNil(t, result)
	require.Len(t, result.GPUs, 1)
	assert.Equal(t, testGPUUUIDFractional, result.GPUs[0].GPUUUID)
	assert.NotNil(t, result.GPUs[0].GPUUtilization)
	assert.Equal(t, testFractionalUtil, *result.GPUs[0].GPUUtilization)
	assert.NotNil(t, result.GPUs[0].MemoryUtilization)
	assert.Equal(t, testFractionalMemUtil, *result.GPUs[0].MemoryUtilization)
	assert.NotNil(t, result.GPUs[0].MemoryTotal)
	assert.Equal(t, testFractionalMem, *result.GPUs[0].MemoryTotal)
	assert.Nil(t, result.GPUs[0].PowerDraw,
		"PowerDraw should be nil when field is absent from JSON (not zero)")
	assert.Nil(t, result.GPUs[0].Temperature,
		"Temperature should be nil when field is absent from JSON (not zero)")
	assert.Equal(t, int64(0), result.GPUs[0].RestartAppXidCount)
}

// TestDCGMMetricsReaderReturnsTimestamp verifies that the reader returns the timestamp
// from the file so callers can detect stale data.
func TestDCGMMetricsReaderReturnsTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	data := gputypes.GPUMetricsFileData{
		Timestamp: testStaleTimestamp,
		GPUs: []gputypes.GPUMetric{
			{GPUUUID: testGPUUUID0, GPUUtilization: aws.Float64(testUtilization0)},
		},
	}
	writeMetricsFile(t, filePath, data)

	reader := NewDCGMMetricsReader(filePath)

	// Reader always returns data with the timestamp — no staleness logic
	result1 := reader.GetGPUMetrics()
	require.NotNil(t, result1)
	assert.Equal(t, testStaleTimestamp, result1.Timestamp)
	require.Len(t, result1.GPUs, 1)

	// Second call returns the same data (reader doesn't track staleness)
	result2 := reader.GetGPUMetrics()
	require.NotNil(t, result2)
	assert.Equal(t, result1.Timestamp, result2.Timestamp)
	assert.Equal(t, result1.GPUs[0].GPUUUID, result2.GPUs[0].GPUUUID)
}

func TestDCGMMetricsReaderDefaultFilePath(t *testing.T) {
	reader := NewDCGMMetricsReader("")
	assert.Equal(t, gputypes.GPUMetricsFilePath, reader.filePath)
}

// TestDCGMMetricsReaderFileMissingInExistingDir covers the common runtime transient:
// the /var/run/ecs directory exists (bind mount present) but dcgm-init has not
// written the metrics file yet. The reader must return nil.
func TestDCGMMetricsReaderFileMissingInExistingDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Point at a file that does not exist inside a directory that does.
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	assert.Nil(t, result, "Missing file in an existing directory should return nil")
}

// TestDCGMMetricsReaderPathIsDirectory covers the case where the metrics file path is
// itself a directory: os.ReadFile fails (EISDIR), so the reader returns nil.
func TestDCGMMetricsReaderPathIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a directory where the metrics file is expected.
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	require.NoError(t, os.Mkdir(filePath, 0755))

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	assert.Nil(t, result, "A directory in place of the metrics file should return nil")
}

// TestDCGMMetricsReaderDirPathNotADirectory covers a non-directory path component: the
// parent path exists but is a regular file, so os.ReadFile fails (ENOTDIR) and
// the reader returns nil.
func TestDCGMMetricsReaderDirPathNotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a regular file, then treat it as the containing directory by
	// requesting a metrics file "inside" it.
	notADir := filepath.Join(tmpDir, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0644))
	filePath := filepath.Join(notADir, "gpu-metrics.json")

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	assert.Nil(t, result, "A non-directory parent path should return nil")
}

// TestDCGMMetricsReaderInvalidTimestamp covers the timestamp-validation branch: valid
// JSON but a timestamp that is not RFC3339 is rejected as corrupt.
func TestDCGMMetricsReaderInvalidTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	writeMetricsFile(t, filePath, gputypes.GPUMetricsFileData{
		Timestamp: "not-a-timestamp",
		Healthy:   true,
		GPUs:      []gputypes.GPUMetric{},
	})

	reader := NewDCGMMetricsReader(filePath)
	result := reader.GetGPUMetrics()
	assert.Nil(t, result, "Unparseable timestamp should return nil (treated as corrupt)")
}

// TestDCGMMetricsReaderJSONTypeMismatch verifies that structurally valid JSON with a
// wrong field type (string where a number is expected) is rejected as corrupt
// (unmarshal error), returning nil rather than panicking.
func TestDCGMMetricsReaderJSONTypeMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	require.NoError(t, os.WriteFile(filePath, []byte(`{
		"timestamp": "2026-01-01T00:00:00Z",
		"gpus": [{"gpu_utilization_percent": "not-a-number"}]
	}`), 0644))

	reader := NewDCGMMetricsReader(filePath)
	assert.Nil(t, reader.GetGPUMetrics(), "wrong-typed field should return nil (parse error)")
}

// TestDCGMMetricsReaderTopLevelArray verifies a JSON array where an object is expected
// is rejected as corrupt, returning nil.
func TestDCGMMetricsReaderTopLevelArray(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	require.NoError(t, os.WriteFile(filePath, []byte(`[]`), 0644))

	reader := NewDCGMMetricsReader(filePath)
	assert.Nil(t, reader.GetGPUMetrics(), "top-level array should return nil (parse error)")
}

// TestDCGMMetricsReaderJSONNull verifies a literal JSON null (which unmarshals without
// error into a zero-value struct) is rejected via the empty-timestamp path,
// returning nil rather than a zero-value result.
func TestDCGMMetricsReaderJSONNull(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")
	require.NoError(t, os.WriteFile(filePath, []byte(`null`), 0644))

	reader := NewDCGMMetricsReader(filePath)
	assert.Nil(t, reader.GetGPUMetrics(), "JSON null should return nil (empty timestamp)")
}

func writeMetricsFile(t *testing.T, path string, data gputypes.GPUMetricsFileData) {
	t.Helper()
	bytes, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, bytes, 0644)
	require.NoError(t, err)
}
