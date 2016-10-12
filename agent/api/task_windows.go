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

import "strings"

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
			hostVol.FSSourcePath = strings.ToLower(hostVol.FSSourcePath)
		}
	}
	for _, container := range task.Containers {
		for _, mountPoint := range container.MountPoints {
			mountPoint.ContainerPath = strings.ToLower(mountPoint.ContainerPath)
		}
	}
}

func getCanonicalPath(path string) string {
	return strings.ToLower(path)
}
