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

package driver

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	mountutils "k8s.io/mount-utils"
)

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

func (m *NodeMounter) NeedResize(devicePath string, deviceMountPath string) (bool, error) {
	return mountutils.NewResizeFs(m.Exec).NeedResize(devicePath, deviceMountPath)
}

func (m *NodeMounter) getExtSize(devicePath string) (uint64, uint64, error) {
	output, err := m.SafeFormatAndMount.Exec.Command("dumpe2fs", "-h", devicePath).CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read size of filesystem on %s: %w: %s", devicePath, err, string(output))
	}

	blockSize, blockCount, _ := m.parseFsInfoOutput(string(output), ":", "block size", "block count")

	if blockSize == 0 {
		return 0, 0, fmt.Errorf("could not find block size of device %s", devicePath)
	}
	if blockCount == 0 {
		return 0, 0, fmt.Errorf("could not find block count of device %s", devicePath)
	}
	return blockSize, blockSize * blockCount, nil
}

func (m *NodeMounter) getXFSSize(devicePath string) (uint64, uint64, error) {
	output, err := m.SafeFormatAndMount.Exec.Command("xfs_io", "-c", "statfs", devicePath).CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read size of filesystem on %s: %w: %s", devicePath, err, string(output))
	}

	blockSize, blockCount, _ := m.parseFsInfoOutput(string(output), "=", "geom.bsize", "geom.datablocks")

	if blockSize == 0 {
		return 0, 0, fmt.Errorf("could not find block size of device %s", devicePath)
	}
	if blockCount == 0 {
		return 0, 0, fmt.Errorf("could not find block count of device %s", devicePath)
	}
	return blockSize, blockSize * blockCount, nil
}

func (m *NodeMounter) parseFsInfoOutput(cmdOutput string, spliter string, blockSizeKey string, blockCountKey string) (uint64, uint64, error) {
	lines := strings.Split(cmdOutput, "\n")
	var blockSize, blockCount uint64
	var err error

	for _, line := range lines {
		tokens := strings.Split(line, spliter)
		if len(tokens) != 2 {
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(tokens[0])), strings.ToLower(strings.TrimSpace(tokens[1]))
		if key == blockSizeKey {
			blockSize, err = strconv.ParseUint(value, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to parse block size %s: %w", value, err)
			}
		}
		if key == blockCountKey {
			blockCount, err = strconv.ParseUint(value, 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to parse block count %s: %w", value, err)
			}
		}
	}
	return blockSize, blockCount, err
}

func (m *NodeMounter) Unpublish(path string) error {
	// On linux, unpublish and unstage both perform an unmount
	return m.Unstage(path)
}

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

func (m *NodeMounter) NewResizeFs() (Resizefs, error) {
	return mountutils.NewResizeFs(m.Exec), nil
}
