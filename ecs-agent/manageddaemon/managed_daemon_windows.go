//go:build windows
// +build windows

package manageddaemon

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
)

const (
	imageTarPath                          = "C:\\ProgramFiles\\Amazon"
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
	ebsCsiTarFile := filepath.Join(imageTarPath, EbsCsiDriver, "ebs-csi-driver.tar")
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
