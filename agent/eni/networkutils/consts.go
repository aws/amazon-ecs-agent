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

package networkutils

import "time"

const (
	pciDevicePrefix     = "pci"
	vifDevicePrefix     = "vif"
	virtualDevicePrefix = "virtual"

	// macAddressBackoffMin specifies the minimum duration for the backoff
	// when looking for an ENI's mac address on the host
	macAddressBackoffMin = 2 * time.Millisecond

	// macAddressBackoffMax specifies the maximum duration for the backoff
	// when looking for an ENI's mac address on the host
	macAddressBackoffMax = 200 * time.Millisecond

	// macAddressBackoffJitter specifies the jitter multiple percentage when
	// looking for an ENI's mac address on the host
	macAddressBackoffJitter = 0.2

	// macAddressBackoffMultiple specifies the backoff duration multiplier
	// when looking for an ENI's mac address on the host
	macAddressBackoffMultiple = 1.5
)
