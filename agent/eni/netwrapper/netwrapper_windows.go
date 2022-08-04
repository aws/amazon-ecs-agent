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

package netwrapper

import (
	"encoding/json"
	"net"
	"os/exec"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

type windowsNetworkAdapter struct {
	Name       string `json:"ifAlias"`
	MacAddress string `json:"MacAddress"`
	IfIndex    int    `json:"ifIndex"`
	MTU        int    `json:"ActiveMaximumTransmissionUnit"`
}

type winAdapters []windowsNetworkAdapter

// Making it visible for unit testing
var execCommand = exec.Command

// getAllNetworkInterfaces returns all the network adapters.
// Unfortunately, as of August 2022, the Win32 IPHelper API seems to be flaky wherein
// the same stops responding in few intermittent cases. Therefore, to avoid failures
// in such cases, we have switched to using Powershell cmdlet for fetching all the interfaces.
func (utils *utils) getAllNetworkInterfaces() ([]net.Interface, error) {
	log.Info("Running Powershell command to fetch all the network adapters")

	// This command returns a JSON array of all the network interfaces on the instance.
	cmd := "Get-NetAdapter | select ifAlias, ifIndex, MacAddress, ActiveMaximumTransmissionUnit | ConvertTo-Json"
	out, err := execCommand("Powershell", "-C", cmd).Output()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to execute Get-NetAdapter for retrieving all interfaces")
	}
	log.Debugf("Output of Get-NetAdapters Powershell command execution: %s", out)

	// Convert the obtained JSON array into an array of structs.
	var data winAdapters
	err = json.Unmarshal(out, &data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal the json of network adapters")
	}

	// This is an error condition since the instance would have at least 1 ENI.
	if len(data) == 0 {
		return nil, errors.Errorf("invalid number of network adapters found on the instance")
	}
	log.Debugf("Unmarshalled Powershell output into an array of structs")

	// Convert the data into an array of net.Interface
	var ifaces []net.Interface
	for _, adapter := range data {
		mac, err := net.ParseMAC(adapter.MacAddress)
		if err != nil {
			// Log the error and continue to get as many adapters as possible.
			log.Errorf("unable to parse mac address: %v", err)
			continue
		}

		iface := net.Interface{
			Index:        adapter.IfIndex,
			MTU:          adapter.MTU,
			Name:         adapter.Name,
			HardwareAddr: mac,
		}

		ifaces = append(ifaces, iface)
	}
	log.Debugf("All the interfaces found using Get-NetAdapters are: %v", ifaces)

	return ifaces, nil
}
