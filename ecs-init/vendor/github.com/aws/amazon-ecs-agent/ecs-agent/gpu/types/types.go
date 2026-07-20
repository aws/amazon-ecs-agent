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

package types

const (
	GPUMetricsDirPath  = "/var/run/ecs/gpu"
	GPUMetricsFileName = "gpu-metrics.json"
	GPUMetricsFilePath = GPUMetricsDirPath + "/" + GPUMetricsFileName
)

// GPUMetric holds per-device GPU telemetry. This struct is used by both
// dcgm-init (for collection) and the agent (for TACS conversion).
type GPUMetric struct {
	GPUUUID            string   `json:"gpu_uuid"`
	GPUUtilization     *float64 `json:"gpu_utilization_percent,omitempty"`
	MemoryUtilization  *float64 `json:"memory_utilization_percent,omitempty"`
	MemoryTotal        *uint64  `json:"memory_total_bytes,omitempty"`
	MemoryUsed         *uint64  `json:"memory_used_bytes,omitempty"`
	PowerDraw          *float64 `json:"power_draw_watts,omitempty"`
	Temperature        *float64 `json:"temperature_celsius,omitempty"`
	RestartAppXidCount int64    `json:"restart_app_xid_count"`
}

// GPUMetricsFileData is the JSON structure dcgm-init writes to the shared
// metrics file and the agent reads. Callers use the timestamp to detect stale data.
type GPUMetricsFileData struct {
	Timestamp       string `json:"timestamp"`
	Healthy         bool   `json:"healthy"`
	UnhealthyReason string `json:"unhealthy_reason,omitempty"`
	// ConnectionLost means dcgm-init lost its DCGM connection (past the grace
	// period). Healthy stays true while disconnected, so callers must treat this
	// as unknown (INSUFFICIENT_DATA), not OK.
	ConnectionLost bool        `json:"connection_lost,omitempty"`
	GPUs           []GPUMetric `json:"gpus"`
}
