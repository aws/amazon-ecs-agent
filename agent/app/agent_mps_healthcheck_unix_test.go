//go:build linux && unit
// +build linux,unit

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

package app

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/gpu"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/stretchr/testify/assert"
)

// TestAppendMpsDaemonHealthcheck asserts that the MPS control-daemon health check is
// registered on exactly the instances that would advertise gpu-sharing-mps: it must be
// appended when ShouldAdvertiseMpsCapability is satisfied and withheld otherwise, so a
// plain GPU box never reports a false ACCELERATED_COMPUTE=IMPAIRED. It reuses the same
// NvidiaGPUManager setup as the capability test to keep the two decisions in lockstep.
func TestAppendMpsDaemonHealthcheck(t *testing.T) {
	// newMpsManager returns a GPU manager carrying the given MPS facts and one device
	// (with usable memory) unless gpuPresent is false. haveMemory controls whether that
	// device reports a memory value, exercising the AllGPUsHaveMemory gate.
	newMpsManager := func(gpuPresent, binary, service, vgpu, haveMemory bool) *gpu.NvidiaGPUManager {
		m := &gpu.NvidiaGPUManager{
			MpsControlBinaryPresent: binary,
			MpsServiceEnabled:       service,
			HasVGPU:                 vgpu,
		}
		if gpuPresent {
			m.SetGPUIDs([]string{"gpu-0"})
			if haveMemory {
				m.SetGPUMemoryMiB(map[string]uint64{"gpu-0": 22563})
			}
			m.SetDevices()
		}
		return m
	}

	cases := []struct {
		name     string
		mgr      *gpu.NvidiaGPUManager // nil means no GPU manager on the instance
		register bool
	}{
		{"all conditions met", newMpsManager(true, true, true, false, true), true},
		{"no gpu manager", nil, false},
		{"no gpu present", newMpsManager(false, true, true, false, true), false},
		{"mps binary absent", newMpsManager(true, false, true, false, true), false},
		{"mps service disabled", newMpsManager(true, true, false, false, true), false},
		{"is vgpu", newMpsManager(true, true, true, true, true), false},
		{"gpu present but no memory reported", newMpsManager(true, true, true, false, false), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// NvidiaGPUManager is an interface, so a typed-nil pointer would read as a
			// non-nil interface; leave the field unset to model "no GPU manager".
			rf := &taskresource.ResourceFields{}
			if tc.mgr != nil {
				rf.NvidiaGPUManager = tc.mgr
			}
			agent := &ecsAgent{resourceFields: rf}
			got := agent.appendMpsDaemonHealthcheck(nil)
			if tc.register {
				assert.Len(t, got, 1, "the MPS health check must be registered")
			} else {
				assert.Empty(t, got, "the MPS health check must not be registered")
			}
		})
	}
}
