//go:build linux
// +build linux

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

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
)

const (
	imageTarPath                          = "/var/lib/ecs/deps/daemons"
	imageTagDefault                       = "latest"
	defaultAgentCommunicationPathHostRoot = "/var/run/ecs"
	defaultApplicationLogPathHostRoot     = "/var/log/ecs/daemons"
	defaultAgentCommunicationMount        = "agentCommunicationMount"
	defaultApplicationLogMount            = "applicationLogMount"
)

// ImportAll function will parse/validate all managed daemon definitions
// defined in /var/lib/ecs/deps/daemons and will return an array
// of valid ManagedDeamon objects
func defaultImportAll() ([]*ManagedDaemon, error) {
	// TODO parse taskdef json files in parameterized dir ie /deps/daemons
	// TODO validate that each daemon's layers are loaded or that daemon has a corresponding image tar
	ebsCsiTarFile := filepath.Join(imageTarPath, EbsCsiDriver, imageFileName)
	if _, err := os.Stat(ebsCsiTarFile); err != nil {
		return []*ManagedDaemon{}, nil
	}
	// found the EBS CSI tar file -- import
	ebsManagedDaemon := NewManagedDaemon(EbsCsiDriver, "latest")
	// add required mounts
	ebsMounts := []*MountPoint{
		&MountPoint{
			SourceVolumeID:       "agentCommunicationMount",
			SourceVolume:         "agentCommunicationMount",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "/var/run/ecs/ebs-csi-driver/",
			ContainerPath:        "/csi-driver/",
		},
		&MountPoint{
			SourceVolumeID:       "applicationLogMount",
			SourceVolume:         "applicationLogMount",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "/var/log/ecs/daemons/ebs-csi-driver/",
			ContainerPath:        "/var/log/",
		},
		&MountPoint{
			SourceVolumeID:       "sharedMounts",
			SourceVolume:         "sharedMounts",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "/mnt/ecs/ebs",
			ContainerPath:        "/mnt/ecs/ebs",
			PropagationShared:    true,
		},
		&MountPoint{
			SourceVolumeID:       "devMount",
			SourceVolume:         "devMount",
			SourceVolumeType:     "host",
			SourceVolumeHostPath: "/dev",
			ContainerPath:        "/dev",
			PropagationShared:    true,
		},
	}
	if err := ebsManagedDaemon.SetMountPoints(ebsMounts); err != nil {
		return nil, fmt.Errorf("Unable to import EBS ManagedDaemon: %s", err)
	}
	var thisCommand []string
	thisCommand = append(thisCommand, "--endpoint=unix://csi-driver/csi-driver.sock")
	thisCommand = append(thisCommand, "--log_file=/var/log/csi.log")
	thisCommand = append(thisCommand, "--log_file_max_size=20")
	thisCommand = append(thisCommand, "--logtostderr=false")
	sysAdmin := "SYS_ADMIN"
	addCapabilities := []*string{&sysAdmin}
	kernelCapabilities := ecsacs.KernelCapabilities{Add: addCapabilities}
	ebsLinuxParams := ecsacs.LinuxParameters{Capabilities: &kernelCapabilities}
	ebsManagedDaemon.linuxParameters = &ebsLinuxParams

	ebsManagedDaemon.command = thisCommand
	ebsManagedDaemon.privileged = true
	return []*ManagedDaemon{ebsManagedDaemon}, nil
}
