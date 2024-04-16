//go:build linux
// +build linux

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
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

func (d *nodeService) appendPartition(devicePath, partition string) string {
	if partition == "" {
		return devicePath
	}

	if strings.HasPrefix(devicePath, "/dev/nvme") {
		return devicePath + nvmeDiskPartitionSuffix + partition
	}

	return devicePath + diskPartitionSuffix + partition
}

// findDevicePath finds path of device and verifies its existence
// if the device is not nvme, return the path directly
// if the device is nvme, finds and returns the nvme device path eg. /dev/nvme1n1
func (d *nodeService) findDevicePath(devicePath, volumeID, partition string) (string, error) {
	canonicalDevicePath := ""

	// If the given path exists, the device MAY be nvme. Further, it MAY be a
	// symlink to the nvme device path like:
	// | $ stat /dev/xvdba
	// | File: ‘/dev/xvdba’ -> ‘nvme1n1’
	// Since these are maybes, not guarantees, the search for the nvme device
	// path below must happen and must rely on volume ID

	klog.InfoS("Device Path:",
		"Driver", devicePath,
		"VolumeID", volumeID,
		"Partition", partition,
	)
	exists, err := d.mounter.PathExists(devicePath)

	if err != nil {
		return "", fmt.Errorf("failed to check if path %q exists: %w", devicePath, err)
	}

	if exists {
		stat, lstatErr := d.deviceIdentifier.Lstat(devicePath)

		klog.InfoS("Device Path:",
			"Driver", devicePath,
			"VolumeID", volumeID,
			"Partition", partition,
		)

		if lstatErr != nil {
			return "", fmt.Errorf("failed to lstat %q: %w", devicePath, err)
		}

		if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
			canonicalDevicePath, err = d.deviceIdentifier.EvalSymlinks(devicePath)
			if err != nil {
				return "", fmt.Errorf("failed to evaluate symlink %q: %w", devicePath, err)
			}
		} else {
			canonicalDevicePath = devicePath
		}

		klog.V(5).InfoS("[Debug] The canonical device path was resolved", "devicePath", devicePath, "cacanonicalDevicePath", canonicalDevicePath)
		return d.appendPartition(canonicalDevicePath, partition), nil
	}

	klog.V(5).InfoS("[Debug] Falling back to nvme volume ID lookup", "devicePath", devicePath)

	// AWS recommends identifying devices by volume ID
	// (https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nvme-ebs-volumes.html),
	// so find the nvme device path using volume ID. This is the magic name on
	// which AWS presents NVME devices under /dev/disk/by-id/. For example,
	// vol-0fab1d5e3f72a5e23 creates a symlink at
	// /dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0fab1d5e3f72a5e23
	nvmeName := "nvme-Amazon_Elastic_Block_Store_" + strings.Replace(volumeID, "-", "", -1)

	nvmeDevicePath, err := findNvmeVolume(d.deviceIdentifier, nvmeName)

	if err == nil {
		klog.V(5).InfoS("[Debug] successfully resolved", "nvmeName", nvmeName, "nvmeDevicePath", nvmeDevicePath)
		canonicalDevicePath = nvmeDevicePath
		return d.appendPartition(canonicalDevicePath, partition), nil
	} else {
		klog.V(5).InfoS("[Debug] error searching for nvme path", "nvmeName", nvmeName, "err", err)
	}

	if canonicalDevicePath == "" {
		return "", errNoDevicePathFound(devicePath, volumeID)
	}

	canonicalDevicePath = d.appendPartition(canonicalDevicePath, partition)
	return canonicalDevicePath, nil
}

func errNoDevicePathFound(devicePath, volumeID string) error {
	return fmt.Errorf("no device path for device %q volume %q found", devicePath, volumeID)
}

// findNvmeVolume looks for the nvme volume with the specified name
// It follows the symlink (if it exists) and returns the absolute path to the device
func findNvmeVolume(deviceIdentifier DeviceIdentifier, findName string) (device string, err error) {
	p := filepath.Join("/dev/disk/by-id/", findName)
	stat, err := deviceIdentifier.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			klog.V(5).InfoS("[Debug] nvme path not found", "path", p)
			return "", fmt.Errorf("nvme path %q not found", p)
		}
		return "", fmt.Errorf("error getting stat of %q: %w", p, err)
	}

	if stat.Mode()&os.ModeSymlink != os.ModeSymlink {
		klog.InfoS("nvme file found, but was not a symlink", "path", p)
		return "", fmt.Errorf("nvme file %q found, but was not a symlink", p)
	}
	// Find the target, resolving to an absolute path
	// For example, /dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0fab1d5e3f72a5e23 -> ../../nvme2n1
	resolved, err := deviceIdentifier.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("error reading target of symlink %q: %w", p, err)
	}

	if !strings.HasPrefix(resolved, "/dev") {
		return "", fmt.Errorf("resolved symlink for %q was unexpected: %q", p, resolved)
	}

	return resolved, nil
}

// IsBlock checks if the given path is a block device
func (d *nodeService) IsBlockDevice(fullPath string) (bool, error) {
	var st unix.Stat_t
	err := unix.Stat(fullPath, &st)
	if err != nil {
		return false, err
	}

	return (st.Mode & unix.S_IFMT) == unix.S_IFBLK, nil
}

func (d *nodeService) getBlockSizeBytes(devicePath string) (int64, error) {
	cmd := d.mounter.(*NodeMounter).Exec.Command("blockdev", "--getsize64", devicePath)
	output, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("error when getting size of block volume at path %s: output: %s, err: %w", devicePath, string(output), err)
	}
	strOut := strings.TrimSpace(string(output))
	gotSizeBytes, err := strconv.ParseInt(strOut, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse size %s as int", strOut)
	}
	return gotSizeBytes, nil
}
