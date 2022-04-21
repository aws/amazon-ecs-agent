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
	"testing"

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
