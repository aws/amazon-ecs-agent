//go:build linux
// +build linux

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
	agentdoctor "github.com/aws/amazon-ecs-agent/agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/gpu"
)

// appendMpsDaemonHealthcheck registers the MPS control-daemon health check on exactly
// the instances that advertise ecs.capability.gpu-sharing-mps, reusing the same
// predicate so the instance health signal and the capability agree on what
// MPS-capable means. A plain GPU box with no nvidia-mps.service would otherwise fail
// every probe and report a false ACCELERATED_COMPUTE=IMPAIRED.
func (agent *ecsAgent) appendMpsDaemonHealthcheck(list []doctor.Healthcheck) []doctor.Healthcheck {
	inputs, ok := agent.mpsCapabilityInputs()
	if !ok {
		return list
	}
	if advertise, _ := gpu.ShouldAdvertiseMpsCapability(inputs); !advertise {
		return list
	}
	return append(list, agentdoctor.NewMpsDaemonHealthcheck())
}
