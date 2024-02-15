//go:build !linux && !windows
// +build !linux,!windows

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
	"errors"

	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/cihub/seelog"
)

func (agent *ecsAgent) initializeTaskENIDependencies(state dockerstate.TaskEngineState, taskEngine engine.TaskEngine) (error, bool) {
	return errors.New("unsupported platform"), true
}

// startWindowsService is not supported on non windows platforms
func (agent *ecsAgent) startWindowsService() int {
	seelog.Error("Windows Services are not supported on unspecified platforms")
	return 1
}

func (agent *ecsAgent) initializeResourceFields(credentialsManager credentials.Manager) {
}

func (agent *ecsAgent) cgroupInit() error {
	return nil
}

func (agent *ecsAgent) initializeGPUManager() error {
	return nil
}

func (agent *ecsAgent) getPlatformDevices() []*ecs.PlatformDevice {
	return nil
}

func (agent *ecsAgent) getDevicePlugins() []*ecs.PlatformDevice {
	return nil
}

func (agent *ecsAgent) loadPauseContainer() error {
	return nil
}
