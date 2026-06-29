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
	dockerdoctor "github.com/aws/amazon-ecs-agent/agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/doctor"
	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu"
)

// appendGPUHealthcheck adds the ACCELERATED_COMPUTE healthcheck to the list when
// GPU support is enabled. The healthcheck reads GPU health from the shared metrics
// file written by dcgm-init via gpu.DCGMHandler.
func (agent *ecsAgent) appendGPUHealthcheck(healthcheckList []doctor.Healthcheck) []doctor.Healthcheck {
	if agent.cfg.GPUSupportEnabled {
		gpuHandler := gpu.NewDCGMHandler("")
		gpuHealthCheck := dockerdoctor.NewGPUHealthcheck(gpuHandler)
		healthcheckList = append(healthcheckList, gpuHealthCheck)
	}
	return healthcheckList
}
