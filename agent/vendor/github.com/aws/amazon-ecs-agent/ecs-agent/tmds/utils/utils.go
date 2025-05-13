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
package utils

import (
	"net"
	"strings"
)

type IPType int

const (
	IPv4 IPType = iota
	IPv6
)

func IsIPv4(s string) bool {
	parsedIP := net.ParseIP(s)
	return parsedIP != nil && parsedIP.To4() != nil
}

func IsIPv4CIDR(s string) bool {
	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		return false
	}

	return ip.To4() != nil
}

func IsIPv6(s string) bool {
	parsedIP := net.ParseIP(s)
	return parsedIP != nil && parsedIP.To4() == nil
}

func IsIPv6CIDR(s string) bool {
	if !strings.Contains(s, "/") {
		return false
	}

	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		return false
	}

	return ip.To4() == nil
}
