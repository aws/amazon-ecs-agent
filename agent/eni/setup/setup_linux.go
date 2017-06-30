// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package setup

import (
	"context"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	eniWatcher "github.com/aws/amazon-ecs-agent/agent/eni"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"
	"github.com/aws/amazon-ecs-agent/agent/statechange"

	log "github.com/cihub/seelog"
)

// New returns an udev watcher
func New(ctx context.Context, state dockerstate.TaskEngineState, stateChangeEvents chan<- statechange.Event) (*eniWatcher.UdevWatcher, error) {
	log.Debug("Setting up ENI Watcher")

	// Create UDev Monitor
	udevMonitor, err := udevwrapper.New()
	if err != nil {
		log.Errorf("Error creating udev monitor: %v", err)
		return nil, err
	}
	return initializeENIWatcher(ctx, udevMonitor, state, stateChangeEvents)
}

// initializeENIWatcher wraps up the setup for creating the ENI Watcher
func initializeENIWatcher(ctx context.Context, udevMonitor udevwrapper.Udev,
	state dockerstate.TaskEngineState, stateChangeEvents chan<- statechange.Event) (*eniWatcher.UdevWatcher, error) {
	// Create Watcher
	watcher := eniWatcher.New(ctx, udevMonitor, state, stateChangeEvents)
	if err := watcher.Init(); err != nil {
		log.Errorf("Error initializing ENI Watcher: %v", err)
		return nil, err
	}
	go watcher.Start()
	return watcher, nil
}
