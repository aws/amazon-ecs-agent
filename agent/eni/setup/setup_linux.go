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

	eniWatcher "github.com/aws/amazon-ecs-agent/agent/eni"
	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"

	log "github.com/cihub/seelog"
)

// New returns an udev watcher
func New(ctx context.Context) (*eniWatcher.UdevWatcher, error) {
	log.Debug("Setting up ENI Watcher")

	// Create UDev Monitor
	udevMonitor, err := udevwrapper.New()
	if err != nil {
		log.Errorf("Error creating udev monitor: %v", err)
		return nil, err
	}
	return initializeENIWatcher(ctx, udevMonitor)
}

// initializeENIWatcher wraps up the setup for creating the ENI Watcher
func initializeENIWatcher(ctx context.Context, udevMonitor udevwrapper.Udev) (*eniWatcher.UdevWatcher, error) {
	// Create Watcher
	watcher := eniWatcher.New(ctx, udevMonitor)
	err := watcher.Init()
	if err != nil {
		log.Errorf("Error initializing ENI Watcher: %v", err)
		return nil, err
	}
	go watcher.Start()
	return watcher, nil
}
