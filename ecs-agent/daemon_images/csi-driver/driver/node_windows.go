//go:build windows
// +build windows

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

package driver

import (
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/daemon_images/csi-driver/mounter"
)

// IsBlockDevice checks if the given path is a block device
func (d *nodeService) IsBlockDevice(fullPath string) (bool, error) {
	return false, nil
}

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
