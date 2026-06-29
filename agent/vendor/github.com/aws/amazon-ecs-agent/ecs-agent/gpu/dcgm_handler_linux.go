//go:build linux

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
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	seelog "github.com/cihub/seelog"
)

// GPUMetric is the shared type from ecs-agent/gpu/types.
type GPUMetric = types.GPUMetric

const (
	// DefaultGPUMetricsFilePath is the shared file where dcgm-init writes GPU metrics.
	DefaultGPUMetricsFilePath = "/var/run/ecs/gpu-metrics.json"
)

// GPUMetricsFileData represents the JSON structure written by dcgm-init.
type GPUMetricsFileData struct {
	Timestamp       string      `json:"timestamp"`
	Healthy         bool        `json:"healthy"`
	UnhealthyReason string      `json:"unhealthy_reason,omitempty"`
	GPUs            []GPUMetric `json:"gpus"`
}

// GPUMetricsResult holds the parsed GPU metrics along with the timestamp
// and health status from dcgm-init. Callers use the timestamp to detect stale data.
type GPUMetricsResult struct {
	Timestamp       string
	Healthy         bool
	UnhealthyReason string
	Metrics         []GPUMetric
}

// DCGMHandler reads GPU metrics from the shared file written by dcgm-init
// and provides them to the stats engine for TACS reporting.
type DCGMHandler struct {
	filePath string
}

// NewDCGMHandler creates a new handler for reading GPU metrics from the shared file.
func NewDCGMHandler(filePath string) *DCGMHandler {
	if filePath == "" {
		filePath = DefaultGPUMetricsFilePath
	}
	return &DCGMHandler{
		filePath: filePath,
	}
}

// GetGPUMetrics reads and parses the GPU metrics file.
// Returns nil if the file is missing, corrupt, or has an invalid timestamp.
// The caller is responsible for staleness detection using the returned Timestamp.
func (h *DCGMHandler) GetGPUMetrics() *GPUMetricsResult {
	data, err := os.ReadFile(h.filePath)
	if err != nil {
		seelog.Debugf("GPU metrics file not available: %v", err)
		return nil
	}

	var fileData GPUMetricsFileData
	if err := json.Unmarshal(data, &fileData); err != nil {
		seelog.Warnf("Failed to parse GPU metrics file: %v", err)
		return nil
	}

	// Validate timestamp is parseable (reject corrupt files).
	if _, err := time.Parse(time.RFC3339, fileData.Timestamp); err != nil {
		seelog.Warnf("Failed to parse GPU metrics timestamp: %v", err)
		return nil
	}

	return &GPUMetricsResult{
		Timestamp:       fileData.Timestamp,
		Healthy:         fileData.Healthy,
		UnhealthyReason: fileData.UnhealthyReason,
		Metrics:         fileData.GPUs,
	}
}

// GPUHealthStatus holds the health state from the GPU metrics file.
type GPUHealthStatus struct {
	Healthy         bool
	UnhealthyReason string
}

// GetGPUHealthStatus returns the GPU health status from the shared metrics file.
// Returns nil if the file is unavailable or corrupt.
func (h *DCGMHandler) GetGPUHealthStatus() *GPUHealthStatus {
	result := h.GetGPUMetrics()
	if result == nil {
		return nil
	}
	return &GPUHealthStatus{
		Healthy:         result.Healthy,
		UnhealthyReason: result.UnhealthyReason,
	}
}
