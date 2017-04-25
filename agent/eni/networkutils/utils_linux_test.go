// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package networkutils

import (
	"errors"
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"

	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper/mocks"
)

const (
	randomDevice     = "eth1"
	validMAC         = "00:0a:95:9d:68:16"
	pciDevPath       = " ../../devices/pci0000:00/0000:00:03.0/net/eth1"
	virtualDevPath   = "../../devices/virtual/net/lo"
	invalidDevPath   = "../../virtual/net/lo"
	incorrectDevPath = "../../devices/totally/wrong/net/path"
)

// TestGetMACAddress checks obtaining MACAddress by using netlinkClient
func TestGetMACAddress(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	pm, _ := net.ParseMAC(validMAC)
	mockNetlink.EXPECT().LinkByName(randomDevice).Return(
		&netlink.Device{
			LinkAttrs: netlink.LinkAttrs{
				HardwareAddr: pm,
				Name:         randomDevice,
			},
		}, nil)
	mac, err := GetMACAddress(randomDevice, mockNetlink)
	assert.Nil(t, err)
	assert.Equal(t, mac, validMAC)
}

// TestGetMACAddressWithNetlinkError attempts to test the netlinkClient
// error code path
func TestGetMACAddressWithNetlinkError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockNetlink := mock_netlinkwrapper.NewMockNetLink(mockCtrl)
	mockNetlink.EXPECT().LinkByName(randomDevice).Return(
		&netlink.Device{},
		errors.New("Dummy Netlink Error"))
	mac, err := GetMACAddress(randomDevice, mockNetlink)
	assert.Error(t, err)
	assert.Empty(t, mac)
}

// TestIsValidDevicePathWithPCIDevice checks for valid PCI device path
func TestIsValidDevicePathWithPCIDevice(t *testing.T) {
	devStatus := IsValidNetworkDevice(pciDevPath)
	assert.True(t, devStatus)
}

// TestIsValidDevicePathWithVirtualDevice checks for virtual devices
func TestIsValidDevicePathWithVirtualDevice(t *testing.T) {
	devStatus := IsValidNetworkDevice(virtualDevPath)
	assert.False(t, devStatus)
}

// TestIsValidDevicePathWithInvalidDevPath checks for invalid device path
func TestIsValidDevicePathWithInvalidDevPath(t *testing.T) {
	devStatus := IsValidNetworkDevice(invalidDevPath)
	assert.False(t, devStatus)
}

// TestIsValidDevicePathWithIncorrectDevPath tests for incorrect device path
func TestIsValidDevicePathWithIncorrectDevPath(t *testing.T) {
	devStatus := IsValidNetworkDevice(incorrectDevPath)
	assert.False(t, devStatus)
}
