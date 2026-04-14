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

package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveCloudWatchLogsEndpoint(t *testing.T) {
	testCases := []struct {
		name          string
		region        string
		useDualStack  bool
		expectedError string
		expected      string
	}{
		{
			name:          "Empty region",
			region:        "",
			useDualStack:  true,
			expectedError: "region is required to resolve CloudWatch Logs endpoint",
		},
		// Standard AWS partition - dual-stack
		{
			name:         "Standard region dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: true,
			expected:     "https://logs.us-east-1.api.aws",
		},
		{
			name:         "Standard region dual-stack (eu-west-1)",
			region:       "eu-west-1",
			useDualStack: true,
			expected:     "https://logs.eu-west-1.api.aws",
		},
		{
			name:         "Standard region dual-stack (ap-northeast-1)",
			region:       "ap-northeast-1",
			useDualStack: true,
			expected:     "https://logs.ap-northeast-1.api.aws",
		},
		{
			name:         "Standard region dual-stack (sa-east-1)",
			region:       "sa-east-1",
			useDualStack: true,
			expected:     "https://logs.sa-east-1.api.aws",
		},
		// Standard AWS partition - non-dual-stack
		{
			name:         "Standard region non-dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: false,
			expected:     "https://logs.us-east-1.amazonaws.com",
		},
		{
			name:         "Standard region non-dual-stack (eu-central-1)",
			region:       "eu-central-1",
			useDualStack: false,
			expected:     "https://logs.eu-central-1.amazonaws.com",
		},
		{
			name:         "Standard region non-dual-stack (ap-south-1)",
			region:       "ap-south-1",
			useDualStack: false,
			expected:     "https://logs.ap-south-1.amazonaws.com",
		},
		// China partition - dual-stack
		{
			name:         "China region dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: true,
			expected:     "https://logs.cn-north-1.api.amazonwebservices.com.cn",
		},
		{
			name:         "China region dual-stack (cn-northwest-1)",
			region:       "cn-northwest-1",
			useDualStack: true,
			expected:     "https://logs.cn-northwest-1.api.amazonwebservices.com.cn",
		},
		// China partition - non-dual-stack
		{
			name:         "China region non-dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: false,
			expected:     "https://logs.cn-north-1.amazonaws.com.cn",
		},
		{
			name:         "China region non-dual-stack (cn-northwest-1)",
			region:       "cn-northwest-1",
			useDualStack: false,
			expected:     "https://logs.cn-northwest-1.amazonaws.com.cn",
		},
		// AWS GovCloud partition - dual-stack
		{
			name:         "GovCloud region dual-stack (us-gov-east-1)",
			region:       "us-gov-east-1",
			useDualStack: true,
			expected:     "https://logs.us-gov-east-1.api.aws",
		},
		{
			name:         "GovCloud region dual-stack (us-gov-west-1)",
			region:       "us-gov-west-1",
			useDualStack: true,
			expected:     "https://logs.us-gov-west-1.api.aws",
		},
		// AWS GovCloud partition - non-dual-stack
		{
			name:         "GovCloud region non-dual-stack (us-gov-west-1)",
			region:       "us-gov-west-1",
			useDualStack: false,
			expected:     "https://logs.us-gov-west-1.amazonaws.com",
		},
		{
			name:         "GovCloud region non-dual-stack (us-gov-east-1)",
			region:       "us-gov-east-1",
			useDualStack: false,
			expected:     "https://logs.us-gov-east-1.amazonaws.com",
		},
		// AWS Isolated partition - non-dual-stack
		{
			name:         "Isolated region non-dual-stack (us-iso-east-1)",
			region:       "us-iso-east-1",
			useDualStack: false,
			expected:     "https://logs.us-iso-east-1.c2s.ic.gov",
		},
		{
			name:         "Isolated region non-dual-stack (us-iso-west-1)",
			region:       "us-iso-west-1",
			useDualStack: false,
			expected:     "https://logs.us-iso-west-1.c2s.ic.gov",
		},
		// AWS Isolated partition - dual-stack (not supported)
		{
			name:         "Isolated region dual-stack (us-iso-east-1)",
			region:       "us-iso-east-1",
			useDualStack: true,
			expectedError: "failed to resolve CloudWatch Logs endpoint for region 'us-iso-east-1':" +
				" endpoint rule error, DualStack is enabled but this partition does not support DualStack",
		},
		// AWS Isolated-E partition - non-dual-stack
		{
			name:         "Isolated-E region non-dual-stack (eu-isoe-west-1)",
			region:       "eu-isoe-west-1",
			useDualStack: false,
			expected:     "https://logs.eu-isoe-west-1.cloud.adc-e.uk",
		},
		// AWS Isolated-E partition - dual-stack (not supported)
		{
			name:         "Isolated-E region dual-stack (eu-isoe-west-1)",
			region:       "eu-isoe-west-1",
			useDualStack: true,
			expectedError: "failed to resolve CloudWatch Logs endpoint for region 'eu-isoe-west-1':" +
				" endpoint rule error, DualStack is enabled but this partition does not support DualStack",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := ResolveCloudWatchLogsEndpoint(tc.region, tc.useDualStack)

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, endpoint)
			}
		})
	}
}
