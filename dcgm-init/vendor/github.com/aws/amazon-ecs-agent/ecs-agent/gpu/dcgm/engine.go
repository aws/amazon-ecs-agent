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

package dcgm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	gputypes "github.com/aws/amazon-ecs-agent/ecs-agent/gpu/types"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
)

const (
	// DefaultOutputPath is the shared file where dcgm-init writes GPU metrics
	// for the agent to consume. It must match gpu.DefaultGPUMetricsFilePath.
	DefaultOutputPath = "/var/run/ecs/gpu-metrics.json"

	// DefaultCollectionFreq is the default interval between metrics collections.
	DefaultCollectionFreq = 60 * time.Second
)

// Engine drives the dcgm-init metrics collection loop: it connects to DCGM via
// the Client, periodically collects GPU metrics, and writes them to a shared
// JSON file that the agent reads through gpu.DCGMHandler.
type Engine struct {
	client         Client
	outputPath     string
	collectionFreq time.Duration
	oneShot        bool
}

// NewEngine creates an Engine with the given configuration.
func NewEngine(socketPath string, outputPath string, collectionFreq time.Duration, oneShot bool) *Engine {
	config := Config{
		SocketPath:                socketPath,
		InitializationGracePeriod: DefaultInitializationGracePeriod,
	}

	return &Engine{
		client:         NewClient(config),
		outputPath:     outputPath,
		collectionFreq: collectionFreq,
		oneShot:        oneShot,
	}
}

// Start connects to DCGM and runs the metrics collection loop until
// a SIGTERM/SIGINT is received. This is the main entry point for the
// "start" command.
func (e *Engine) Start() error {
	defer e.client.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("dcgm-init received shutdown signal")
		cancel()
	}()

	outputDir := filepath.Dir(e.outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	return e.run(ctx)
}

// Stop is a no-op. The running dcgm-init process is stopped via SIGTERM
// from systemd, not by this command. This exists for symmetry with the
// systemd unit lifecycle.
func (e *Engine) Stop() error {
	return nil
}

func (e *Engine) run(ctx context.Context) error {
	reinitialized, err := e.client.Reconcile(ctx)
	if err != nil {
		return fmt.Errorf("initial DCGM reconciliation failed: %w", err)
	}
	logger.Info("DCGM client reconciled", logger.Fields{"reinitialized": reinitialized})

	if e.oneShot {
		return e.collectAndWrite(ctx)
	}

	ticker := time.NewTicker(e.collectionFreq)
	defer ticker.Stop()

	if err := e.collectAndWrite(ctx); err != nil {
		logger.Warn("dcgm-init initial collection failed, will retry", logger.Fields{"error": err})
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("dcgm-init is shutting down metrics collection")
			return nil
		case <-ticker.C:
			if _, err := e.client.Reconcile(ctx); err != nil {
				logger.Warn("dcgm-init client reconciliation failed", logger.Fields{"error": err})
				continue
			}
			if err := e.collectAndWrite(ctx); err != nil {
				logger.Warn("dcgm-init metrics collection failed", logger.Fields{"error": err})
			}
		}
	}
}

// metricsOutput is the JSON structure written to the shared metrics file.
// Its shape and tags must match gpu.GPUMetricsFileData, which the agent reads.
type metricsOutput struct {
	Timestamp       string               `json:"timestamp"`
	Healthy         bool                 `json:"healthy"`
	UnhealthyReason string               `json:"unhealthy_reason,omitempty"`
	GPUs            []gputypes.GPUMetric `json:"gpus"`
}

func (e *Engine) collectAndWrite(ctx context.Context) error {
	metrics, err := e.client.GetMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect GPU metrics: %w", err)
	}

	output := metricsOutput{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Healthy:         e.client.IsHealthy(),
		UnhealthyReason: e.client.UnhealthyReason(),
		GPUs:            metrics,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	// Write atomically: write to a staging file then rename onto the final
	// path so readers never observe a partially written file.
	stagingPath := e.outputPath + ".staging"
	if err := os.WriteFile(stagingPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metrics to %s: %w", stagingPath, err)
	}
	if err := os.Rename(stagingPath, e.outputPath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", stagingPath, e.outputPath, err)
	}

	logger.Info("metrics written", logger.Fields{"path": e.outputPath, "gpuCount": len(output.GPUs)})
	return nil
}
