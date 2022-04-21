//go:build unit

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

package serviceconnect

import (
	"math/rand"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/stretchr/testify/assert"
)

func TestDNSConfigToDockerExtraHostsFormat(t *testing.T) {
	tt := []struct {
		dnsConf         task.DNSConfig
		expectedRestult []string
	}{
		{
			dnsConf: task.DNSConfig{
				{
					HostName:    "my.test.host",
					IPV4Address: "169.254.1.1",
				},
				{
					HostName:    "my.test.host2",
					IPV6Address: "ff06::c3",
				},
				{
					HostName:    "my.test.host3",
					IPV4Address: "169.254.1.2",
					IPV6Address: "ff06::c4",
				},
			},
			expectedRestult: []string{
				"my.test.host:169.254.1.1",
				"my.test.host2:ff06::c3",
				"my.test.host3:169.254.1.2",
				"my.test.host3:ff06::c4",
			},
		},
		{
			dnsConf:         nil,
			expectedRestult: nil,
		},
	}

	for _, tc := range tt {
		res := DNSConfigToDockerExtraHostsFormat(tc.dnsConf)
		assert.Equal(t, tc.expectedRestult, res, "Wrong docker host config ")
	}
}

func TestGenerateEphemeralPortNumbers(t *testing.T) {
	// This number is just to "stress" this test by increasing the changes of collision, which should not be a problem
	// in prod, only around a dozen ports will be needed in the worst case.
	expectedPortsGenerated := 1000
	var excludePorts []int
	rand.Seed(time.Now().UnixNano())
	// Since absolute max containers for a task is 20, let's exclude 40 ports at random, 2 per container
	// in order to make a somewhat realistic test
	for i := 0; i < 40; i++ {
		port := rand.Intn(ephemeralPortMax-ephemeralPortMin+1) + ephemeralPortMin
		excludePorts = append(excludePorts, port)
	}
	toExcludeSet := map[int]struct{}{}
	for _, e := range excludePorts {
		toExcludeSet[e] = struct{}{}
	}
	ports, err := GenerateEphemeralPortNumbers(expectedPortsGenerated, excludePorts)
	assert.NoError(t, err)
	assert.Len(t, ports, expectedPortsGenerated, "Not enough ports generated")
	for _, port := range ports {
		_, ok := toExcludeSet[port]
		assert.False(t, ok, "Port collision detected")
		assert.Conditionf(t, func() (success bool) {
			return port >= ephemeralPortMin && port <= ephemeralPortMax
		}, "Port was generated outside the ephemeral range [%d-%d]", ephemeralPortMin, ephemeralPortMax)
	}
}
