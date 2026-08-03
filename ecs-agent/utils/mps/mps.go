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

import "fmt"

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
)

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
