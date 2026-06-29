//go:build unit && linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPUHealthcheckReportsOkWhenHealthy(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	err := os.WriteFile(filePath, []byte(`{
		"timestamp": "2026-01-01T00:00:00Z",
		"healthy": true,
		"gpus": [{"gpu_uuid": "GPU-001"}]
	}`), 0644)
	require.NoError(t, err)

	handler := gpu.NewDCGMHandler(filePath)
	hc := NewGPUHealthcheck(handler)
	status := hc.RunCheck()

	assert.Equal(t, ecstcs.InstanceHealthCheckStatusOk, status)
	assert.Equal(t, ecstcs.InstanceHealthCheckTypeAcceleratedCompute, hc.GetHealthcheckType())
}

func TestGPUHealthcheckReportsImpairedWhenUnhealthy(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	err := os.WriteFile(filePath, []byte(`{
		"timestamp": "2026-01-01T00:00:00Z",
		"healthy": false,
		"unhealthy_reason": "XID_48",
		"gpus": [{"gpu_uuid": "GPU-001"}]
	}`), 0644)
	require.NoError(t, err)

	handler := gpu.NewDCGMHandler(filePath)
	hc := NewGPUHealthcheck(handler)
	status := hc.RunCheck()

	assert.Equal(t, ecstcs.InstanceHealthCheckStatusImpaired, status)
}

func TestGPUHealthcheckReportsInsufficientDataWhenFileMissing(t *testing.T) {
	handler := gpu.NewDCGMHandler("/nonexistent/gpu-metrics.json")
	hc := NewGPUHealthcheck(handler)
	status := hc.RunCheck()

	assert.Equal(t, ecstcs.InstanceHealthCheckStatusInsufficientData, status)
}

func TestGPUHealthcheckReportsInsufficientDataWhenFileCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	err := os.WriteFile(filePath, []byte(`not valid json{{{`), 0644)
	require.NoError(t, err)

	handler := gpu.NewDCGMHandler(filePath)
	hc := NewGPUHealthcheck(handler)
	status := hc.RunCheck()

	assert.Equal(t, ecstcs.InstanceHealthCheckStatusInsufficientData, status)
}

func TestGPUHealthcheckStatusTransition(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "gpu-metrics.json")

	err := os.WriteFile(filePath, []byte(`{"timestamp":"2026-01-01T00:00:00Z","healthy":true,"gpus":[]}`), 0644)
	require.NoError(t, err)

	handler := gpu.NewDCGMHandler(filePath)
	hc := NewGPUHealthcheck(handler)
	status := hc.RunCheck()
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusOk, status)

	// Transition to unhealthy
	err = os.WriteFile(filePath, []byte(`{"timestamp":"2026-01-01T00:01:00Z","healthy":false,"unhealthy_reason":"XID_79","gpus":[]}`), 0644)
	require.NoError(t, err)

	status = hc.RunCheck()
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusImpaired, status)
	assert.Equal(t, ecstcs.InstanceHealthCheckStatusOk, hc.GetLastHealthcheckStatus())
}
