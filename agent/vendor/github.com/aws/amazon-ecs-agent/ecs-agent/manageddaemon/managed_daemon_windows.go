//go:build windows
// +build windows

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package manageddaemon

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	imageTarPath                          = "C:\\ProgramData\\Amazon\\ECS\\data\\"
	imageTagDefault                       = "latest"
	defaultAgentCommunicationPathHostRoot = "C:\\ProgramData\\Amazon\\ECS"
	defaultApplicationLogPathHostRoot     = "C:\\ProgramData\\Amazon\\ECS\\log"
	defaultAgentCommunicationMount        = "agentCommunicationMount"
	defaultApplicationLogMount            = "applicationLogMount"
)

// defaultImportAll function will parse/validate all managed daemon definitions
// defined and will return an array of valid ManagedDaemon objects
func defaultImportAll() ([]*ManagedDaemon, error) {
	ebsCsiTarFile := filepath.Join(imageTarPath, EbsCsiDriver, imageFileName)
	if _, err := os.Stat(ebsCsiTarFile); err != nil {
		return []*ManagedDaemon{}, nil
	}
	// locate the EBS CSI tar file -- import
	ebsManagedDaemon := NewManagedDaemon(EbsCsiDriver, "latest")
	// add required mounts
	ebsMounts := []*MountPoint{
		&MountPoint{
			SourceVolumeID:       "agentCommunicationMount",
			SourceVolume:         "agentCommunicationMount",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "C:\\ProgramData\\Amazon\\ECS\\ebs-csi-driver",
			ContainerPath:        "C:\\ebs-csi-driver\\",
		},
		&MountPoint{
			SourceVolumeID:       "applicationLogMount",
			SourceVolume:         "applicationLogMount",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "C:\\ProgramData\\Amazon\\ECS\\log\\daemons",
			ContainerPath:        "C:\\csi-driver\\log",
		},
		&MountPoint{
			SourceVolumeID:       "sharedMounts",
			SourceVolume:         "sharedMounts",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "C:\\ProgramData\\Amazon\\ECS\\ebs",
			ContainerPath:        "C:\\csi-driver\\ebs\\",
			PropagationShared:    false,
		},
		// the following three mount points are for connecting to the CSIProxy server that is running
		// as a Windows Service on the host
		&MountPoint{
			SourceVolumeID:       "csiProxyDiskMount",
			SourceVolume:         "csiProxyDiskMount",
			SourceVolumeType:     "npipe",
			SourceVolumeHostPath: "\\\\.\\pipe\\csi-proxy-disk-v1",
			ContainerPath:        "\\\\.\\pipe\\csi-proxy-disk-v1",
			PropagationShared:    false,
		},
		&MountPoint{
			SourceVolumeID:       "csiProxyFileSystemMount",
			SourceVolume:         "csiProxyFileSystemMount",
			SourceVolumeType:     "npipe",
			SourceVolumeHostPath: "\\\\.\\pipe\\csi-proxy-filesystem-v1",
			ContainerPath:        "\\\\.\\pipe\\csi-proxy-filesystem-v1",
			PropagationShared:    false,
		},
		&MountPoint{
			SourceVolumeID:       "csiProxyVolumeMount",
			SourceVolume:         "csiProxyVolumeMount",
			SourceVolumeType:     "npipe",
			SourceVolumeHostPath: "\\\\.\\pipe\\csi-proxy-volume-v1",
			ContainerPath:        "\\\\.\\pipe\\csi-proxy-volume-v1",
			PropagationShared:    false,
		},
	}
	if err := ebsManagedDaemon.SetMountPoints(ebsMounts); err != nil {
		return nil, fmt.Errorf("Unable to import EBS ManagedDaemon: %s", err)
	}
	var thisCommand []string
	thisCommand = append(thisCommand, "-endpoint=unix://ebs-csi-driver/csi-driver.sock")
	thisCommand = append(thisCommand, "-log_file=C:\\csi-driver\\log\\csi.log")
	thisCommand = append(thisCommand, "-log_file_max_size=20")
	thisCommand = append(thisCommand, "-logtostderr=false")

	ebsManagedDaemon.command = thisCommand
	ebsManagedDaemon.privileged = false
	return []*ManagedDaemon{ebsManagedDaemon}, nil
}
