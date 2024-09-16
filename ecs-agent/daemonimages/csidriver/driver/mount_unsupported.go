//go:build !linux && !windows
// +build !linux,!windows

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
	"errors"
	"os"
)

// GetDeviceNameFromMount returns the volume ID for a mount path.
func (m NodeMounter) GetDeviceNameFromMount(mountPath string) (string, int, error) {
	return "", 0, errors.New("unsupported platform")
}

// IsCorruptedMnt return true if err is about corrupted mount point
func (m NodeMounter) IsCorruptedMnt(err error) bool {
	return false
}

func (m *NodeMounter) MakeFile(path string) error {
	return errors.New("unsupported platform")
}

func (m *NodeMounter) MakeDir(path string) error {
	return errors.New("unsupported platform")
}

func (m *NodeMounter) PathExists(path string) (bool, error) {
	return false, errors.New("unsupported platform")
}

func (m *NodeMounter) NeedResize(devicePath string, deviceMountPath string) (bool, error) {
	return false, errors.New("unsupported platform")
}

func (m *NodeMounter) Unpublish(path string) error {
	return errors.New("unsupported platform")
}

//lint:ignore U1000 Ignore function used only for tests
func (m *NodeMounter) getExtSize(devicePath string) (uint64, uint64, error) {
	return 0, 0, errors.New("unsupported platform")
}

//lint:ignore U1000 Ignore function used only for tests
func (m *NodeMounter) getXFSSize(devicePath string) (uint64, uint64, error) {
	return 0, 0, errors.New("unsupported platform")
}

//lint:ignore U1000 Ignore function used only for tests
func (m *NodeMounter) parseFsInfoOutput(cmdOutput string, spliter string, blockSizeKey string, blockCountKey string) (uint64, uint64, error) {
	return 0, 0, errors.New("unsupported platform")
}

func (m *NodeMounter) Unstage(path string) error {
	return errors.New("unsupported platform")
}

func (m *NodeMounter) NewResizeFs() (Resizefs, error) {
	return nil, errors.New("unsupported platform")
}

// DeviceIdentifier is for mocking os io functions used for the driver to
// identify an EBS volume's corresponding device (in Linux, the path under
// /dev; in Windows, the volume number) so that it can mount it. For volumes
// already mounted, see GetDeviceNameFromMount.
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nvme-ebs-volumes.html#identify-nvme-ebs-device
type DeviceIdentifier interface {
	Lstat(name string) (os.FileInfo, error)
	EvalSymlinks(path string) (string, error)
}

type nodeDeviceIdentifier struct{}

func newNodeDeviceIdentifier() DeviceIdentifier {
	return &nodeDeviceIdentifier{}
}

func (i *nodeDeviceIdentifier) Lstat(name string) (os.FileInfo, error) {
	return nil, errors.New("unsupported platform")
}

func (i *nodeDeviceIdentifier) EvalSymlinks(path string) (string, error) {
	return "", errors.New("unsupported platform")
}
