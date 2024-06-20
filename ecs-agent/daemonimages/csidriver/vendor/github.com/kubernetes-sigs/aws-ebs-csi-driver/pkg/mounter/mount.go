/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package mounter implements OS-specific functionality for interacting with mounts.
//
// The package should any implementation of mount related functionality that is not portable across platforms.
package mounter

import (
	mountutils "k8s.io/mount-utils"
)

// Mounter is the interface implemented by NodeMounter.
// A mix & match of functions defined in upstream libraries. (FormatAndMount
// from struct SafeFormatAndMount, PathExists from an old edition of
// mount.Interface). Define it explicitly so that it can be mocked and to
// insulate from oft-changing upstream interfaces/structs
type Mounter interface {
	mountutils.Interface

	FormatAndMountSensitiveWithFormatOptions(source string, target string, fstype string, options []string, sensitiveOptions []string, formatOptions []string) error
	IsCorruptedMnt(err error) bool
	GetDeviceNameFromMount(mountPath string) (string, int, error)
	MakeFile(path string) error
	MakeDir(path string) error
	PathExists(path string) (bool, error)
	NeedResize(devicePath string, deviceMountPath string) (bool, error)
	Unpublish(path string) error
	Unstage(path string) error
	Resize(devicePath, deviceMountPath string) (bool, error)
	FindDevicePath(devicePath, volumeID, partition, region string) (string, error)
	PreparePublishTarget(target string) error
	IsBlockDevice(fullPath string) (bool, error)
	GetBlockSizeBytes(devicePath string) (int64, error)
}

// NodeMounter implements Mounter.
// A superstruct of SafeFormatAndMount.
type NodeMounter struct {
	*mountutils.SafeFormatAndMount
}

// NewNodeMounter returns a new intsance of NodeMounter.
func NewNodeMounter(hostprocess bool) (Mounter, error) {
	var safeMounter *mountutils.SafeFormatAndMount
	var err error

	if hostprocess {
		safeMounter, err = NewSafeMounterV2()
	} else {
		safeMounter, err = NewSafeMounter()
	}

	if err != nil {
		return nil, err
	}
	return &NodeMounter{safeMounter}, nil
}
