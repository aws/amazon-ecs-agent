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

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	mock_dcgm "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgm/mocks"
	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEngine builds an Engine wired to a mock client, writing to outputPath
// and ticking at collectionInterval, so tests can redirect writes to a temp
// directory and shrink the ticker instead of touching /var/run/ecs or waiting a
// full production interval.
func newTestEngine(client *mock_dcgm.MockClient, outputPath string, collectionInterval time.Duration) *Engine {
	return &Engine{
		client:             client,
		outputPath:         outputPath,
		collectionInterval: collectionInterval,
	}
}

// expectStatus stubs the health-reporting methods every reconcileAndCollect
// invokes once it gets past reconciliation. The returned values are not asserted
// for the time being; the stubs only satisfy the calls reconcileAndCollect makes.
func expectStatus(m *mock_dcgm.MockClient, healthy bool, reason string, connLost bool) {
	m.EXPECT().IsHealthy().Return(healthy).AnyTimes()
	m.EXPECT().UnhealthyReason().Return(reason).AnyTimes()
	m.EXPECT().IsConnectionLost().Return(connLost).AnyTimes()
}

// readOutput reads and unmarshals the metrics file written by the engine.
func readOutput(t *testing.T, path string) dcgmOutput {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var out dcgmOutput
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// sentinelTimestamp seeds a metrics file with a timestamp decades in the past.
// Since reconcileAndCollect stamps the current time on write, a read-back that
// still shows the sentinel proves the file was left untouched, while any other
// value proves a fresh write — independent of clock granularity.
const sentinelTimestamp = "2000-01-01T00:00:00Z"

// seedOutput writes a pre-existing metrics snapshot stamped with
// sentinelTimestamp to path, so a later read reveals whether reconcileAndCollect
// overwrote it.
func seedOutput(t *testing.T, path string) {
	t.Helper()
	data, err := json.MarshalIndent(dcgmOutput{
		Timestamp: sentinelTimestamp,
		Healthy:   true,
		GPUs:      []gputypes.GPUMetric{{GPUUUID: "GPU-preexisting"}},
	}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, metricsFilePermission))
}

func TestNewEngine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{
		{"creates Engine with correct fields"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			eng, err := New()

			require.NoError(t, err)
			require.NotNil(t, eng)
			assert.NotNil(t, eng.client)
			assert.Equal(t, MetricsFilePath, eng.outputPath)
			assert.Equal(t, metricsCollectionInterval, eng.collectionInterval)
			assert.Equal(t, MetricsFilePath+".tmp", eng.tempPath())
		})
	}
}

func TestEngine_ReconcileAndCollect(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		setupMock        func(*mock_dcgm.MockClient, *atomic.Int64, *atomic.Int64)
		expectGetMetrics bool
		// wantFreshWrite is true when reconcileAndCollect is expected to write a
		// fresh snapshot (advancing the seeded sentinel timestamp), false when it
		// should leave the pre-existing file untouched.
		wantFreshWrite bool
		verify         func(t *testing.T, out dcgmOutput)
	}{
		{
			name: "reconcile failure leaves the existing file untouched",
			setupMock: func(m *mock_dcgm.MockClient, reconciles, getMetrics *atomic.Int64) {
				m.EXPECT().Reconcile(gomock.Any()).DoAndReturn(func(context.Context) (bool, error) {
					reconciles.Add(1)
					return false, errors.New("connection failed")
				}).AnyTimes()
				m.EXPECT().GetMetrics(gomock.Any()).DoAndReturn(func(context.Context) ([]gputypes.GPUMetric, error) {
					getMetrics.Add(1)
					return nil, nil
				}).AnyTimes()
			},
			expectGetMetrics: false,
			wantFreshWrite:   false,
		},
		{
			name: "GetMetrics failure writes a fresh status snapshot",
			setupMock: func(m *mock_dcgm.MockClient, reconciles, getMetrics *atomic.Int64) {
				m.EXPECT().Reconcile(gomock.Any()).DoAndReturn(func(context.Context) (bool, error) {
					reconciles.Add(1)
					return true, nil
				}).AnyTimes()
				m.EXPECT().GetMetrics(gomock.Any()).DoAndReturn(func(context.Context) ([]gputypes.GPUMetric, error) {
					getMetrics.Add(1)
					return nil, errors.New("metrics unavailable")
				}).AnyTimes()
				expectStatus(m, true, "", true)
			},
			expectGetMetrics: true,
			// A GetMetrics failure is non-fatal: a status snapshot with no per-GPU
			// metrics is still written. The harness confirms the timestamp advanced;
			// here we confirm the snapshot has no GPUs.
			wantFreshWrite: true,
			verify: func(t *testing.T, out dcgmOutput) {
				assert.Empty(t, out.GPUs, "no per-GPU metrics should be present on collection failure")
			},
		},
		{
			name: "successful collection writes metrics to file",
			setupMock: func(m *mock_dcgm.MockClient, reconciles, getMetrics *atomic.Int64) {
				m.EXPECT().Reconcile(gomock.Any()).DoAndReturn(func(context.Context) (bool, error) {
					reconciles.Add(1)
					return true, nil
				}).AnyTimes()
				utilization := 75.0
				m.EXPECT().GetMetrics(gomock.Any()).DoAndReturn(func(context.Context) ([]gputypes.GPUMetric, error) {
					getMetrics.Add(1)
					return []gputypes.GPUMetric{
						{GPUUUID: "GPU-test-uuid-1", GPUUtilization: &utilization},
					}, nil
				}).AnyTimes()
				expectStatus(m, true, "", false)
			},
			expectGetMetrics: true,
			wantFreshWrite:   true,
			verify: func(t *testing.T, out dcgmOutput) {
				require.Len(t, out.GPUs, 1)
				assert.Equal(t, "GPU-test-uuid-1", out.GPUs[0].GPUUUID)
				require.NotNil(t, out.GPUs[0].GPUUtilization)
				assert.Equal(t, 75.0, *out.GPUs[0].GPUUtilization)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			outputPath := filepath.Join(t.TempDir(), "gpu-metrics.json")

			// Seed a pre-existing snapshot stamped with sentinelTimestamp so we can
			// tell, by whether that timestamp advances, if reconcileAndCollect wrote
			// a fresh snapshot or left the file untouched.
			seedOutput(t, outputPath)

			var reconcileCalls, getMetricsCalls atomic.Int64
			mockClient := mock_dcgm.NewMockClient(ctrl)
			tc.setupMock(mockClient, &reconcileCalls, &getMetricsCalls)

			eng := newTestEngine(mockClient, outputPath, time.Hour)

			// Call reconcileAndCollect directly (no need to run the loop since we
			// are not exercising the ticker here).
			require.NoError(t, eng.reconcileAndCollect(context.Background()))

			// Verify GetMetrics was called (or not) based on the Reconcile outcome.
			if tc.expectGetMetrics {
				assert.GreaterOrEqual(t, getMetricsCalls.Load(), int64(1),
					"GetMetrics should have been called")
			} else {
				assert.Equal(t, int64(0), getMetricsCalls.Load(),
					"GetMetrics should not have been called")
			}

			// Verify Reconcile was always called.
			assert.GreaterOrEqual(t, reconcileCalls.Load(), int64(1),
				"Reconcile should always be called")

			// The staging temp file must never linger, regardless of outcome.
			_, err := os.Stat(eng.tempPath())
			assert.True(t, os.IsNotExist(err), "temp file should not remain after reconcileAndCollect")

			out := readOutput(t, outputPath)
			if tc.wantFreshWrite {
				// A fresh snapshot overwrote the seed: the sentinel timestamp is gone.
				assert.NotEqual(t, sentinelTimestamp, out.Timestamp,
					"reconcileAndCollect should overwrite the seeded snapshot with a fresh timestamp")
				assert.NotEmpty(t, out.Timestamp, "fresh snapshot should carry a timestamp")
				if tc.verify != nil {
					tc.verify(t, out)
				}
			} else {
				// The file was left untouched: the seeded snapshot survives verbatim.
				assert.Equal(t, sentinelTimestamp, out.Timestamp,
					"reconcileAndCollect should not rewrite the file when reconciliation fails")
			}
		})
	}
}

func TestEngine_PeriodicCollection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		testFunc func(
			t *testing.T,
			eng *Engine,
			mockClient *mock_dcgm.MockClient,
			reconcileCalls *atomic.Int64,
			getMetricsCalls *atomic.Int64,
			cancel context.CancelFunc,
		)
	}{
		{
			name: "ticker triggers collection",
			testFunc: func(
				t *testing.T,
				eng *Engine,
				mockClient *mock_dcgm.MockClient,
				reconcileCalls *atomic.Int64,
				getMetricsCalls *atomic.Int64,
				cancel context.CancelFunc,
			) {
				// Wait for at least one collection tick.
				assert.Eventually(t, func() bool {
					return getMetricsCalls.Load() >= 1
				}, 200*time.Millisecond, 5*time.Millisecond,
					"Expected at least one GetMetrics call from ticker")
			},
		},
		{
			name: "context cancellation stops the loop",
			testFunc: func(
				t *testing.T,
				eng *Engine,
				mockClient *mock_dcgm.MockClient,
				reconcileCalls *atomic.Int64,
				getMetricsCalls *atomic.Int64,
				cancel context.CancelFunc,
			) {
				// Wait for at least one tick to confirm the loop is running.
				assert.Eventually(t, func() bool {
					return reconcileCalls.Load() >= 1
				}, 200*time.Millisecond, 5*time.Millisecond,
					"Expected at least one Reconcile call")

				// Cancel the context to stop the loop.
				cancel()

				// Record the call count after cancellation.
				time.Sleep(50 * time.Millisecond)
				countAfterCancel := reconcileCalls.Load()

				// Verify no more calls happen after cancellation.
				time.Sleep(50 * time.Millisecond)
				assert.Equal(t, countAfterCancel, reconcileCalls.Load(),
					"No more Reconcile calls should happen after context cancellation")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			outputPath := filepath.Join(t.TempDir(), "gpu-metrics.json")

			var reconcileCalls, getMetricsCalls atomic.Int64
			mockClient := mock_dcgm.NewMockClient(ctrl)
			mockClient.EXPECT().Reconcile(gomock.Any()).DoAndReturn(func(context.Context) (bool, error) {
				reconcileCalls.Add(1)
				return true, nil
			}).AnyTimes()
			mockClient.EXPECT().GetMetrics(gomock.Any()).DoAndReturn(func(context.Context) ([]gputypes.GPUMetric, error) {
				getMetricsCalls.Add(1)
				return []gputypes.GPUMetric{{GPUUUID: "GPU-tick-1"}}, nil
			}).AnyTimes()
			expectStatus(mockClient, true, "", false)

			// Short interval so the ticker fires many times within the test window.
			eng := newTestEngine(mockClient, outputPath, 5*time.Millisecond)

			done := make(chan error, 1)
			go func() { done <- eng.run(ctx) }()

			tc.testFunc(t, eng, mockClient, &reconcileCalls, &getMetricsCalls, cancel)

			// Cancel (idempotent) and confirm the loop returns cleanly.
			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err, "run() should return nil when the context is cancelled")
			case <-time.After(2 * time.Second):
				t.Fatal("run() did not return after context cancellation")
			}
		})
	}
}

// TestStartFilePreconditions verifies Start()'s up-front file checks: the
// metrics file and its temp file are created on demand, so Start() fails fast
// (before the loop and before touching the client) only when a file exists but
// is unwritable, and otherwise proceeds into the loop (which we stop via SIGTERM).
func TestStartFilePreconditions(t *testing.T) {
	testCases := []struct {
		name string
		// seedOutput/seedTemp report which of the two files to pre-create; a file
		// left un-seeded is missing and should be created by Start().
		seedOutput bool
		seedTemp   bool
		// unwritable names the seeded file ("output" or "temp") to strip write
		// permission from, or "" to leave both writable.
		unwritable string
		// wantErr is true when the precondition check should fail Start();
		// errContains is a substring the returned error must contain.
		wantErr     bool
		errContains string
	}{
		// A missing file is created on demand, so absence is never fatal.
		{name: "both files created when missing", seedOutput: false, seedTemp: false, wantErr: false},
		{name: "temp file created when missing", seedOutput: true, seedTemp: false, wantErr: false},
		{name: "both files exist and writable", seedOutput: true, seedTemp: true, wantErr: false},
		// An existing but unwritable file is fatal — every collection would fail
		// to write it.
		{name: "metrics file not writable", seedOutput: true, seedTemp: true, unwritable: "output", wantErr: true, errContains: "cannot create or write"},
		{name: "temp file not writable", seedOutput: true, seedTemp: true, unwritable: "temp", wantErr: true, errContains: "cannot create or write"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Root bypasses permission enforcement, so a read-only file is still
			// writable; skip cases that expect an error from a write denial.
			if tc.unwritable != "" && tc.wantErr && os.Geteuid() == 0 {
				t.Skip("write-permission checks are bypassed when running as root")
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			outputPath := filepath.Join(t.TempDir(), "gpu-metrics.json")
			tempPath := outputPath + ".tmp"
			if tc.seedOutput {
				require.NoError(t, os.WriteFile(outputPath, nil, metricsFilePermission))
			}
			if tc.seedTemp {
				require.NoError(t, os.WriteFile(tempPath, nil, metricsFilePermission))
			}
			switch tc.unwritable {
			case "output":
				require.NoError(t, os.Chmod(outputPath, 0400))
			case "temp":
				require.NoError(t, os.Chmod(tempPath, 0400))
			}

			mockClient := mock_dcgm.NewMockClient(ctrl)

			if tc.wantErr {
				// On the failure path the client must never be touched: no
				// expectations are set, so any Reconcile/GetMetrics/Shutdown call
				// fails the test.
				eng := newTestEngine(mockClient, outputPath, time.Hour)

				err := eng.Start()
				require.Error(t, err, "Start() should fail when a required file precondition is not met")
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			// On the success path the preconditions pass and Start() enters the
			// collection loop; wire up the client and stop the loop with SIGTERM.
			var getMetricsCalls atomic.Int64
			mockClient.EXPECT().Reconcile(gomock.Any()).Return(true, nil).AnyTimes()
			mockClient.EXPECT().GetMetrics(gomock.Any()).DoAndReturn(func(context.Context) ([]gputypes.GPUMetric, error) {
				getMetricsCalls.Add(1)
				return []gputypes.GPUMetric{{GPUUUID: "GPU-start-001"}}, nil
			}).AnyTimes()
			expectStatus(mockClient, true, "", false)
			mockClient.EXPECT().Shutdown().Return(nil).Times(1)

			eng := newTestEngine(mockClient, outputPath, 5*time.Millisecond)

			// Buffered so Start()'s final send never blocks, even with no reader.
			done := make(chan error, 1)
			go func() { done <- eng.Start() }()

			// Join the Start() goroutine before the deferred ctrl.Finish() (defers
			// are LIFO, and ctrl.Finish() was deferred earlier). On the t.Fatal
			// timeout path this waits up to 2s for Start() to exit so it stops
			// touching the mock / t.TempDir before teardown; the wait is bounded so
			// a hung Start() is reported via t.Error rather than blocking forever.
			// On the happy path startReturned is set, so this is a no-op.
			startReturned := false
			defer func() {
				if startReturned {
					return
				}
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("Start() goroutine leaked: did not exit after SIGTERM")
				}
			}()

			// Wait for a real collection tick (which also guarantees the SIGTERM
			// handler is installed) before signalling.
			require.Eventually(t, func() bool {
				return getMetricsCalls.Load() >= 1
			}, 2*time.Second, 5*time.Millisecond, "Start() should begin collecting when preconditions pass")

			require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

			select {
			case err := <-done:
				startReturned = true
				assert.NoError(t, err, "Start() should return nil after SIGTERM cancels the run loop")
			case <-time.After(2 * time.Second):
				t.Fatal("Start() did not return after SIGTERM was delivered")
			}

			// The collection ran end-to-end and the temp file was renamed away.
			out := readOutput(t, outputPath)
			require.Len(t, out.GPUs, 1)
			assert.Equal(t, "GPU-start-001", out.GPUs[0].GPUUUID)
			_, err := os.Stat(tempPath)
			assert.True(t, os.IsNotExist(err), "temp file should not remain after the atomic rename")
		})
	}
}
