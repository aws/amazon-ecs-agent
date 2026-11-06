//go:build linux && unit
// +build linux,unit

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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDaemonNATArgsSourceIsTheDaemon verifies the daemon-bridge MASQUERADE rule
// matches only the daemon's own address as its source, for each family. The
// rule is what lets a daemon reach the network from behind the bridge; an
// interface-wide or subnet-wide source would also translate any other traffic
// that found its way onto the bridge subnet.
func TestDaemonNATArgsSourceIsTheDaemon(t *testing.T) {
	for _, tc := range []struct {
		name, daemon, subnet string
	}{
		{"ipv4", DaemonBridgeIP, ECSSubNet},
		{"ipv6", DaemonBridgeIPv6, ECSSubNetIPv6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := strings.Join(getDaemonNATArgs(tc.daemon, tc.subnet), " ")
			assert.Equal(t, "POSTROUTING -s "+tc.daemon+" ! -d "+tc.subnet+" -j MASQUERADE", rule)
		})
	}
}
