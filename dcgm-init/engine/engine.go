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
	// DefaultInitErrorExitCode is used for recoverable init errors (retried by
	// Restart=on-failure).
	DefaultInitErrorExitCode = -1
)

const (
	// metricsFilePermission is the permission for the metrics file and its
	// staging temp file. 0644 keeps it world-readable so the agent can consume
	// it, while only dcgm-init writes it.
	metricsFilePermission os.FileMode = 0644

	// metricsDirPermission is the permission for the metrics file's directory.
	// 0755 keeps it world-readable/traversable so the agent can reach the file,
	// while only dcgm-init writes into it.
	metricsDirPermission os.FileMode = 0755

	// metricsFileTempSuffix is appended to outputPath to form the staging file.
	// Suffixing outputPath (rather than a fixed path) keeps the temp and final
	// files in the same directory, so os.Rename stays atomic (same filesystem)
	// even when tests redirect outputPath to a temp dir.
	metricsFileTempSuffix = ".tmp"

	// metricsCollectionInterval is how often GPU metrics are collected and
	// written. The agent samples the file at roughly this cadence, so writing
	// more frequently would not surface additional data downstream.
	metricsCollectionInterval = 60 * time.Second
)

// Engine drives the dcgm-init metrics collection loop: it collects GPU metrics
// via the dcgm.Client and writes them to a shared JSON file the agent reads.
//
// outputPath and collectionInterval default to the package constants in New();
// tests override them to redirect writes to a temp dir and shrink the ticker.
type Engine struct {
	client             dcgm.Client
	outputPath         string
	collectionInterval time.Duration
}

// New creates an Engine. The DCGM client connects lazily on the first Reconcile,
// so New is cheap and succeeds even when nv-hostengine is not yet up.
func New() (*Engine, error) {
	return &Engine{
		client:             dcgm.NewClient(dcgm.Config{}),
		outputPath:         gputypes.GPUMetricsFilePath,
		collectionInterval: metricsCollectionInterval,
	}, nil
}

// tempPath is the staging file that reconcileAndCollect writes before atomically
// renaming it onto outputPath.
func (e *Engine) tempPath() string {
	return e.outputPath + metricsFileTempSuffix
}

// Start creates the metrics dir and files on demand and runs the collection
// loop until SIGTERM (systemd stop sends SIGTERM).
func (e *Engine) Start() error {
	// Create the parent dir on demand (MkdirAll is a no-op if it exists).
	outputDir := filepath.Dir(e.outputPath)
	if err := os.MkdirAll(outputDir, metricsDirPermission); err != nil {
		return fmt.Errorf("dcgm-init cannot create metrics directory %s: %w", outputDir, err)
	}

	// Fail fast if either file can't be written, rather than spin a loop whose
	// writes fail every tick. Both are created on demand, so only an existing
	// unwritable file (or unwritable directory) is fatal.
	for _, path := range []string{e.outputPath, e.tempPath()} {
		if err := ensureCreatable(path); err != nil {
			return fmt.Errorf("dcgm-init cannot create or write metrics file %s: %w", path, err)
		}
	}

	// Shutdown is signal-driven (systemd's default stop sends SIGTERM); there is
	// no separate "stop" command. NotifyContext cancels ctx on SIGTERM so the
	// run loop unwinds cleanly, and stop removes the handler on return.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	// Release DCGM/nv-hostengine resources on the way out.
	defer func() {
		if err := e.client.Shutdown(); err != nil {
			logger.Warn("dcgm-init failed to shut down DCGM client cleanly", logger.Fields{"error": err})
		}
	}()

	e.run(ctx)
	return nil
}

// run collects metrics on every tick of collectionInterval until the context
// is cancelled.
func (e *Engine) run(ctx context.Context) {
	ticker := time.NewTicker(e.collectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("dcgm-init is shutting down metrics collection")
			return
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
	// ConnectionLost indicates the DCGM/nv-hostengine connection is lost. When
	// true the reader should report INSUFFICIENT_DATA rather than trust Healthy:
	// IsHealthy() only flips to false on a known violation, so it stays true
	// while disconnected.
	ConnectionLost bool                 `json:"connection_lost,omitempty"`
	GPUs           []gputypes.GPUMetric `json:"gpus"`
}

// reconcileAndCollect reconciles the DCGM connection, collects metrics, and
// writes them to the output file atomically (staged to a temp file, then
// renamed) so the agent never sees a partial write.
//
// A reconcile or collect failure is non-fatal: a status-only snapshot (no
// per-GPU metrics) is still written so the file stays fresh. Only a marshal or
// write/rename failure is returned.
func (e *Engine) reconcileAndCollect(ctx context.Context) error {
	metrics := []gputypes.GPUMetric{}
	if _, err := e.client.Reconcile(ctx); err != nil {
		logger.Warn("dcgm-init DCGM reconciliation failed, skipping metrics collection", logger.Fields{"error": err})
	} else {
		collected, err := e.client.GetMetrics(ctx)
		if err != nil {
			logger.Warn("dcgm-init failed to collect GPU metrics, writing status only", logger.Fields{"error": err})
		} else {
			metrics = collected
		}
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
