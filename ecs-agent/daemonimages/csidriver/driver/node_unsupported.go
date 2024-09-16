//go:build !linux && !windows
// +build !linux,!windows

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
	"errors"
)

func (d *nodeService) appendPartition(devicePath, partition string) string {
	return ""
}

// findDevicePath finds path of device and verifies its existence
// if the device is not nvme, return the path directly
// if the device is nvme, finds and returns the nvme device path eg. /dev/nvme1n1
func (d *nodeService) findDevicePath(devicePath, volumeID, partition string) (string, error) {
	return "", errors.New("unsupported platform")
}

// IsBlock checks if the given path is a block device
func (d *nodeService) IsBlockDevice(fullPath string) (bool, error) {
	return false, errors.New("unsupported platform")
}

func (d *nodeService) getBlockSizeBytes(devicePath string, _ string) (int64, error) {
	return -1, errors.New("unsupported platform")
}
