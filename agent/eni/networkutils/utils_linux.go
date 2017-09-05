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

// Package networkutils is a collection of helpers for eni/watcher
package networkutils

import (
	"path/filepath"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/eni/netlinkwrapper"

	log "github.com/cihub/seelog"
)

// GetMACAddress retrieves the MAC address of a device using netlink
func GetMACAddress(dev string, netlinkClient netlinkwrapper.NetLink) (string, error) {
	dev = filepath.Base(dev)
	link, err := netlinkClient.LinkByName(dev)
	if err != nil {
		return "", err
	}
	return link.Attrs().HardwareAddr.String(), err
}

// IsValidNetworkDevice is used to differentiate virtual and physical devices
// Returns true only for pci or vif interfaces
func IsValidNetworkDevice(devicePath string) bool {
	/*
	* DevicePath Samples:
	* eth1 -> /devices/pci0000:00/0000:00:05.0/net/eth1
	* eth0 -> ../../devices/pci0000:00/0000:00:03.0/net/eth0
	* lo   -> ../../devices/virtual/net/lo
	*/
	splitDevLink := strings.SplitN(devicePath, "devices/", 2)
	if len(splitDevLink) != 2 {
		log.Warnf("Cannot determine device validity: %s", devicePath)
		return false
	}
	/*
	* CoreOS typically employs the vif style for physical net interfaces
	* Amazon Linux, Ubuntu, RHEL, Fedora, Suse use the traditional pci convention
	*/
	if strings.HasPrefix(splitDevLink[1], pciDevicePrefix) || strings.HasPrefix(splitDevLink[1], vifDevicePrefix) {
		return true
	}
	if strings.HasPrefix(splitDevLink[1], virtualDevicePrefix) {
		return false
	}
	// NOTE: Should never reach here
	log.Criticalf("Failed to validate device path: %s", devicePath)
	return false
}
