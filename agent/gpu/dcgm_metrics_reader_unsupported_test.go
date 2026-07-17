//go:build unit && !linux

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDCGMMetricsReaderUnsupportedNoOp locks the non-linux stub's contract: it
// constructs without a real dcgm-init and reports no data (nil) so that
// cross-platform callers can rely on a uniform nil-means-no-data check.
func TestDCGMMetricsReaderUnsupportedNoOp(t *testing.T) {
	reader := NewDCGMMetricsReader("")
	require.NotNil(t, reader)

	assert.Nil(t, reader.GetGPUMetrics(), "non-linux GetGPUMetrics must return nil")
}
