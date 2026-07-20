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

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// allMet returns inputs where every condition is satisfied. This is the only
// combination that should advertise the capability.
func allMet() MpsCapabilityInputs {
	return MpsCapabilityInputs{
		GPUPresent:        true,
		MpsBinaryPresent:  true,
		MpsServiceEnabled: true,
		IsVGPU:            false, // not a vGPU -> the NotVGPU condition is satisfied
	}
}

func TestAllConditionsMetAdvertises(t *testing.T) {
	advertise, conditions := ShouldAdvertiseMpsCapability(allMet())
	assert.True(t, advertise)
	assert.Empty(t, UnmetMpsConditions(conditions))
}

// TestShouldAdvertiseTruthTable exercises all 16 combinations of the four
// boolean inputs and checks that the capability is advertised only when every
// condition holds.
func TestShouldAdvertiseTruthTable(t *testing.T) {
	for i := 0; i < 16; i++ {
		in := MpsCapabilityInputs{
			GPUPresent:        i&1 != 0,
			MpsBinaryPresent:  i&2 != 0,
			MpsServiceEnabled: i&4 != 0,
			IsVGPU:            i&8 != 0,
		}
		want := in.GPUPresent && in.MpsBinaryPresent && in.MpsServiceEnabled && !in.IsVGPU
		advertise, _ := ShouldAdvertiseMpsCapability(in)
		assert.Equalf(t, want, advertise, "inputs=%+v", in)
	}
}

// TestSingleFailingConditionWithholds flips one condition false at a time and
// checks the capability is withheld and that exact condition is the only one
// reported unmet.
func TestSingleFailingConditionWithholds(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*MpsCapabilityInputs)
		expect MpsGateCondition
	}{
		{"no gpu", func(in *MpsCapabilityInputs) { in.GPUPresent = false }, GPUPresent},
		{"no binary", func(in *MpsCapabilityInputs) { in.MpsBinaryPresent = false }, MpsBinaryPresent},
		{"service disabled", func(in *MpsCapabilityInputs) { in.MpsServiceEnabled = false }, MpsServiceEnabled},
		{"is vgpu", func(in *MpsCapabilityInputs) { in.IsVGPU = true }, NotVGPU},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := allMet()
			tc.mutate(&in)
			advertise, conditions := ShouldAdvertiseMpsCapability(in)
			assert.False(t, advertise)
			assert.Equal(t, []MpsGateCondition{tc.expect}, UnmetMpsConditions(conditions))
		})
	}
}

// TestUnmetConditionsStableOrder checks that multiple failures are all returned
// and always in the fixed order, regardless of Go's random map iteration.
func TestUnmetConditionsStableOrder(t *testing.T) {
	in := MpsCapabilityInputs{} // all false; IsVGPU false means NotVGPU is satisfied
	advertise, conditions := ShouldAdvertiseMpsCapability(in)
	assert.False(t, advertise)
	assert.Equal(t,
		[]MpsGateCondition{GPUPresent, MpsBinaryPresent, MpsServiceEnabled},
		UnmetMpsConditions(conditions))
}

// TestVGPUFailOpen checks the vGPU gate only withholds on a definite vGPU
// result. An errored or unknown probe leaves IsVGPU false, which must still
// advertise so a flaky probe does not strip MPS from a real GPU.
func TestVGPUFailOpen(t *testing.T) {
	in := allMet()
	in.IsVGPU = true
	advertise, _ := ShouldAdvertiseMpsCapability(in)
	assert.False(t, advertise)

	in.IsVGPU = false
	advertise, _ = ShouldAdvertiseMpsCapability(in)
	assert.True(t, advertise)
}

func TestUnmetReason(t *testing.T) {
	assert.Equal(t, "no NVIDIA GPU present", GPUPresent.UnmetReason())
	assert.Equal(t, "MPS control binary not present on host", MpsBinaryPresent.UnmetReason())
	assert.Equal(t, "nvidia-mps systemd service is not enabled on host", MpsServiceEnabled.UnmetReason())
	assert.Equal(t, "instance has an NVIDIA vGPU slice, where we do not support MPS", NotVGPU.UnmetReason())
	// Unknown condition falls back to its raw string.
	assert.Equal(t, "some-unknown", MpsGateCondition("some-unknown").UnmetReason())
}
