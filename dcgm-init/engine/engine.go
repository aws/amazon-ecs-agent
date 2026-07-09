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

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgm"
	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

const (
	// DefaultInitErrorExitCode is the exit code used for general init errors.
	DefaultInitErrorExitCode = -1
)

const (
	MetricsFileDir = "/var/run/ecs"

	// MetricsFilePath is the shared file dcgm-init writes GPU metrics to and the
	// agent reads. Its parent directory is created on demand by Start(); the
	// file itself is created on the first collection tick.
	MetricsFilePath = MetricsFileDir + "/gpu-metrics.json"

	// metricsFilePermission is the permission for the metrics file and its
	// staging temp file. 0644 keeps the file world-readable so the agent can
	// consume it; only dcgm-init (running as root) writes it.
	metricsFilePermission os.FileMode = 0644

	// metricsDirPermission is the permission for the directory holding the
	// metrics file. 0755 keeps it world-readable/traversable so the agent can
	// reach the file; only dcgm-init (running as root) writes into it.
	metricsDirPermission os.FileMode = 0755

	// metricsFileTempSuffix is appended to outputPath to form the staging file
	// that reconcileAndCollect writes before atomically renaming it onto
	// outputPath. It stays a suffix on outputPath (rather than a fully qualified
	// path) so the temp and final files always share a directory: os.Rename is
	// only atomic within a single filesystem, and tests redirect outputPath to a
	// temp directory.
	metricsFileTempSuffix = ".tmp"

	// metricsCollectionInterval is how often GPU metrics are collected and
	// written. The agent samples the file at roughly this cadence, so writing
	// more frequently would not surface additional data downstream.
	metricsCollectionInterval = 60 * time.Second
)

// Engine drives the dcgm-init metrics collection loop: it connects to DCGM via
// the dcgm.Client, periodically collects GPU metrics, and writes them to a
// shared JSON file that the agent reads.
//
// outputPath and collectionInterval default to the package constants in New()
// and are only overridden in tests, so they can redirect writes to a temp
// directory and shrink the ticker without waiting a full production interval.
type Engine struct {
	client             dcgm.Client
	outputPath         string
	collectionInterval time.Duration
}

// New creates an instance of Engine. The DCGM client is created here but does
// not connect until the first Reconcile inside the collection loop, so New is
// cheap and does not fail on hosts where nv-hostengine is not yet up.
func New() (*Engine, error) {
	return &Engine{
		client:             dcgm.NewClient(dcgm.Config{}),
		outputPath:         MetricsFilePath,
		collectionInterval: metricsCollectionInterval,
	}, nil
}

// tempPath is the staging file that reconcileAndCollect writes before atomically
// renaming it onto outputPath.
func (e *Engine) tempPath() string {
	return e.outputPath + metricsFileTempSuffix
}

// Start runs the metrics collection loop until a SIGTERM/SIGINT is received
// (systemd's default stop sends SIGTERM). The parent directory, the metrics
// file, and its staging temp file are all created on demand if missing; Start
// only fails when a file already exists but cannot be written.
func (e *Engine) Start() error {
	// Create the parent directory on demand: as of recent changes it is no
	// longer guaranteed to exist before dcgm-init runs. MkdirAll is a no-op if
	// it already exists.
	outputDir := filepath.Dir(e.outputPath)
	if err := os.MkdirAll(outputDir, metricsDirPermission); err != nil {
		return fmt.Errorf("dcgm-init cannot create directory %s: %w", outputDir, err)
	}
	// Fail fast if either file can't be written, rather than spin a loop whose
	// writes fail every tick. Both are created on demand, so only an existing
	// unwritable file (or unwritable directory) is fatal.
	for _, path := range []string{e.outputPath, e.tempPath()} {
		if err := ensureCreatable(path); err != nil {
			return fmt.Errorf("dcgm-init cannot create or write metrics file %s: %w", path, err)
		}
	}

	// Cancel the context when a shutdown signal arrives so the run loop unwinds
	// cleanly. There is no separate "stop" command; shutdown is signal-driven
	// (systemd's default stop sends SIGTERM). NotifyContext installs the signal
	// handler and returns a context that is cancelled on SIGTERM; stop removes
	// the handler when we return.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	// Release DCGM/nv-hostengine resources on the way out.
	defer func() {
		if err := e.client.Shutdown(); err != nil {
			logger.Warn("dcgm-init failed to shut down DCGM client cleanly", logger.Fields{"error": err})
		}
	}()

	return e.run(ctx)
}

// run collects metrics on every tick of collectionInterval until the context
// is cancelled.
func (e *Engine) run(ctx context.Context) error {
	ticker := time.NewTicker(e.collectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("dcgm-init is shutting down metrics collection")
			return nil
		case <-ticker.C:
			if err := e.reconcileAndCollect(ctx); err != nil {
				logger.Warn("dcgm-init metrics collection failed", logger.Fields{"error": err})
			}
		}
	}
}

// dcgmOutput is the JSON structure written to the shared metrics file. Its
// shape and tags must match what the agent reads.
type dcgmOutput struct {
	Timestamp       string `json:"timestamp"`
	Healthy         bool   `json:"healthy"`
	UnhealthyReason string `json:"unhealthy_reason,omitempty"`
	// ConnectionLost indicates the DCGM/nv-hostengine connection is lost
	// (outside the grace period). When true, the reader cannot determine GPU
	// health and should report INSUFFICIENT_DATA rather than trusting Healthy:
	// IsHealthy() returns true when disconnected (it only flips to false on a
	// known violation/FAIL), so Healthy alone is not sufficient.
	ConnectionLost bool                 `json:"connection_lost,omitempty"`
	GPUs           []gputypes.GPUMetric `json:"gpus"`
}

// reconcileAndCollect reconciles the DCGM connection, collects the latest
// metrics, and writes them to the output file atomically (staged to the temp
// file, then renamed) so the agent never observes a partial write.
//
// A reconcile failure skips this cycle. A collect failure is not fatal: a
// status-only snapshot (health/connection fields, no per-GPU metrics) is still
// written so the file stays fresh. Only a marshal or write/rename failure is
// returned as an error.
func (e *Engine) reconcileAndCollect(ctx context.Context) error {
	if _, err := e.client.Reconcile(ctx); err != nil {
		logger.Warn("dcgm-init DCGM reconciliation failed, skipping metrics collection", logger.Fields{"error": err})
		return nil
	}

	metrics, err := e.client.GetMetrics(ctx)
	if err != nil {
		// Write a status-only snapshot: the health/connection fields still convey
		// state even with no per-GPU metrics.
		logger.Warn("dcgm-init failed to collect GPU metrics, writing status only", logger.Fields{"error": err})
		metrics = []gputypes.GPUMetric{}
	}

	output := dcgmOutput{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Healthy:         e.client.IsHealthy(),
		UnhealthyReason: e.client.UnhealthyReason(),
		ConnectionLost:  e.client.IsConnectionLost(),
		GPUs:            metrics,
	}
	logger.Debug("dcgm-init collected GPU metrics", logger.Fields{"path": e.outputPath, "gpuCount": len(output.GPUs)})

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("dcgm-init failed to marshal metrics: %w", err)
	}

	// Stage to the temp file, then atomically rename onto outputPath so a reader
	// never sees a partial write.
	tempPath := e.tempPath()
	if err := os.WriteFile(tempPath, data, metricsFilePermission); err != nil {
		return fmt.Errorf("dcgm-init failed to write metrics to %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, e.outputPath); err != nil {
		return fmt.Errorf("dcgm-init failed to rename %s to %s: %w", tempPath, e.outputPath, err)
	}

	logger.Debug("dcgm-init wrote GPU metrics")
	return nil
}

// ensureCreatable verifies path is writable, creating it if missing. It opens
// O_CREATE|O_WRONLY without O_TRUNC, so an existing file's contents are
// preserved and a missing parent directory or bad permissions surface as errors.
func ensureCreatable(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, metricsFilePermission)
	if err != nil {
		return err
	}
	return f.Close()
}
