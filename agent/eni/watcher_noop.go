// +build !linux

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

// Package eni is a culmination of interfaces, structs and methods that are
// placeholders for build purposes on unsupported platforms
package eni

import (
	"context"
	"fmt"
	"runtime"

	"github.com/aws/amazon-ecs-agent/agent/eni/udevwrapper"
)

// UdevWatcher is a placeholders for build purposes in unsupported platforms
type UdevWatcher struct{}

// New is a placeholder for build purposes on unsupported platforms
func New(ctx context.Context, udevwrap udevwrapper.Udev) *UdevWatcher {
	return &UdevWatcher{}
}

// Init is a placeholder for build purposes on unsupported platforms
func (udevWatcher *UdevWatcher) Init() error {
	return fmt.Errorf("Udev watcher init: unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

// Start is a placeholder for build purposes on unsupported platforms
func (udevWatcher *UdevWatcher) Start() {
	return
}

// Stop is a placeholder for build purposes on unsupported platforms
func (udevWatcher *UdevWatcher) Stop() {
	return
}
