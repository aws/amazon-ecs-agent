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

package volume

// Volume is an interface for something that may be used as the host half of a
// docker volume mount
type Volume interface {
	Source() string
	GetType() string
	GetVolumeId() string
	GetVolumeName() string
}

// FSHostVolume is a simple type of HostVolume which references an arbitrary
// location on the host as the Volume.
type FSHostVolume struct {
	FSSourcePath string `json:"sourcePath"`
}

// SourcePath returns the path on the host filesystem that should be mounted
func (fs *FSHostVolume) Source() string {
	return fs.FSSourcePath
}

// Currently not meant for use
func (fs *FSHostVolume) GetType() string {
	return FSHostVolumeType
}

// Currently not meant for use
func (fs *FSHostVolume) GetVolumeId() string {
	return ""
}

// Currently not meant for use
func (fs *FSHostVolume) GetVolumeName() string {
	return ""
}

// LocalDockerVolume represents a volume without a specified host path
// This is essentially DockerVolume with only the name specified; however,
// for backward compatibility we can't directly map to DockerVolume.
type LocalDockerVolume struct {
	HostPath string `json:"hostPath"`
}

// SourcePath returns the generated host path for the volume
func (e *LocalDockerVolume) Source() string {
	return e.HostPath
}

// Currently not meant for use
func (e *LocalDockerVolume) GetType() string {
	return DockerLocalVolumeDriver
}

// Currently not meant for use
func (e *LocalDockerVolume) GetVolumeId() string {
	return ""
}

// Currently not meant for use
func (e *LocalDockerVolume) GetVolumeName() string {
	return ""
}
