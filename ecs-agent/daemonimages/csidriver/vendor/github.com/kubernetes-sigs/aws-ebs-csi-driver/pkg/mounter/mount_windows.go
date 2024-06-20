//go:build windows
// +build windows

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

package mounter

import (
	"fmt"

	"regexp"

	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/util"

	"k8s.io/klog/v2"
	mountutils "k8s.io/mount-utils"
)

var (
	ErrUnsupportedMounter = fmt.Errorf("unsupported mounter type")
)

func (m NodeMounter) FindDevicePath(devicePath, volumeID, _, _ string) (string, error) {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		return proxyMounter.FindDevicePath(devicePath, volumeID, "", "")
	case *CSIProxyMounter:
		return proxyMounter.FindDevicePath(devicePath, volumeID, "", "")
	default:
		return "", ErrUnsupportedMounter
	}
}

func (m NodeMounter) PreparePublishTarget(target string) error {
	// On Windows, Mount will create the parent of target and mklink (create a symbolic link) at target later, so don't create a
	// directory at target now. Otherwise mklink will error: "Cannot create a file when that file already exists".
	// Instead, delete the target if it already exists (like if it was created by kubelet <1.20)
	// https://github.com/kubernetes/kubernetes/pull/88759
	klog.V(4).InfoS("NodePublishVolume: removing dir", "target", target)
	exists, err := m.PathExists(target)
	if err != nil {
		return fmt.Errorf("error checking path %q exists: %v", target, err)
	}

	if !exists {
		return nil // If the target does not exist, no action is necessary
	}

	// Handle different mounter implementations
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		if err := proxyMounter.Rmdir(target); err != nil {
			return fmt.Errorf("error Rmdir target %q: %v", target, err)
		}
	case *CSIProxyMounter:
		if err := proxyMounter.Rmdir(target); err != nil {
			return fmt.Errorf("error Rmdir target %q: %v", target, err)
		}
	default:
		return ErrUnsupportedMounter
	}

	return nil
}

// IsBlockDevice checks if the given path is a block device
func (m NodeMounter) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}

// getBlockSizeBytes gets the size of the disk in bytes
func (m NodeMounter) GetBlockSizeBytes(devicePath string) (int64, error) {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		sizeInBytes, err := proxyMounter.GetDeviceSize(devicePath)
		if err != nil {
			return -1, err
		}
		return sizeInBytes, nil
	case *CSIProxyMounter:
		sizeInBytes, err := proxyMounter.GetDeviceSize(devicePath)
		if err != nil {
			return -1, err
		}
		return sizeInBytes, nil
	default:
		return -1, ErrUnsupportedMounter
	}
}

func (m NodeMounter) FormatAndMountSensitiveWithFormatOptions(source string, target string, fstype string, options []string, sensitiveOptions []string, formatOptions []string) error {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		return proxyMounter.FormatAndMountSensitiveWithFormatOptions(source, target, fstype, options, sensitiveOptions, formatOptions)
	case *CSIProxyMounter:
		return proxyMounter.FormatAndMountSensitiveWithFormatOptions(source, target, fstype, options, sensitiveOptions, formatOptions)
	default:
		return ErrUnsupportedMounter
	}
}

// GetDeviceNameFromMount returns the volume ID for a mount path.
// The ref count returned is always 1 or 0 because csi-proxy doesn't provide a
// way to determine the actual ref count (as opposed to Linux where the mount
// table gets read). In practice this shouldn't matter, as in the NodeStage
// case the ref count is ignored and in the NodeUnstage case, the ref count
// being >1 is just a warning.
// Command to determine ref count would be something like:
// Get-Volume -UniqueId "\\?\Volume{7c3da0c1-0000-0000-0000-010000000000}\" | Get-Partition | Select AccessPaths
func (m NodeMounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	// HACK change csi-proxy behavior instead of relying on fragile internal
	// implementation details!
	// if err contains '"(Get-Item...).Target, output: , error: <nil>' then the
	// internal Get-Item cmdlet didn't fail but no item/device was found at the
	// path so we should return empty string and nil error just like the Linux
	// implementation would.
	case *CSIProxyMounterV2:
		deviceName, err := proxyMounter.GetDeviceNameFromMount(mountPath, "")
		if err != nil {
			return handleGetDeviceNameFromMountError(err)
		}
		return deviceName, 1, nil
	case *CSIProxyMounter:
		deviceName, err := proxyMounter.GetDeviceNameFromMount(mountPath, "")
		if err != nil {
			return handleGetDeviceNameFromMountError(err)
		}
		return deviceName, 1, nil
	default:
		return "", 0, ErrUnsupportedMounter
	}
}

// handleGetDeviceNameFromMountError processes common error patterns for GetDeviceNameFromMount
func handleGetDeviceNameFromMountError(err error) (string, int, error) {
	// Handling common error pattern as seen in previous implementations
	pattern := `(Get-Item -Path \S+).Target, output: , error: <nil>|because it does not exist`
	matched, matchErr := regexp.MatchString(pattern, err.Error())
	if matched {
		return "", 0, nil // No device found, but not an error condition
	}
	if matchErr != nil {
		return "", 0, fmt.Errorf("error processing error message: %v, matching pattern %q: %v", err, pattern, matchErr)
	}
	return "", 0, fmt.Errorf("error getting device name from mount: %v", err)
}

// IsCorruptedMnt return true if err is about corrupted mount point
func (m NodeMounter) IsCorruptedMnt(err error) bool {
	return mountutils.IsCorruptedMnt(err)
}

func (m *NodeMounter) MakeFile(path string) error {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		return proxyMounter.MakeFile(path)
	case *CSIProxyMounter:
		return proxyMounter.MakeFile(path)
	default:
		return ErrUnsupportedMounter
	}
}

func (m *NodeMounter) MakeDir(path string) error {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		return proxyMounter.MakeDir(path)
	case *CSIProxyMounter:
		return proxyMounter.MakeDir(path)
	default:
		return ErrUnsupportedMounter
	}
}

func (m *NodeMounter) PathExists(path string) (bool, error) {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		return proxyMounter.ExistsPath(path)
	case *CSIProxyMounter:
		return proxyMounter.ExistsPath(path)
	default:
		return false, ErrUnsupportedMounter
	}
}

func (m *NodeMounter) Resize(devicePath, deviceMountPath string) (bool, error) {
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		return proxyMounter.ResizeVolume(deviceMountPath)
	case *CSIProxyMounter:
		return proxyMounter.ResizeVolume(deviceMountPath)
	default:
		return false, ErrUnsupportedMounter
	}
}

// NeedResize called at NodeStage to ensure file system is the correct size
func (m *NodeMounter) NeedResize(devicePath, deviceMountPath string) (bool, error) {
	var err error
	var deviceSize, fsSize int64

	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		deviceSize, err = proxyMounter.GetDeviceSize(devicePath)
		if err != nil {
			return false, fmt.Errorf("failed to get device size: %w", err)
		}
		fsSize, err = proxyMounter.GetVolumeSizeInBytes(deviceMountPath)
		if err != nil {
			return false, fmt.Errorf("failed to get filesystem size: %w", err)
		}
	case *CSIProxyMounter:
		deviceSize, err = proxyMounter.GetDeviceSize(devicePath)
		if err != nil {
			return false, fmt.Errorf("failed to get device size: %w", err)
		}
		fsSize, err = proxyMounter.GetVolumeSizeInBytes(deviceMountPath)
		if err != nil {
			return false, fmt.Errorf("failed to get filesystem size: %w", err)
		}
	default:
		return false, ErrUnsupportedMounter
	}

	// Tolerate one block difference (4096 bytes)
	if deviceSize <= util.DefaultBlockSize+fsSize {
		return true, nil
	}
	return false, nil
}

// Unmount volume from target path
func (m *NodeMounter) Unpublish(target string) error {
	var err error
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		err = proxyMounter.Rmdir(target)
	case *CSIProxyMounter:
		err = proxyMounter.Rmdir(target)
	default:
		return ErrUnsupportedMounter
	}

	if err != nil {
		return fmt.Errorf("failed to remove directory: %w", err)
	}

	return nil
}

// Unmount volume from staging path
// usually this staging path is a global directory on the node
func (m *NodeMounter) Unstage(target string) error {
	var err error
	switch proxyMounter := m.SafeFormatAndMount.Interface.(type) {
	case *CSIProxyMounterV2:
		err = proxyMounter.Unmount(target)
	case *CSIProxyMounter:
		err = proxyMounter.Unmount(target)
	default:
		return ErrUnsupportedMounter
	}

	if err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}

	return nil
}
