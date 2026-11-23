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

package net

import "net"

// InterfaceLister lists the network interfaces in the caller's network
// namespace. Both the agent's and the shared library's network wrappers
// satisfy it.
type InterfaceLister interface {
	Interfaces() ([]net.Interface, error)
}

// MACToNameMap maps each host interface's MAC address to its device name.
// Callers holding an interface's MAC address from a control plane message use
// it to find the device the host actually created.
func MACToNameMap(lister InterfaceLister) (map[string]string, error) {
	links, err := lister.Interfaces()
	if err != nil {
		return nil, err
	}

	macToName := make(map[string]string, len(links))
	for _, link := range links {
		macToName[link.HardwareAddr.String()] = link.Name
	}

	return macToName, nil
}
