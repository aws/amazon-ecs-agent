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

package engine

// SetupPlatformResources sets up platform level resources
func (mtask *managedTask) SetupPlatformResources() error {
	if mtask.Task.CgroupEnabled() {
		return mtask.setupCgroup()
	}
	return nil
}

// CleanupPlatformResources cleans up platform level resources
func (mtask *managedTask) CleanupPlatformResources() error {
	if mtask.Task.CgroupEnabled() {
		return mtask.cleanupCgroup()
	}
	return nil
}

// setupCgroup sets up the cgroup for each managed task
func (mtask *managedTask) setupCgroup() error {
	// Stub for now
	return nil
}

// cleanupCgroup removes the task cgroup
func (mtask *managedTask) cleanupCgroup() error {
	// Stub for now
	return nil
}
