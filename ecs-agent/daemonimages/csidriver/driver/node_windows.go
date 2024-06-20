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
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemonimages/csidriver/mounter"
)

// getBlockSizeBytes gets the size of the disk in bytes
func (d *nodeService) getBlockSizeBytes(devicePath string) (int64, error) {
	proxyMounter, ok := (d.mounter.(*NodeMounter)).SafeFormatAndMount.Interface.(*mounter.CSIProxyMounter)
	if !ok {
		return -1, fmt.Errorf("failed to cast mounter to csi proxy mounter")
	}

	sizeInBytes, err := proxyMounter.GetDeviceSize(devicePath)
	if err != nil {
		return -1, err
	}

	return sizeInBytes, nil
}

// findDevicePath finds disk number of device
// https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/ec2-windows-volumes.html#list-nvme-powershell
func (d *nodeService) findDevicePath(devicePath, volumeID, _ string) (string, error) {
	diskIDs, err := d.deviceIdentifier.ListDiskIDs()
	if err != nil {
		return "", fmt.Errorf("error listing disk ids: %q", err)
	}

	foundDiskNumber := ""
	for diskNumber, diskID := range diskIDs {
		serialNumber := diskID.GetSerialNumber()
		cleanVolumeID := strings.ReplaceAll(volumeID, "-", "")
		if strings.Contains(serialNumber, cleanVolumeID) {
			foundDiskNumber = strconv.Itoa(int(diskNumber))
			break
		}
	}

	if foundDiskNumber == "" {
		return "", fmt.Errorf("disk number for device path %q volume id %q not found", devicePath, volumeID)
	}

	return foundDiskNumber, nil
}
