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

package ipcompatibility

import "sync"

// Helper struct to track IP compatibility of a network.
type ipCompatibility struct {
	mutex          *sync.RWMutex
	ipv4Compatible bool
	ipv6Compatible bool
}

// Creates a new ipCompatibility struct.
func NewIPCompatibility() *ipCompatibility {
	return &ipCompatibility{
		mutex:          &sync.RWMutex{},
		ipv4Compatible: false,
		ipv6Compatible: false,
	}
}

// SetIPv4Compatible sets the IPv4 compatibility status.
// This method is thread-safe.
func (ic *ipCompatibility) SetIPv4Compatible(compatibility bool) {
	ic.mutex.Lock()
	defer ic.mutex.Unlock()
	ic.ipv4Compatible = compatibility
}

// SetIPv6Compatible sets the IPv6 compatibility status.
// This method is thread-safe.
func (ic *ipCompatibility) SetIPv6Compatible(compatibility bool) {
	ic.mutex.Lock()
	defer ic.mutex.Unlock()
	ic.ipv6Compatible = compatibility
}

// IsIPv4Compatible returns the current IPv4 compatibility status.
// This method is thread-safe.
func (ic *ipCompatibility) IsIPv4Compatible() bool {
	ic.mutex.RLock()
	defer ic.mutex.RUnlock()
	return ic.ipv4Compatible
}

// IsIPv6Compatible returns the current IPv6 compatibility status.
// This method is thread-safe.
func (ic *ipCompatibility) IsIPv6Compatible() bool {
	ic.mutex.RLock()
	defer ic.mutex.RUnlock()
	return ic.ipv6Compatible
}
