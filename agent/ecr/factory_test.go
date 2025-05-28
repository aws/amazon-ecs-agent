//go:build unit
// +build unit

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

package ecr

import (
	"fmt"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ipcompatibility"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
)

const (
	endpointOverride string = "api.ecr.us-west-2.amazonaws.com"
)

func TestGetClientConfigEndpointOverride(t *testing.T) {
	cases := []struct {
		Name             string
		EndpointOverride string
		IPCompatibility  ipcompatibility.IPCompatibility
	}{
		{
			Name:             "IPv4 with endpoint override",
			EndpointOverride: endpointOverride,
			IPCompatibility:  ipcompatibility.NewIPv4OnlyCompatibility(),
		},
		{
			Name:             "IPv4 without endpoint override",
			EndpointOverride: "",
			IPCompatibility:  ipcompatibility.NewIPv4OnlyCompatibility(),
		},
		{
			Name:             "IPv6 with endpoint override",
			EndpointOverride: endpointOverride,
			IPCompatibility:  ipcompatibility.NewIPv6OnlyCompatibility(),
		},
		{
			Name:             "IPv6 without endpoint override",
			EndpointOverride: "",
			IPCompatibility:  ipcompatibility.NewIPv6OnlyCompatibility(),
		},
		{
			Name:             "DualStack with endpoint override",
			EndpointOverride: endpointOverride,
			IPCompatibility:  ipcompatibility.NewIPCompatibility(true, true),
		},
		{
			Name:             "DualStack without endpoint override",
			EndpointOverride: "",
			IPCompatibility:  ipcompatibility.NewIPCompatibility(true, true),
		},
	}

	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			testAuthData := &apicontainer.ECRAuthData{
				Region:           "us-west-2",
				EndpointOverride: test.EndpointOverride,
				UseExecutionRole: false,
			}
			cfg, err := getClientConfig(nil, testAuthData, test.IPCompatibility.IsIPv6Only())

			assert.Nil(t, err)
			if test.EndpointOverride != "" {
				assert.Equal(t, fmt.Sprintf("https://%s", test.EndpointOverride), *cfg.BaseEndpoint)
			}

			for _, value := range cfg.ConfigSources {
				// Default state is unset
				var useDualStackEndpoint = aws.DualStackEndpointStateUnset
				// Enabled will only be set when no override is provided and IPv6-only
				if test.EndpointOverride == "" && test.IPCompatibility.IsIPv6Only() {
					useDualStackEndpoint = aws.DualStackEndpointStateEnabled
				}

				// config.LoadOptions contains details on if DualStack endpoint was enabled or not
				if loadOptions, ok := value.(config.LoadOptions); ok {
					assert.Equal(t, useDualStackEndpoint, loadOptions.UseDualStackEndpoint)
				}
			}
		})
	}
}
