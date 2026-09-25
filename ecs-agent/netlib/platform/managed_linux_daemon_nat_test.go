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

	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddDaemonBridgeNATRuleByIPFamily verifies the daemon-bridge MASQUERADE
// rule installed for each address family.
func TestAddDaemonBridgeNATRuleByIPFamily(t *testing.T) {
	cases := []struct {
		name          string
		ipComp        ipcompatibility.IPCompatibility
		wantV4, want6 bool
	}{
		{"ipv4-only", ipcompatibility.NewIPv4OnlyCompatibility(), true, false},
		{"ipv6-only", ipcompatibility.NewIPv6OnlyCompatibility(), false, true},
		{"dual-stack", ipcompatibility.NewDualStackCompatibility(), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v4, v6 []string
			origIptables, origSysctl := runIptablesCommand, runSysctlCommand
			defer func() { runIptablesCommand, runSysctlCommand = origIptables, origSysctl }()
			runIptablesCommand = func(executable string, args ...string) ([]byte, error) {
				joined := strings.Join(args, " ")
				// A check reports "absent" so the append is exercised.
				if strings.Contains(joined, " -C ") {
					return nil, assert.AnError
				}
				if strings.Contains(joined, "MASQUERADE") {
					if executable == ipv6Tables {
						v6 = append(v6, joined)
					} else {
						v4 = append(v4, joined)
					}
				}
				return nil, nil
			}
			runSysctlCommand = func(args ...string) ([]byte, error) { return nil, nil }

			ml := &managedLinux{}
			require.NoError(t, ml.addDaemonBridgeNATRule(tc.ipComp))

			if tc.wantV4 {
				require.Len(t, v4, 1, "one IPv4 MASQUERADE; got %v", v4)
				assert.Contains(t, v4[0], "-s "+DaemonBridgeIP)
				assert.Contains(t, v4[0], "! -d "+ECSSubNet)
			} else {
				assert.Empty(t, v4)
			}
			if tc.want6 {
				require.Len(t, v6, 1, "one IPv6 MASQUERADE; got %v", v6)
				assert.Contains(t, v6[0], "-s "+DaemonBridgeIPv6)
				assert.Contains(t, v6[0], "! -d "+ECSSubNetIPv6)
			} else {
				assert.Empty(t, v6)
			}
		})
	}
}
