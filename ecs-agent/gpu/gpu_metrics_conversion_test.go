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
	"testing"

	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGpuMetricToGeneralMetricsWrapper(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		metric            gputypes.GPUMetric
		expectNil         bool
		expectedCount     int
		expectedDimValue  string
		expectedNames     []string
		expectedUnits     []string
		expectedIsDoubles []bool
	}{
		{
			name: "all fields populated",
			metric: gputypes.GPUMetric{
				GPUUUID:           "GPU-abc-123",
				GPUUtilization:    aws.Float64(75.5),
				MemoryUtilization: aws.Float64(60.0),
				MemoryTotal:       uint64Ptr(8589934592),
				MemoryUsed:        uint64Ptr(4294967296),
				PowerDraw:         aws.Float64(250.0),
				Temperature:       aws.Float64(72.0),
			},
			expectNil:        false,
			expectedCount:    7,
			expectedDimValue: "GPU-abc-123",
			expectedNames: []string{
				"GPUUtilization", "GPUMemoryUtilization", "GPUMemoryTotal",
				"GPUMemoryUsed", "GPUPowerDraw", "GPUTemperature", "GPURestartAppXidCount",
			},
			expectedUnits: []string{
				"Percent", "Percent", "Bytes", "Bytes", "None", "None", "Count",
			},
			expectedIsDoubles: []bool{true, true, false, false, true, true, false},
		},
		{
			name: "all fields nil",
			metric: gputypes.GPUMetric{
				GPUUUID: "GPU-nil-all",
			},
			expectNil: true,
		},
		{
			name: "only GPUUtilization set",
			metric: gputypes.GPUMetric{
				GPUUUID:        "GPU-single-util",
				GPUUtilization: aws.Float64(42.0),
			},
			expectNil:         false,
			expectedCount:     2,
			expectedDimValue:  "GPU-single-util",
			expectedNames:     []string{"GPUUtilization", "GPURestartAppXidCount"},
			expectedUnits:     []string{"Percent", "Count"},
			expectedIsDoubles: []bool{true, false},
		},
		{
			name: "only integer fields set",
			metric: gputypes.GPUMetric{
				GPUUUID:     "GPU-int-only",
				MemoryTotal: uint64Ptr(16000000000),
				MemoryUsed:  uint64Ptr(8000000000),
			},
			expectNil:         false,
			expectedCount:     3,
			expectedDimValue:  "GPU-int-only",
			expectedNames:     []string{"GPUMemoryTotal", "GPUMemoryUsed", "GPURestartAppXidCount"},
			expectedUnits:     []string{"Bytes", "Bytes", "Count"},
			expectedIsDoubles: []bool{false, false, false},
		},
		{
			name: "mixed nil and non-nil fields",
			metric: gputypes.GPUMetric{
				GPUUUID:           "GPU-mixed",
				GPUUtilization:    aws.Float64(90.0),
				MemoryUtilization: nil,
				MemoryTotal:       uint64Ptr(1024),
				MemoryUsed:        nil,
				PowerDraw:         aws.Float64(100.0),
				Temperature:       nil,
			},
			expectNil:         false,
			expectedCount:     4,
			expectedDimValue:  "GPU-mixed",
			expectedNames:     []string{"GPUUtilization", "GPUMemoryTotal", "GPUPowerDraw", "GPURestartAppXidCount"},
			expectedUnits:     []string{"Percent", "Bytes", "None", "Count"},
			expectedIsDoubles: []bool{true, false, true, false},
		},
		{
			name: "empty GPUUUID with fields populated",
			metric: gputypes.GPUMetric{
				GPUUUID:        "",
				GPUUtilization: aws.Float64(10.0),
			},
			expectNil:         false,
			expectedCount:     2,
			expectedDimValue:  "",
			expectedNames:     []string{"GPUUtilization", "GPURestartAppXidCount"},
			expectedUnits:     []string{"Percent", "Count"},
			expectedIsDoubles: []bool{true, false},
		},
		{
			name: "zero values for all fields",
			metric: gputypes.GPUMetric{
				GPUUUID:           "GPU-zeros",
				GPUUtilization:    aws.Float64(0),
				MemoryUtilization: aws.Float64(0),
				MemoryTotal:       uint64Ptr(0),
				MemoryUsed:        uint64Ptr(0),
				PowerDraw:         aws.Float64(0),
				Temperature:       aws.Float64(0),
			},
			expectNil:        false,
			expectedCount:    7,
			expectedDimValue: "GPU-zeros",
			expectedNames: []string{
				"GPUUtilization", "GPUMemoryUtilization", "GPUMemoryTotal",
				"GPUMemoryUsed", "GPUPowerDraw", "GPUTemperature", "GPURestartAppXidCount",
			},
			expectedUnits:     []string{"Percent", "Percent", "Bytes", "Bytes", "None", "None", "Count"},
			expectedIsDoubles: []bool{true, true, false, false, true, true, false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := GPUMetricToGeneralMetricsWrapper(tc.metric)

			if tc.expectNil {
				assert.Nil(t, result, "Expected nil wrapper when all fields are nil.")
				return
			}

			require.NotNil(t, result, "Expected non-nil wrapper.")

			// Verify dimension.
			require.Len(t, result.Dimensions, 1, "Expected exactly one dimension.")
			assert.Equal(t, "AcceleratedDevice", *result.Dimensions[0].Key)
			assert.Equal(t, tc.expectedDimValue, *result.Dimensions[0].Value)

			// Verify metric count.
			require.Len(t, result.GeneralMetrics, tc.expectedCount, "Unexpected number of GeneralMetric entries.")

			// Verify each metric name, unit, and value type.
			for i, gm := range result.GeneralMetrics {
				assert.Equal(t, tc.expectedNames[i], *gm.MetricName, "Metric name mismatch at index %d.", i)
				assert.Equal(t, tc.expectedUnits[i], *gm.Unit, "Unit mismatch at index %d.", i)
				if tc.expectedIsDoubles[i] {
					assert.NotNil(t, gm.MetricValueDouble, "Expected MetricValueDouble at index %d.", i)
					assert.Nil(t, gm.MetricValueLong, "Expected nil MetricValueLong for double metric at index %d.", i)
				} else {
					assert.NotNil(t, gm.MetricValueLong, "Expected MetricValueLong at index %d.", i)
					assert.Nil(t, gm.MetricValueDouble, "Expected nil MetricValueDouble for integer metric at index %d.", i)
				}
			}
		})
	}
}

func TestGpuMetricsToInstancePayload(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		metrics       []gputypes.GPUMetric
		usageTotal    int64
		expectNil     bool
		expectedLimit int64
		expectedUsage int64
	}{
		{
			name:      "empty slice returns nil",
			metrics:   []gputypes.GPUMetric{},
			expectNil: true,
		},
		{
			name:      "nil slice returns nil",
			metrics:   nil,
			expectNil: true,
		},
		{
			name: "single device with usage total 1",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
			},
			usageTotal:    1,
			expectNil:     false,
			expectedLimit: 1,
			expectedUsage: 1,
		},
		{
			name: "multiple devices with usage total matching count",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
				{GPUUUID: "GPU-2", Temperature: aws.Float64(70.0)},
				{GPUUUID: "GPU-3", MemoryTotal: uint64Ptr(8000)},
			},
			usageTotal:    3,
			expectNil:     false,
			expectedLimit: 3,
			expectedUsage: 3,
		},
		{
			name: "usage total less than device count",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
				{GPUUUID: "GPU-2"},
				{GPUUUID: "GPU-3", Temperature: aws.Float64(70.0)},
				{GPUUUID: "GPU-4"},
			},
			usageTotal:    2,
			expectNil:     false,
			expectedLimit: 4,
			expectedUsage: 2,
		},
		{
			name: "usage total zero",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1"},
				{GPUUUID: "GPU-2"},
			},
			usageTotal:    0,
			expectNil:     false,
			expectedLimit: 2,
			expectedUsage: 0,
		},
		{
			name: "eight devices with usage total 5",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1"}, {GPUUUID: "GPU-2"}, {GPUUUID: "GPU-3"}, {GPUUUID: "GPU-4"},
				{GPUUUID: "GPU-5"}, {GPUUUID: "GPU-6"}, {GPUUUID: "GPU-7"}, {GPUUUID: "GPU-8"},
			},
			usageTotal:    5,
			expectNil:     false,
			expectedLimit: 8,
			expectedUsage: 5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := GPUMetricsToInstancePayload(tc.metrics, tc.usageTotal)

			if tc.expectNil {
				assert.Nil(t, result, "Expected nil payload for empty input.")
				return
			}

			require.NotNil(t, result, "Expected non-nil payload.")
			require.Len(t, result, 1, "Expected exactly one wrapper.")

			wrapper := result[0]
			require.Len(t, wrapper.GeneralMetrics, 2, "Expected two instance-level metrics.")

			// Find metrics by name.
			var limitMetric, usageMetric *struct {
				name  string
				value int64
				unit  string
			}
			for _, gm := range wrapper.GeneralMetrics {
				require.NotNil(t, gm.MetricName)
				require.NotNil(t, gm.MetricValueLong)
				require.NotNil(t, gm.Unit)
				switch *gm.MetricName {
				case "InstanceGPULimit":
					limitMetric = &struct {
						name  string
						value int64
						unit  string
					}{*gm.MetricName, *gm.MetricValueLong, *gm.Unit}
				case "InstanceGPUUsageTotal":
					usageMetric = &struct {
						name  string
						value int64
						unit  string
					}{*gm.MetricName, *gm.MetricValueLong, *gm.Unit}
				}
			}

			require.NotNil(t, limitMetric, "InstanceGPULimit metric not found.")
			require.NotNil(t, usageMetric, "InstanceGPUUsageTotal metric not found.")

			assert.Equal(t, tc.expectedLimit, limitMetric.value, "InstanceGPULimit mismatch.")
			assert.Equal(t, "Count", limitMetric.unit)

			assert.Equal(t, tc.expectedUsage, usageMetric.value, "InstanceGPUUsageTotal mismatch.")
			assert.Equal(t, "Count", usageMetric.unit)
		})
	}
}

func TestGpuMetricsForContainer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		metrics       []gputypes.GPUMetric
		deviceIDs     []string
		expectNil     bool
		expectedUUIDs []string
	}{
		{
			name:      "empty metrics returns nil",
			metrics:   []gputypes.GPUMetric{},
			deviceIDs: []string{"GPU-1"},
			expectNil: true,
		},
		{
			name: "empty device list returns nil",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
			},
			deviceIDs: []string{},
			expectNil: true,
		},
		{
			name: "nil device list returns nil",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
			},
			deviceIDs: nil,
			expectNil: true,
		},
		{
			name: "matching UUIDs returns correct wrappers",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
				{GPUUUID: "GPU-2", Temperature: aws.Float64(70.0)},
				{GPUUUID: "GPU-3", PowerDraw: aws.Float64(200.0)},
			},
			deviceIDs:     []string{"GPU-1", "GPU-3"},
			expectNil:     false,
			expectedUUIDs: []string{"GPU-1", "GPU-3"},
		},
		{
			name: "no matching UUIDs returns nil",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
				{GPUUUID: "GPU-2", Temperature: aws.Float64(70.0)},
			},
			deviceIDs: []string{"GPU-99", "GPU-100"},
			expectNil: true,
		},
		{
			name: "partial matches returns only matched",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
				{GPUUUID: "GPU-2", Temperature: aws.Float64(70.0)},
				{GPUUUID: "GPU-3", PowerDraw: aws.Float64(200.0)},
			},
			deviceIDs:     []string{"GPU-2", "GPU-99"},
			expectNil:     false,
			expectedUUIDs: []string{"GPU-2"},
		},
		{
			name: "matching UUID but all nil fields returns nil",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1"}, // all nil fields
			},
			deviceIDs: []string{"GPU-1"},
			expectNil: true,
		},
		{
			name: "all UUIDs match",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
				{GPUUUID: "GPU-2", Temperature: aws.Float64(70.0)},
			},
			deviceIDs:     []string{"GPU-1", "GPU-2"},
			expectNil:     false,
			expectedUUIDs: []string{"GPU-1", "GPU-2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := GPUMetricsForContainer(tc.metrics, tc.deviceIDs)

			if tc.expectNil {
				assert.Nil(t, result, "Expected nil result.")
				return
			}

			require.NotNil(t, result, "Expected non-nil result.")
			require.Len(t, result, len(tc.expectedUUIDs), "Unexpected number of wrappers.")

			// Collect returned UUIDs.
			var returnedUUIDs []string
			for _, wrapper := range result {
				require.Len(t, wrapper.Dimensions, 1)
				assert.Equal(t, "AcceleratedDevice", *wrapper.Dimensions[0].Key)
				returnedUUIDs = append(returnedUUIDs, *wrapper.Dimensions[0].Value)
			}

			assert.Equal(t, tc.expectedUUIDs, returnedUUIDs, "Returned UUIDs mismatch.")
		})
	}
}

// uint64Ptr is a helper to create a pointer to a uint64 value.
func uint64Ptr(v uint64) *uint64 {
	return &v
}

// TestExtractInstanceGPUPayloadValues verifies that ExtractInstanceGPUPayloadValues
// correctly extracts InstanceGPULimit and InstanceGPUUsageTotal from payloads produced
// by GPUMetricsToInstancePayload, and returns ok=false for invalid payloads.
func TestExtractInstanceGPUPayloadValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		metrics       []gputypes.GPUMetric
		usageTotal    int64
		buildPayload  bool
		rawPayload    []*ecstcs.GeneralMetricsWrapper
		expectOK      bool
		expectedLimit int64
		expectedUsage int64
	}{
		{
			name: "round-trip single device usage 1",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1", GPUUtilization: aws.Float64(50.0)},
			},
			usageTotal:    1,
			buildPayload:  true,
			expectOK:      true,
			expectedLimit: 1,
			expectedUsage: 1,
		},
		{
			name: "round-trip four devices usage 2",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1"}, {GPUUUID: "GPU-2"}, {GPUUUID: "GPU-3"}, {GPUUUID: "GPU-4"},
			},
			usageTotal:    2,
			buildPayload:  true,
			expectOK:      true,
			expectedLimit: 4,
			expectedUsage: 2,
		},
		{
			name: "round-trip eight devices usage 0",
			metrics: []gputypes.GPUMetric{
				{GPUUUID: "GPU-1"}, {GPUUUID: "GPU-2"}, {GPUUUID: "GPU-3"}, {GPUUUID: "GPU-4"},
				{GPUUUID: "GPU-5"}, {GPUUUID: "GPU-6"}, {GPUUUID: "GPU-7"}, {GPUUUID: "GPU-8"},
			},
			usageTotal:    0,
			buildPayload:  true,
			expectOK:      true,
			expectedLimit: 8,
			expectedUsage: 0,
		},
		{
			name:         "nil payload returns ok false",
			buildPayload: false,
			rawPayload:   nil,
			expectOK:     false,
		},
		{
			name:         "empty payload returns ok false",
			buildPayload: false,
			rawPayload:   []*ecstcs.GeneralMetricsWrapper{},
			expectOK:     false,
		},
		{
			name:         "payload with nil wrapper returns ok false",
			buildPayload: false,
			rawPayload:   []*ecstcs.GeneralMetricsWrapper{nil},
			expectOK:     false,
		},
		{
			name:         "payload with empty metrics returns ok false",
			buildPayload: false,
			rawPayload: []*ecstcs.GeneralMetricsWrapper{
				{GeneralMetrics: []*ecstcs.GeneralMetric{}},
			},
			expectOK: false,
		},
		{
			name:         "payload with nil metric fields skips them and returns ok false",
			buildPayload: false,
			rawPayload: []*ecstcs.GeneralMetricsWrapper{
				{GeneralMetrics: []*ecstcs.GeneralMetric{
					{MetricName: nil, MetricValueLong: aws.Int64(1)},
					{MetricName: aws.String("InstanceGPULimit"), MetricValueLong: nil},
				}},
			},
			expectOK: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var payload []*ecstcs.GeneralMetricsWrapper
			if tc.buildPayload {
				payload = GPUMetricsToInstancePayload(tc.metrics, tc.usageTotal)
				require.NotNil(t, payload)
			} else {
				payload = tc.rawPayload
			}

			limit, usage, ok := ExtractInstanceGPUPayloadValues(payload)

			if !tc.expectOK {
				assert.False(t, ok, "Expected ok=false for invalid payload.")
				return
			}

			assert.True(t, ok, "Expected ok=true for valid payload.")
			assert.Equal(t, tc.expectedLimit, limit, "InstanceGPULimit mismatch.")
			assert.Equal(t, tc.expectedUsage, usage, "InstanceGPUUsageTotal mismatch.")
		})
	}
}
