//go:build windows
// +build windows

// this file has been modified from its original found in:
// https://github.com/kubernetes-sigs/aws-ebs-csi-driver

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

package driver

import (
	"fmt"
	"regexp"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/mounter"
	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/resizefs"
	mountutils "k8s.io/mount-utils"
)

func (m NodeMounter) FormatAndMountSensitiveWithFormatOptions(source string, target string, fstype string, options []string, sensitiveOptions []string, formatOptions []string) error {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	return proxyMounter.FormatAndMountSensitiveWithFormatOptions(source, target, fstype, options, sensitiveOptions, formatOptions)
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
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return "", 0, fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	deviceName, err := proxyMounter.GetDeviceNameFromMount(mountPath, "")
	if err != nil {
		// HACK change csi-proxy behavior instead of relying on fragile internal
		// implementation details!
		// if err contains '"(Get-Item...).Target, output: , error: <nil>' then the
		// internal Get-Item cmdlet didn't fail but no item/device was found at the
		// path so we should return empty string and nil error just like the Linux
		// implementation would.
		pattern := `(Get-Item -Path \S+).Target, output: , error: <nil>|because it does not exist`
		matched, matchErr := regexp.MatchString(pattern, err.Error())
		if matched {
			return "", 0, nil
		}
		err = fmt.Errorf("error getting device name from mount: %v", err)
		if matchErr != nil {
			err = fmt.Errorf("%v, and error matching pattern %q: %v", err, pattern, matchErr)
		}
		return "", 0, err
	}
	return deviceName, 1, nil
}

// IsCorruptedMnt return true if err is about corrupted mount point
func (m NodeMounter) IsCorruptedMnt(err error) bool {
	return mountutils.IsCorruptedMnt(err)
}

func (m *NodeMounter) MakeFile(path string) error {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	return proxyMounter.MakeFile(path)
}

func (m *NodeMounter) MakeDir(path string) error {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	return proxyMounter.MakeDir(path)
}

func (m *NodeMounter) PathExists(path string) (bool, error) {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return false, fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	return proxyMounter.ExistsPath(path)
}

// NeedResize called at NodeStage to ensure file system is the correct size
func (m *NodeMounter) NeedResize(devicePath, deviceMountPath string) (bool, error) {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return false, fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}

	deviceSize, err := proxyMounter.GetDeviceSize(devicePath)
	if err != nil {
		return false, err
	}

	fsSize, err := proxyMounter.GetVolumeSizeInBytes(deviceMountPath)
	if err != nil {
		return false, err
	}
	// Tolerate one block difference (4096 bytes)
	if deviceSize <= DefaultBlockSize+fsSize {
		return true, nil
	}
	return false, nil
}

// Unmount volume from target path
func (m *NodeMounter) Unpublish(target string) error {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	// Remove symlink
	err := proxyMounter.Rmdir(target)
	if err != nil {
		return err
	}
	return nil
}

// Unmount volume from staging path
// usually this staging path is a global directory on the node
func (m *NodeMounter) Unstage(target string) error {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	// Unmounts and offlines the disk via the CSI Proxy API
	err := proxyMounter.Unmount(target)
	if err != nil {
		return err
	}
	return nil
}

func (m *NodeMounter) NewResizeFs() (Resizefs, error) {
	proxyMounter, ok := m.SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return nil, fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}
	return resizefs.NewResizeFs(proxyMounter), nil
}
