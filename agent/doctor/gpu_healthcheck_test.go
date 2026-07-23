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

//go:build unit && linux

package doctor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/gpu"
	"github.com/aws/amazon-ecs-agent/ecs-agent/tcs/model/ecstcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ok           = ecstcs.InstanceHealthCheckStatusOk
	impaired     = ecstcs.InstanceHealthCheckStatusImpaired
	insufficient = ecstcs.InstanceHealthCheckStatusInsufficientData
	initializing = ecstcs.InstanceHealthCheckStatusInitializing
)

type step struct {
	write    *string // rewrite the metrics file before the check
	remove   bool    // remove the metrics file before the check
	now      time.Time
	want     ecstcs.InstanceHealthCheckStatus
	wantLast *ecstcs.InstanceHealthCheckStatus // optional GetLastHealthcheckStatus assertion
}

func TestGPUHealthcheckType(t *testing.T) {
	hc := NewGPUHealthcheck(gpu.NewDCGMMetricsReader("/nonexistent/gpu-metrics.json"))
	assert.Equal(t, ecstcs.InstanceHealthCheckTypeAcceleratedCompute, hc.GetHealthcheckType())
}

// TestGPUHealthcheckRunCheck is a table-driven test over RunCheck. Each case runs
// one or more sequential steps against a single healthcheck instance; a step
// optionally rewrites (or removes) the shared metrics file, sets the mocked
// clock, runs the check, and asserts the resulting status. The clock is anchored
// so every file timestamp has a deterministic age relative to the staleness
// threshold, and createdAt fixes the boot-grace origin.
func TestGPUHealthcheckRunCheck(t *testing.T) {

	str := func(s string) *string { return &s }
	last := func(s ecstcs.InstanceHealthCheckStatus) *ecstcs.InstanceHealthCheckStatus { return &s }

	// base is the healthcheck construction time (boot-grace origin) for the
	// multi-step scenarios; graceWithin/graceAfter bracket the boot grace window.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	graceWithin := base.Add(gpuBootGracePeriod - time.Second)
	graceAfter := base.Add(gpuBootGracePeriod)
	// fresh anchors the single-read cases; freshTS is an age-0 file timestamp.
	fresh := time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC)
	const freshTS = "2026-01-01T00:05:00Z"

	testCases := []struct {
		name      string
		createdAt time.Time
		steps     []step
	}{
		{
			name:      "healthy reports OK",
			createdAt: fresh,
			steps: []step{{
				write: str(`{"timestamp":"` + freshTS + `","healthy":true,"gpus":[{"gpu_uuid":"GPU-001"}]}`),
				now:   fresh, want: ok,
			}},
		},
		{
			name:      "unhealthy reports IMPAIRED",
			createdAt: fresh,
			steps: []step{{
				write: str(`{"timestamp":"` + freshTS + `","healthy":false,"unhealthy_reason":"XID_48","gpus":[{"gpu_uuid":"GPU-001"}]}`),
				now:   fresh, want: impaired,
			}},
		},
		{
			name:      "connection lost reports INSUFFICIENT_DATA even when healthy",
			createdAt: fresh,
			steps: []step{{
				write: str(`{"timestamp":"` + freshTS + `","healthy":true,"connection_lost":true,"gpus":[]}`),
				now:   fresh, want: insufficient,
			}},
		},
		{
			name:      "connection lost takes precedence over unhealthy",
			createdAt: fresh,
			steps: []step{{
				write: str(`{"timestamp":"` + freshTS + `","healthy":false,"unhealthy_reason":"XID_48","connection_lost":true,"gpus":[]}`),
				now:   fresh, want: insufficient,
			}},
		},
		{
			name:      "stale timestamp reports INSUFFICIENT_DATA",
			createdAt: fresh,
			steps: []step{{
				write: str(`{"timestamp":"2026-01-01T00:00:00Z","healthy":true,"gpus":[]}`), // 5m old > 120s
				now:   fresh, want: insufficient,
			}},
		},
		{
			name:      "future timestamp is treated as fresh (OK)",
			createdAt: fresh,
			steps: []step{{
				write: str(`{"timestamp":"2099-01-01T00:00:00Z","healthy":true,"gpus":[]}`),
				now:   fresh, want: ok,
			}},
		},
		{
			name:      "missing file: INITIALIZING within grace then INSUFFICIENT_DATA after",
			createdAt: base,
			steps: []step{
				{now: graceWithin, want: initializing}, // no write: file never exists
				{now: graceAfter, want: insufficient},
			},
		},
		{
			name:      "corrupt file: INITIALIZING within grace then INSUFFICIENT_DATA after",
			createdAt: base,
			steps: []step{
				{write: str(`not valid json{{{`), now: graceWithin, want: initializing},
				{now: graceAfter, want: insufficient},
			},
		},
		{
			name:      "empty file: INITIALIZING within grace then INSUFFICIENT_DATA after",
			createdAt: base,
			steps: []step{
				{write: str(``), now: graceWithin, want: initializing},
				{now: graceAfter, want: insufficient},
			},
		},
		{
			name:      "data loss after a real status is not masked by the boot grace",
			createdAt: base,
			steps: []step{
				{write: str(`{"timestamp":"2026-01-01T00:00:00Z","healthy":true,"gpus":[{"gpu_uuid":"GPU-001"}]}`), now: base, want: ok},
				{remove: true, now: graceWithin, want: insufficient},
			},
		},
		{
			name:      "OK to IMPAIRED transition records previous status",
			createdAt: base,
			steps: []step{
				{write: str(`{"timestamp":"2026-01-01T00:00:00Z","healthy":true,"gpus":[]}`), now: base, want: ok},
				{write: str(`{"timestamp":"2026-01-01T00:00:30Z","healthy":false,"unhealthy_reason":"XID_79","gpus":[]}`), now: base.Add(30 * time.Second), want: impaired, wantLast: last(ok)},
			},
		},
		{
			name:      "recovers from INSUFFICIENT_DATA back to OK (not latched)",
			createdAt: base,
			steps: []step{
				{write: str(`{"timestamp":"2026-01-01T00:00:00Z","healthy":true,"connection_lost":true,"gpus":[]}`), now: base, want: insufficient},
				{write: str(`{"timestamp":"2026-01-01T00:00:30Z","healthy":true,"gpus":[]}`), now: base.Add(30 * time.Second), want: ok, wantLast: last(insufficient)},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			original := timeNow
			defer func() { timeNow = original }()

			filePath := filepath.Join(t.TempDir(), "gpu-metrics.json")
			timeNow = func() time.Time { return tc.createdAt }
			hc := NewGPUHealthcheck(gpu.NewDCGMMetricsReader(filePath))

			for i, s := range tc.steps {
				if s.write != nil {
					require.NoError(t, os.WriteFile(filePath, []byte(*s.write), 0644), "step %d write", i)
				}
				if s.remove {
					require.NoError(t, os.Remove(filePath), "step %d remove", i)
				}
				stepNow := s.now
				timeNow = func() time.Time { return stepNow }

				assert.Equal(t, s.want, hc.RunCheck(), "step %d status", i)
				if s.wantLast != nil {
					assert.Equal(t, *s.wantLast, hc.GetLastHealthcheckStatus(), "step %d last status", i)
				}
			}
		})
	}
}
