//go:build e2e
// +build e2e

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

package platform

import (
	"os"
	"testing"

	netutils "github.com/aws/amazon-ecs-agent/ecs-agent/utils/net"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
)

// Shared helpers for the platform e2e tests. These are referenced by the
// daemon network namespace e2e tests to select which IP-family sub-tests can
// run on the current host.

// getEnvOrDefaultE2E returns the value of the environment variable when set,
// and the provided default otherwise.
func getEnvOrDefaultE2E(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// hostIPCompatibilityE2E determines the host's IP-family support using the
// same route-based detection production code uses.
func hostIPCompatibilityE2E() (ipv4, ipv6 bool) {
	ipComp, err := netutils.DetermineIPCompatibility(netlinkwrapper.New(), "")
	if err != nil {
		return false, false
	}
	return ipComp.IsIPv4Compatible(), ipComp.IsIPv6Compatible()
}

// instanceSupportsIPv4E2E reports whether the host has IPv4 connectivity.
func instanceSupportsIPv4E2E() bool {
	v4, _ := hostIPCompatibilityE2E()
	return v4
}

// instanceSupportsIPv6E2E reports whether the host has IPv6 connectivity.
func instanceSupportsIPv6E2E() bool {
	_, v6 := hostIPCompatibilityE2E()
	return v6
}

// skipIfNoIPv4E2E skips the test when the host has no IPv4 connectivity.
func skipIfNoIPv4E2E(t *testing.T) {
	if !instanceSupportsIPv4E2E() {
		t.Skip("host has no IPv4 connectivity, skipping")
	}
}

// skipIfNotIPv6OnlyE2E skips the test unless the host is IPv6-only.
func skipIfNotIPv6OnlyE2E(t *testing.T) {
	v4, v6 := hostIPCompatibilityE2E()
	if !v6 || v4 {
		t.Skip("host is not IPv6-only, skipping")
	}
}

// skipIfNoDualStackE2E skips the test unless the host supports both IPv4 and IPv6.
func skipIfNoDualStackE2E(t *testing.T) {
	v4, v6 := hostIPCompatibilityE2E()
	if !v4 || !v6 {
		t.Skip("host is not dual-stack, skipping")
	}
}
