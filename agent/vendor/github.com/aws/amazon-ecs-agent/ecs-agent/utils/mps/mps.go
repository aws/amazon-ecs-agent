// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package mps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/execwrapper"
)

const (
	// PipeDirectory is the MPS control daemon's Unix domain socket directory. It
	// must match what the systemd nvidia-mps.service exposes on the host and is
	// bind mounted into each MPS container so the CUDA runtime can reach the
	// daemon.
	PipeDirectory = "/tmp/nvidia-mps"

	// PipeDirectoryEnvVar tells the CUDA runtime where to find the MPS pipes.
	PipeDirectoryEnvVar = "CUDA_MPS_PIPE_DIRECTORY"

	// PinnedDeviceMemLimitEnvVar is the per-client hard GPU memory ceiling,
	// enforced by the MPS server in software (zero GPU memory overhead).
	PinnedDeviceMemLimitEnvVar = "CUDA_MPS_PINNED_DEVICE_MEM_LIMIT"

	// ActiveThreadPercentageEnvVar is the per-client SM (compute) ceiling.
	ActiveThreadPercentageEnvVar = "CUDA_MPS_ACTIVE_THREAD_PERCENTAGE"

	// inContainerDeviceIndex is the device index the memory limit is keyed on.
	inContainerDeviceIndex = 0

	// ControlBinary is the MPS control utility. Execing it with a control
	// command checks daemon readiness. It talks to the daemon socket under
	// CUDA_MPS_PIPE_DIRECTORY.
	ControlBinary = "/usr/bin/nvidia-cuda-mps-control"

	// ProbeCommand is the control command fed to the control utility. It answers
	// even before any MPS client exists, and the utility exit code reflects daemon
	// reachability regardless of the command, so it serves as a liveness check.
	ProbeCommand = "get_default_active_thread_percentage"

	// ProbeTimeout bounds the probe exec. A wedged daemon accepts the socket
	// connection but never replies, hanging the control utility. The context
	// timeout turns that hang into a failure.
	ProbeTimeout = 3 * time.Second
)

// ProbeResult captures what one health-probe exec observed.
type ProbeResult struct {
	// ExitCode is the control utility exit code. 0 means the daemon is serving. A
	// positive value is the utility's own nonzero exit, meaning the daemon is not
	// serving (e.g. 1 "Cannot find MPS control daemon process"). -1 means the
	// process could not be run or was killed by the context timeout (TimedOut is
	// true and Err is set).
	ExitCode int
	// Stdout is the trimmed combined stdout/stderr of the probe.
	Stdout string
	// Latency is the wall-clock time the probe exec took.
	Latency time.Duration
	// TimedOut is true when the context deadline fired.
	TimedOut bool
	// Err is non-nil when the daemon is not functionally serving.
	Err error
}

// ProbeControlDaemon reports whether the MPS control daemon is serving: it feeds
// command to the control utility on stdin and treats exit code 0 within
// ProbeTimeout as serving.
func ProbeControlDaemon(exec execwrapper.Exec, command string) ProbeResult {
	ctx, cancel := exec.NewExecContextWithTimeout(context.Background(), ProbeTimeout)
	defer cancel()

	// ControlBinary is a fixed constant path and no arguments are passed. command
	// is fed on stdin, not as an argument, and exec runs the binary directly (no
	// shell), so there is no shell-injection surface.
	// nosemgrep: command-injection-exec-variable
	cmd := exec.CommandContext(ctx, ControlBinary)
	cmd.SetIOStreams(strings.NewReader(command+"\n"), nil, nil)

	start := time.Now()
	out, err := cmd.CombinedOutput()

	result := ProbeResult{
		Stdout:  strings.TrimSpace(string(out)),
		Latency: time.Since(start),
	}
	if err == nil {
		return result
	}

	result.TimedOut = ctx.Err() == context.DeadlineExceeded
	if exitErr, ok := exec.ConvertToExitError(err); ok {
		result.ExitCode = exec.GetExitCode(exitErr)
	} else {
		result.ExitCode = -1
	}
	if result.TimedOut {
		result.Err = fmt.Errorf("mps control daemon probe timed out after %s (daemon wedged?): %w", ProbeTimeout, err)
	} else {
		result.Err = fmt.Errorf("mps control daemon probe failed (exit %d, output %q): %w", result.ExitCode, result.Stdout, err)
	}
	return result
}

// BuildEnv returns the MPS environment variables for a single MPS container.
func BuildEnv(memoryMiB uint, computePercent *uint) map[string]string {
	env := map[string]string{
		PipeDirectoryEnvVar:        PipeDirectory,
		PinnedDeviceMemLimitEnvVar: PinnedDeviceMemLimit(memoryMiB),
	}
	if computePercent != nil {
		env[ActiveThreadPercentageEnvVar] = fmt.Sprintf("%d", *computePercent)
	}
	return env
}

// PinnedDeviceMemLimit formats the per-client memory ceiling for the assigned
// (in-container index 0) GPU as MPS expects it: "0=<MiB>M".
func PinnedDeviceMemLimit(memoryMiB uint) string {
	return fmt.Sprintf("%d=%dM", inContainerDeviceIndex, memoryMiB)
}
