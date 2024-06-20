//go:build windows && integration
// +build windows,integration

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
package csiclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

const timeoutDuration = 1 * time.Second

var (
	volumeID = "vol-06a9cf692fc9f4bc4"
	nvmeName = "xvde"
	// FSTypeNtfs represents the ntfs filesystem type
	FSTypeNtfs = "ntfs"
	// can be mounted in read/write mode to exactly 1 host
	ReadWriteOnce v1.PersistentVolumeAccessMode = "ReadWriteOnce"
)

// Tests that CSIClient can stage a EBS volume using the CSI Driver.
//
// This test assumes that there is only one EBS Volume attached to the host that's mounted
// on "/" path. It also assumes that an EBS CSI Driver is running and listening on
// /tmp/ebs-csi-driver.sock socket file.
//
// A typical environment for this test is an EC2 instance with a single EBS volume as its
// root device volume.
func TestNodeStageVolume(t *testing.T) {

	targetPath := "C:\\csi_proxy\\mount"
	devicePath := "/dev/fake"

	// Get metrics for the root volume from EBS CSI Driver.
	csiClient := NewCSIClient("C:\\Program Files\\Amazon\\ECS\\ebs-csi-driver\\socket\\csi.sock")
	//getVolumeCtx, getVolumeCtxCancel := context.WithTimeout(context.Background(), timeoutDuration)
	//defer getVolumeCtxCancel()
	var err = csiClient.NodeStageVolume(context.TODO(), volumeID, map[string]string{"devicePath": devicePath},
		targetPath, FSTypeNtfs, ReadWriteOnce, nil, nil,
		nil, nil)

	require.NoError(t, err)

}

func TestNodeUnstageVolume(t *testing.T) {

	targetPath := "C:\\csi_proxy\\mount"
	// Get metrics for the root volume from EBS CSI Driver.
	csiClient := NewCSIClient("C:\\Program Files\\Amazon\\ECS\\ebs-csi-driver\\socket\\csi.sock")
	//getVolumeCtx, getVolumeCtxCancel := context.WithTimeout(context.Background(), timeoutDuration)
	//defer getVolumeCtxCancel()
	err := csiClient.NodeUnstageVolume(context.TODO(), volumeID, targetPath)
	require.NoError(t, err)

}
