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

	testTimestamp       = "2026-07-19T00:00:00Z"
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

// TestDCGMMetricsReaderValidFile verifies healthy snapshots round-trip through
// the file: one deep-equal per case asserts the reader returns exactly what was
// written. Covers a fully populated GPU, four GPUs, a fractional vGPU with absent
// optional fields, connection-lost, and unhealthy.
func TestDCGMMetricsReaderValidFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		data gputypes.GPUMetricsFileData
	}{
		{
			name: "single GPU with all fields populated",
			data: gputypes.GPUMetricsFileData{
				Timestamp: testT4Timestamp,
				Healthy:   true,
				GPUs: []gputypes.GPUMetric{{
					GPUUUID:            testT4GPUUUID,
					GPUUtilization:     aws.Float64(testT4Util),
					MemoryUtilization:  aws.Float64(testT4MemUtil),
					MemoryTotal:        aws.Uint64(testT4MemTotal),
					MemoryUsed:         aws.Uint64(testT4MemUsed),
					PowerDraw:          aws.Float64(testT4PowerDraw),
					Temperature:        aws.Float64(testT4Temp),
					RestartAppXidCount: testT4Xid,
				}},
			},
		},
		{
			name: "four GPUs",
			data: gputypes.GPUMetricsFileData{
				Timestamp: testTimestamp,
				GPUs: []gputypes.GPUMetric{
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
				},
			},
		},
		{
			// PowerDraw and Temperature are left nil: their omitempty tags drop
			// them from the JSON, exactly as dcgm-init emits on a g6f fractional
			// vGPU instance. They must parse back as nil, not zero.
			name: "fractional vGPU with absent power and temperature",
			data: gputypes.GPUMetricsFileData{
				Timestamp: testTimestamp,
				GPUs: []gputypes.GPUMetric{{
					GPUUUID:           testGPUUUIDFractional,
					GPUUtilization:    aws.Float64(testFractionalUtil),
					MemoryUtilization: aws.Float64(testFractionalMemUtil),
					MemoryTotal:       aws.Uint64(testFractionalMem),
					MemoryUsed:        aws.Uint64(0),
				}},
			},
		},
		{
			// Healthy stays true while disconnected; callers rely on
			// ConnectionLost, not Healthy, to detect the dropped connection.
			name: "connection lost",
			data: gputypes.GPUMetricsFileData{
				Timestamp:      testTimestamp,
				Healthy:        true,
				ConnectionLost: true,
				GPUs:           []gputypes.GPUMetric{},
			},
		},
		{
			// Unhealthy GPU: healthy=false with a fault code. "XID_48" mirrors
			// dcgm/client's fmt.Sprintf("XID_%d").
			name: "unhealthy with fault reason",
			data: gputypes.GPUMetricsFileData{
				Timestamp:       testTimestamp,
				Healthy:         false,
				UnhealthyReason: testUnhealthyReason,
				GPUs:            []gputypes.GPUMetric{},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "gpu-metrics.json")
			writeMetricsFile(t, filePath, tc.data)

			result := NewDCGMMetricsReader(filePath).GetGPUMetrics()
			require.NotNil(t, result)
			assert.Equal(t, &tc.data, result, "parsed struct should match the written file")
		})
	}
}

// TestDCGMMetricsReaderRejectsAbsentFile covers cases where os.ReadFile fails, so
// the reader returns nil before parsing: missing parent dir, missing file in an
// existing dir (the common pre-first-write transient), path is a dir (EISDIR),
// non-dir parent (ENOTDIR), and an unreadable file. Each setup returns the path.
func TestDCGMMetricsReaderRejectsAbsentFile(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{
			name: "parent directory missing",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/gpu-metrics.json"
			},
		},
		{
			name: "file missing in existing directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "gpu-metrics.json")
			},
		},
		{
			name: "path is a directory",
			setup: func(t *testing.T) string {
				filePath := filepath.Join(t.TempDir(), "gpu-metrics.json")
				require.NoError(t, os.Mkdir(filePath, 0755))
				return filePath
			},
		},
		{
			name: "parent path is not a directory",
			setup: func(t *testing.T) string {
				notADir := filepath.Join(t.TempDir(), "not-a-dir")
				require.NoError(t, os.WriteFile(notADir, []byte("x"), 0644))
				return filepath.Join(notADir, "gpu-metrics.json")
			},
		},
		{
			// Root bypasses Unix permission checks, so this case cannot be
			// exercised as root and skips itself.
			name: "unreadable file",
			setup: func(t *testing.T) string {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses file permission checks; cannot exercise unreadable path")
				}
				filePath := filepath.Join(t.TempDir(), "gpu-metrics.json")
				require.NoError(t, os.WriteFile(filePath, []byte(`{"timestamp":"2026-01-01T00:00:00Z","gpus":[]}`), 0000))
				return filePath
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := NewDCGMMetricsReader(tc.setup(t)).GetGPUMetrics()
			assert.Nil(t, result, "an unreadable/absent file should return nil")
		})
	}
}

// TestDCGMMetricsReaderRejectsCorruptFile covers files that read but whose
// contents are rejected: non-JSON bytes, empty/whitespace (dcgm-init's
// pre-first-write state), a wrong-typed field, a top-level array, literal null,
// and an unparseable timestamp. All must return nil, not a partial result.
func TestDCGMMetricsReaderRejectsCorruptFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"invalid json", "not valid json{{{"},
		{"zero bytes", ""},
		{"whitespace only", "   \n\t  \n"},
		{"wrong field type", `{"timestamp":"2026-01-01T00:00:00Z","gpus":[{"gpu_utilization_percent":"not-a-number"}]}`},
		{"top-level array", "[]"},
		{"json null", "null"},
		{"unparseable timestamp", `{"timestamp":"not-a-timestamp","healthy":true,"gpus":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "gpu-metrics.json")
			require.NoError(t, os.WriteFile(filePath, []byte(tc.content), 0644))

			result := NewDCGMMetricsReader(filePath).GetGPUMetrics()
			assert.Nil(t, result, "corrupt file contents should return nil")
		})
	}
}

// TestDCGMMetricsReaderReturnsTimestamp verifies the reader returns the file's
// timestamp (so callers can detect stale data) and that repeated calls return
// the same data (the reader tracks no staleness itself).
func TestDCGMMetricsReaderReturnsTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	data := gputypes.GPUMetricsFileData{
		Timestamp: testTimestamp,
		GPUs: []gputypes.GPUMetric{
			{GPUUUID: testGPUUUID0, GPUUtilization: aws.Float64(testUtilization0)},
		},
	}
	writeMetricsFile(t, filePath, data)

	reader := NewDCGMMetricsReader(filePath)

	// Reader always returns data with the timestamp — no staleness logic
	result1 := reader.GetGPUMetrics()
	require.NotNil(t, result1)
	assert.Equal(t, testTimestamp, result1.Timestamp)
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

func writeMetricsFile(t *testing.T, path string, data gputypes.GPUMetricsFileData) {
	t.Helper()
	bytes, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, bytes, 0644)
	require.NoError(t, err)
}
