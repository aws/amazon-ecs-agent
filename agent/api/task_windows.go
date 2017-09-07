// +build windows

// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"path/filepath"
	"strings"

	"github.com/fsouza/go-dockerclient"
)

const (
	//memorySwappinessDefault is the expected default value for this platform
	memorySwappinessDefault = -1
)

// adjustForPlatform makes Windows-specific changes to the task after unmarshal
func (task *Task) adjustForPlatform() {
	task.downcaseAllVolumePaths()
}

// downcaseAllVolumePaths forces all volume paths (host path and container path)
// to be lower-case.  This is to account for Windows case-insensitivity and the
// case-sensitive string comparison that takes place elsewhere in the code.
func (task *Task) downcaseAllVolumePaths() {
	for _, volume := range task.Volumes {
		if hostVol, ok := volume.Volume.(*FSHostVolume); ok {
			hostVol.FSSourcePath = getCanonicalPath(hostVol.FSSourcePath)
		}
	}
	for _, container := range task.Containers {
		for i, mountPoint := range container.MountPoints {
			// container.MountPoints is a slice of values, not a slice of pointers so
			// we need to mutate the actual value instead of the copied value
			container.MountPoints[i].ContainerPath = getCanonicalPath(mountPoint.ContainerPath)
		}
	}
}

func getCanonicalPath(path string) string {
	return filepath.Clean(strings.ToLower(path))
}

// platformHostConfigOverride provides an entry point to set up default HostConfig options to be
// passed to Docker API.
func (task *Task) platformHostConfigOverride(hostConfig *docker.HostConfig) {
	task.overrideDefaultMemorySwappiness(hostConfig);
}

// overrideDefaultMemorySwappiness Overrides the value of MemorySwappiness to -1
// Version 1.12.x of Docker for Windows would ignore the unsupported option MemorySwappiness.
// Version 17.03.x will cause an error if any value other than -1 is passed in for MemorySwappiness.
// This bug is not noticed when no value is passed in. However, the go-dockerclient client version
// we are using removed the json option omitempty causing this parameter to default to 0 if empty.
// https://github.com/fsouza/go-dockerclient/commit/72342f96fabfa614a94b6ca57d987eccb8a836bf
func (task *Task) overrideDefaultMemorySwappiness(hostConfig *docker.HostConfig) {
	hostConfig.MemorySwappiness = memorySwappinessDefault
}