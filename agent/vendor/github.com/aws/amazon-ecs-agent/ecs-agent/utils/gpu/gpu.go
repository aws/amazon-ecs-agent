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
}

// ShouldAdvertiseMpsCapability reports whether ecs.capability.gpu-sharing-mps
// should be advertised.
func ShouldAdvertiseMpsCapability(in MpsCapabilityInputs) (advertise bool, conditions map[MpsGateCondition]bool) {
	conditions = map[MpsGateCondition]bool{
		GPUPresent:        in.GPUPresent,
		MpsBinaryPresent:  in.MpsBinaryPresent,
		MpsServiceEnabled: in.MpsServiceEnabled,
		NotVGPU:           !in.IsVGPU,
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
	order := []MpsGateCondition{GPUPresent, MpsBinaryPresent, MpsServiceEnabled, NotVGPU}
	var unmet []MpsGateCondition
	for _, c := range order {
		if !conditions[c] {
			unmet = append(unmet, c)
		}
	}
	return unmet
}
