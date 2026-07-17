//go:build !linux

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

// No-op DCGMMetricsReader for non-linux platforms so cross-platform callers (e.g. the
// stats engine) compile everywhere. dcgm-init is linux-only, so every method
// reports no data.

package gpu

import (
	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
)

// DCGMMetricsReader is the non-linux no-op counterpart of the linux reader; it holds
// no state and reports no data.
type DCGMMetricsReader struct{}

// NewDCGMMetricsReader returns a no-op reader on non-linux platforms.
func NewDCGMMetricsReader(_ string) *DCGMMetricsReader { return &DCGMMetricsReader{} }

// GetGPUMetrics always returns nil on non-linux platforms (no GPU metrics).
func (r *DCGMMetricsReader) GetGPUMetrics() *gputypes.GPUMetricsFileData { return nil }
