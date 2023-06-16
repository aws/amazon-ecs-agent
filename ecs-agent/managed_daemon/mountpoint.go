// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package manageddaemon

type MountPoint struct {
	// SourceVolumeID is used to identify the task volume globally, it's empty
	// when for internal mount points.
	SourceVolumeID string `json:"SourceVolumeID"`
	// SourceVolume is the name of the source volume, it's unique within the task.
	SourceVolume string `json:"SourceVolume,omitempty"`
	// SourceVolumeType is the type (EFS, EBS, or host) of the volume. EBS Volumes in particular are
	// treated differently within the microVM, and need special consideration in determining the microVM path.
	SourceVolumeType     string `json:"SourceVolumeType,omitempty"`
	SourceVolumeHostPath string `json:"SourceVolumeHostPath,omitempty"`
	ContainerPath        string `json:"ContainerPath,omitempty"`
	ReadOnly             bool   `json:"ReadOnly,omitempty"`
	Internal             bool   `json:"Internal,omitempty"`
}
