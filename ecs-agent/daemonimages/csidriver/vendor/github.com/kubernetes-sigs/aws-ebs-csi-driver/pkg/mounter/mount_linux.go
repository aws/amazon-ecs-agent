//go:build linux
// +build linux

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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/util"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
	mountutils "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

const (
	nvmeDiskPartitionSuffix = "p"
	diskPartitionSuffix     = ""
)

func NewSafeMounter() (*mountutils.SafeFormatAndMount, error) {
	return &mountutils.SafeFormatAndMount{
		Interface: mountutils.New(""),
		Exec:      utilexec.New(),
	}, nil
}

func NewSafeMounterV2() (*mountutils.SafeFormatAndMount, error) {
	return nil, fmt.Errorf("NewSafeMounterV2 is not supported on this platform")
}

// FindDevicePath finds path of device and verifies its existence
// if the device is not nvme, return the path directly
// if the device is nvme, finds and returns the nvme device path eg. /dev/nvme1n1
func (m *NodeMounter) FindDevicePath(devicePath, volumeID, partition, region string) (string, error) {
	strippedVolumeName := strings.Replace(volumeID, "-", "", -1)
	canonicalDevicePath := ""

	// If the given path exists, the device MAY be nvme. Further, it MAY be a
	// symlink to the nvme device path like:
	// | $ stat /dev/xvdba
	// | File: ‘/dev/xvdba’ -> ‘nvme1n1’
	// Since these are maybes, not guarantees, the search for the nvme device
	// path below must happen and must rely on volume ID
	exists, err := m.PathExists(devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to check if path %q exists: %w", devicePath, err)
	}

	if exists {
		stat, lstatErr := os.Lstat(devicePath)
		if lstatErr != nil {
			return "", fmt.Errorf("failed to lstat %q: %w", devicePath, err)
		}

		if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
			canonicalDevicePath, err = filepath.EvalSymlinks(devicePath)
			if err != nil {
				return "", fmt.Errorf("failed to evaluate symlink %q: %w", devicePath, err)
			}
		} else {
			canonicalDevicePath = devicePath
		}

		klog.V(5).InfoS("[Debug] The canonical device path was resolved", "devicePath", devicePath, "cacanonicalDevicePath", canonicalDevicePath)
		if err = verifyVolumeSerialMatch(canonicalDevicePath, strippedVolumeName, execRunner); err != nil {
			return "", err
		}
		return m.appendPartition(canonicalDevicePath, partition), nil
	}

	klog.V(5).InfoS("[Debug] Falling back to nvme volume ID lookup", "devicePath", devicePath)

	// AWS recommends identifying devices by volume ID
	// (https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nvme-ebs-volumes.html),
	// so find the nvme device path using volume ID. This is the magic name on
	// which AWS presents NVME devices under /dev/disk/by-id/. For example,
	// vol-0fab1d5e3f72a5e23 creates a symlink at
	// /dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0fab1d5e3f72a5e23
	nvmeName := "nvme-Amazon_Elastic_Block_Store_" + strippedVolumeName
	nvmeDevicePath, err := findNvmeVolume(nvmeName)

	if err == nil {
		klog.V(5).InfoS("[Debug] successfully resolved", "nvmeName", nvmeName, "nvmeDevicePath", nvmeDevicePath)
		canonicalDevicePath = nvmeDevicePath
		if err = verifyVolumeSerialMatch(canonicalDevicePath, strippedVolumeName, execRunner); err != nil {
			return "", err
		}
		return m.appendPartition(canonicalDevicePath, partition), nil
	} else {
		klog.V(5).InfoS("[Debug] error searching for nvme path", "nvmeName", nvmeName, "err", err)
	}

	if util.IsSBE(region) {
		klog.V(5).InfoS("[Debug] Falling back to snow volume lookup", "devicePath", devicePath)
		// Snow completely ignores the requested device path and mounts volumes starting at /dev/vda .. /dev/vdb .. etc
		// Morph the device path to the snow form by chopping off the last letter and prefixing with /dev/vd
		// VMs on snow devices are currently limited to 10 block devices each - if that ever exceeds 26 this will need
		// to be adapted
		canonicalDevicePath = "/dev/vd" + devicePath[len(devicePath)-1:]
	}

	if canonicalDevicePath == "" {
		return "", fmt.Errorf("no device path for device %q volume %q found", devicePath, volumeID)
	}

	canonicalDevicePath = m.appendPartition(canonicalDevicePath, partition)
	return canonicalDevicePath, nil
}

// findNvmeVolume looks for the nvme volume with the specified name
// It follows the symlink (if it exists) and returns the absolute path to the device
func findNvmeVolume(findName string) (device string, err error) {
	p := filepath.Join("/dev/disk/by-id/", findName)
	stat, err := os.Lstat(p)
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
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("error reading target of symlink %q: %w", p, err)
	}

	if !strings.HasPrefix(resolved, "/dev") {
		return "", fmt.Errorf("resolved symlink for %q was unexpected: %q", p, resolved)
	}

	return resolved, nil
}

// execRunner is a helper to inject exec.Comamnd().CombinedOutput() for verifyVolumeSerialMatch
// Tests use a mocked version that does not actually execute any binaries
func execRunner(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).CombinedOutput()
}

// verifyVolumeSerialMatch checks the volume serial of the device against the expected volume
func verifyVolumeSerialMatch(canonicalDevicePath string, strippedVolumeName string, execRunner func(string, ...string) ([]byte, error)) error {
	// In some rare cases, a race condition can lead to the /dev/disk/by-id/ symlink becoming out of date
	// See https://github.com/kubernetes-sigs/aws-ebs-csi-driver/issues/1224 for more info
	// Attempt to use lsblk to double check that the nvme device selected was the correct volume
	output, err := execRunner("lsblk", "--noheadings", "--ascii", "--nodeps", "--output", "SERIAL", canonicalDevicePath)

	if err == nil {
		// Look for an EBS volume ID in the output, compare all matches against what we expect
		// (in some rare cases there may be multiple matches due to lsblk printing partitions)
		// If no volume ID is in the output (non-Nitro instances, SBE devices, etc) silently proceed
		volumeRegex := regexp.MustCompile(`vol[a-z0-9]+`)
		for _, volume := range volumeRegex.FindAllString(string(output), -1) {
			klog.V(6).InfoS("Comparing volume serial", "canonicalDevicePath", canonicalDevicePath, "expected", strippedVolumeName, "actual", volume)
			if volume != strippedVolumeName {
				return fmt.Errorf("refusing to mount %s because it claims to be %s but should be %s", canonicalDevicePath, volume, strippedVolumeName)
			}
		}
	} else {
		// If the command fails (for example, because lsblk is not available), silently ignore the error and proceed
		klog.V(5).ErrorS(err, "Ignoring lsblk failure", "canonicalDevicePath", canonicalDevicePath, "strippedVolumeName", strippedVolumeName)
	}

	return nil
}

// PreparePublishTarget creates the target directory for the volume to be mounted
func (m *NodeMounter) PreparePublishTarget(target string) error {
	klog.V(4).InfoS("NodePublishVolume: creating dir", "target", target)
	if err := m.MakeDir(target); err != nil {
		return fmt.Errorf("could not create dir %q: %w", target, err)
	}
	return nil
}

// IsBlockDevice checks if the given path is a block device
func (m *NodeMounter) IsBlockDevice(fullPath string) (bool, error) {
	var st unix.Stat_t
	err := unix.Stat(fullPath, &st)
	if err != nil {
		return false, err
	}

	return (st.Mode & unix.S_IFMT) == unix.S_IFBLK, nil
}

// GetBlockSizeBytes gets the size of the disk in bytes
func (m *NodeMounter) GetBlockSizeBytes(devicePath string) (int64, error) {
	output, err := m.Exec.Command("blockdev", "--getsize64", devicePath).Output()
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

// appendPartition appends the partition to the device path
func (m *NodeMounter) appendPartition(devicePath, partition string) string {
	if partition == "" {
		return devicePath
	}

	if strings.HasPrefix(devicePath, "/dev/nvme") {
		return devicePath + nvmeDiskPartitionSuffix + partition
	}

	return devicePath + diskPartitionSuffix + partition
}

// GetDeviceNameFromMount returns the volume ID for a mount path.
func (m NodeMounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	return mountutils.GetDeviceNameFromMount(m, mountPath)
}

// IsCorruptedMnt return true if err is about corrupted mount point
func (m NodeMounter) IsCorruptedMnt(err error) bool {
	return mountutils.IsCorruptedMnt(err)
}

// This function is mirrored in ./sanity_test.go to make sure sanity test covered this block of code
// Please mirror the change to func MakeFile in ./sanity_test.go
func (m *NodeMounter) MakeFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	if err = f.Close(); err != nil {
		return err
	}
	return nil
}

// This function is mirrored in ./sanity_test.go to make sure sanity test covered this block of code
// Please mirror the change to func MakeFile in ./sanity_test.go
func (m *NodeMounter) MakeDir(path string) error {
	err := os.MkdirAll(path, os.FileMode(0755))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

// This function is mirrored in ./sanity_test.go to make sure sanity test covered this block of code
// Please mirror the change to func MakeFile in ./sanity_test.go
func (m *NodeMounter) PathExists(path string) (bool, error) {
	return mountutils.PathExists(path)
}

// Resize resizes the filesystem of the given devicePath
func (m *NodeMounter) Resize(devicePath, deviceMountPath string) (bool, error) {
	return mountutils.NewResizeFs(m.Exec).Resize(devicePath, deviceMountPath)
}

// NeedResize checks if the filesystem of the given devicePath needs to be resized
func (m *NodeMounter) NeedResize(devicePath string, deviceMountPath string) (bool, error) {
	return mountutils.NewResizeFs(m.Exec).NeedResize(devicePath, deviceMountPath)
}

// Unpublish unmounts the given path
func (m *NodeMounter) Unpublish(path string) error {
	// On linux, unpublish and unstage both perform an unmount
	return m.Unstage(path)
}

// Unstage unmounts the given path
func (m *NodeMounter) Unstage(path string) error {
	err := mountutils.CleanupMountPoint(path, m, false)
	// Ignore the error when it contains "not mounted", because that indicates the
	// world is already in the desired state
	//
	// mount-utils attempts to detect this on its own but fails when running on
	// a read-only root filesystem, which our manifests use by default
	if err == nil || strings.Contains(fmt.Sprint(err), "not mounted") {
		return nil
	} else {
		return err
	}
}
