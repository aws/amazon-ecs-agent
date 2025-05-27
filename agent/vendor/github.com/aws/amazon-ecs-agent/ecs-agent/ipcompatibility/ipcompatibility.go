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

// IPCompatibility is used to track IP compatibility of a network.
type IPCompatibility struct {
	ipv4Compatible bool
	ipv6Compatible bool
}

// NewIPCompatibility returns a new IPCompatibility instance.
func NewIPCompatibility(ipv4Compatible, ipv6Compatible bool) IPCompatibility {
	return IPCompatibility{ipv4Compatible: ipv4Compatible, ipv6Compatible: ipv6Compatible}
}

// NewIPv4OnlyCompatibility returns an IPv4-only IPCompatibility value.
func NewIPv4OnlyCompatibility() IPCompatibility {
	return NewIPCompatibility(true, false)
}

// NewIPv6OnlyCompatibility returns an IPv6-only IPCompatibility value.
func NewIPv6OnlyCompatibility() IPCompatibility {
	return NewIPCompatibility(false, true)
}

// NewDualStackCompatibility returns a dual-stack IPCompatibility value.
func NewDualStackCompatibility() IPCompatibility {
	return NewIPCompatibility(true, true)
}

// IsIPv4Compatible returns the current IPv4 compatibility status.
func (ic *IPCompatibility) IsIPv4Compatible() bool {
	return ic.ipv4Compatible
}

// IsIPv6Compatible returns the current IPv6 compatibility status.
func (ic *IPCompatibility) IsIPv6Compatible() bool {
	return ic.ipv6Compatible
}

// IsIPv6Only checks if the IP compatibility is IPv6-only.
func (ic *IPCompatibility) IsIPv6Only() bool {
	return ic.IsIPv6Compatible() && !ic.IsIPv4Compatible()
}
