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

	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

// DCGMMetricsReader reads GPU metrics from the shared file written by dcgm-init
// and provides them to the stats engine for TACS reporting.
type DCGMMetricsReader struct {
	filePath string
}

// NewDCGMMetricsReader returns a reader for filePath, defaulting to the shared
// gputypes.GPUMetricsFilePath when empty.
func NewDCGMMetricsReader(filePath string) *DCGMMetricsReader {
	if filePath == "" {
		filePath = gputypes.GPUMetricsFilePath
	}
	return &DCGMMetricsReader{
		filePath: filePath,
	}
}

// GetGPUMetrics reads and parses the GPU metrics file, returning nil if it is
// missing, unreadable, corrupt, or has an invalid timestamp. A missing file is
// the expected pre-first-write state (logged at Debug); an unreadable or corrupt
// file is unexpected (logged at Warn). The caller detects staleness via the
// returned Timestamp.
func (r *DCGMMetricsReader) GetGPUMetrics() *gputypes.GPUMetricsFileData {
	// A single os.ReadFile covers every case: os.IsNotExist is the expected
	// missing/pre-first-write file (Debug); any other error is unexpected (Warn).
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("GPU metrics file not available", logger.Fields{"error": err})
		} else {
			logger.Warn("GPU metrics file is not readable", logger.Fields{"path": r.filePath, "error": err})
		}
		return nil
	}

	// Unmarshal also rejects an empty/whitespace-only file, so no separate check.
	var fileData gputypes.GPUMetricsFileData
	if err := json.Unmarshal(data, &fileData); err != nil {
		logger.Warn("Failed to parse GPU metrics file", logger.Fields{"error": err})
		return nil
	}

	// Reject files with an unparseable timestamp as corrupt.
	if _, err := time.Parse(time.RFC3339, fileData.Timestamp); err != nil {
		logger.Warn("Failed to parse GPU metrics timestamp", logger.Fields{"error": err})
		return nil
	}

	return &fileData
}
