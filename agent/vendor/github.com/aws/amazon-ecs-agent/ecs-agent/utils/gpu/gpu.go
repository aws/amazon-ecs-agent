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

// Package gpu holds the GPU-sharing capability logic that the agents
// share, so both decide and report the gpu-sharing-mps capability the same way.
package gpu

type MpsGateCondition string

const (
	GPUPresent        MpsGateCondition = "gpu-present"
	MpsBinaryPresent  MpsGateCondition = "mps-binary-present"
	MpsServiceEnabled MpsGateCondition = "mps-service-enabled"
	NotVGPU           MpsGateCondition = "not-vgpu"
	AllGPUsHaveMemory MpsGateCondition = "all-gpus-have-memory"
)

// UnmetReason returns the message to log when a condition is not satisfied.
// The wording lives here so agents report the same reason for the same failure.
func (c MpsGateCondition) UnmetReason() string {
	switch c {
	case GPUPresent:
		return "no NVIDIA GPU present"
	case MpsBinaryPresent:
		return "MPS control binary not present on host"
	case MpsServiceEnabled:
		return "nvidia-mps systemd service is not enabled on host"
	case NotVGPU:
		return "instance has an NVIDIA vGPU slice, where we do not support MPS"
	case AllGPUsHaveMemory:
		return "not every discovered GPU reported usable memory"
	}
	return string(c)
}

// MpsCapabilityInputs are the host facts the capability decision is made from.
type MpsCapabilityInputs struct {
	// GPUPresent is true when the NVIDIA driver and GPU device injection work.
	GPUPresent bool
	// MpsBinaryPresent is true when the MPS control binary exists on the host.
	MpsBinaryPresent bool
	// MpsServiceEnabled is true when the nvidia-mps systemd unit is enabled.
	// This is a static setup check, not daemon liveness.
	MpsServiceEnabled bool
	// IsVGPU must be set only on a definite NVML VGPU result. Leaving it false
	// on an errored or unknown probe keeps the gate fail-open, so a flaky probe
	// never strips MPS from a real passthrough GPU.
	IsVGPU bool
	// AllGPUsHaveMemory is true only when every discovered GPU has a usable-memory
	// value. MPS placement needs per-GPU memory for every GPU, so if any is missing
	// we withhold the capability. Memory detection is fail-open, so
	// a GPU whose memory could not be read is simply absent and flips this false.
	AllGPUsHaveMemory bool
}

// ShouldAdvertiseMpsCapability reports whether ecs.capability.gpu-sharing-mps
// should be advertised.
func ShouldAdvertiseMpsCapability(in MpsCapabilityInputs) (advertise bool, conditions map[MpsGateCondition]bool) {
	conditions = map[MpsGateCondition]bool{
		GPUPresent:        in.GPUPresent,
		MpsBinaryPresent:  in.MpsBinaryPresent,
		MpsServiceEnabled: in.MpsServiceEnabled,
		NotVGPU:           !in.IsVGPU,
		AllGPUsHaveMemory: in.AllGPUsHaveMemory,
	}
	advertise = true
	for _, satisfied := range conditions {
		if !satisfied {
			advertise = false
			break
		}
	}
	return advertise, conditions
}

// UnmetMpsConditions returns the unsatisfied conditions in a fixed order
func UnmetMpsConditions(conditions map[MpsGateCondition]bool) []MpsGateCondition {
	order := []MpsGateCondition{GPUPresent, MpsBinaryPresent, MpsServiceEnabled, NotVGPU, AllGPUsHaveMemory}
	var unmet []MpsGateCondition
	for _, c := range order {
		if !conditions[c] {
			unmet = append(unmet, c)
		}
	}
	return unmet
}

// AllGPUMemoryReported reports whether every discovered GPU UUID has a usable
// memory value greater than zero. A missing entry (memory detection is fail-open,
// so a GPU that could not be read is absent from the map) and an explicit zero
// both fail: a GPU with no usable memory cannot run MPS. An empty gpuIDs list
// returns false, since an MPS-capable instance must have at least one GPU with
// reported memory.
func AllGPUMemoryReported(gpuIDs []string, memoryMiB map[string]uint64) bool {
	if len(gpuIDs) == 0 {
		return false
	}
	for _, id := range gpuIDs {
		// A GPU is missing from the map when memory detection could not read it
		// (fail-open per device), and a zero value means it reported no usable
		// memory; either way the GPU cannot run MPS.
		if mem, ok := memoryMiB[id]; !ok || mem == 0 {
			return false
		}
	}
	return true
}
