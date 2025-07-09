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

func TestResolveSSMMessagesDualStackEndpoint(t *testing.T) {
	testCases := []struct {
		name          string
		region        string
		expectedError string
		expected      string
	}{
		{
			name:          "Empty region",
			region:        "",
			expectedError: "region is required to resolve ssmmessages dual stack endpoint",
		},
		// Standard AWS partition - dual-stack
		{
			name:     "Standard region (us-east-1)",
			region:   "us-east-1",
			expected: "https://ssmmessages.us-east-1.api.aws",
		},
		{
			name:     "Standard region (us-west-2)",
			region:   "us-west-2",
			expected: "https://ssmmessages.us-west-2.api.aws",
		},
		// China partition - dual-stack
		{
			name:     "China region (cn-north-1)",
			region:   "cn-north-1",
			expected: "https://ssmmessages.cn-north-1.api.amazonwebservices.com.cn",
		},
		// AWS GovCloud partition - dual-stack
		{
			name:     "GovCloud region (us-gov-west-1)",
			region:   "us-gov-west-1",
			expected: "https://ssmmessages.us-gov-west-1.api.aws",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := ResolveSSMMessagesDualStackEndpoint(tc.region)

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, endpoint)
			}
		})
	}
}

func TestResolveEC2MessagesDualStackEndpoint(t *testing.T) {
	testCases := []struct {
		name          string
		region        string
		expectedError string
		expected      string
	}{
		{
			name:          "Empty region",
			region:        "",
			expectedError: "region is required to resolve ec2messages dual stack endpoint",
		},
		// Standard AWS partition - dual-stack
		{
			name:     "Standard region (us-east-1)",
			region:   "us-east-1",
			expected: "https://ec2messages.us-east-1.api.aws",
		},
		// China partition - dual-stack
		{
			name:     "China region (cn-north-1)",
			region:   "cn-north-1",
			expected: "https://ec2messages.cn-north-1.api.amazonwebservices.com.cn",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := ResolveEC2MessagesDualStackEndpoint(tc.region)

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, endpoint)
			}
		})
	}
}

func TestResolveS3Endpoint(t *testing.T) {
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
			expectedError: "region is required to resolve S3 endpoint",
		},
		// Standard AWS partition - dual-stack
		{
			name:         "Standard region dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: true,
			expected:     "https://s3.dualstack.us-east-1.amazonaws.com",
		},
		// Standard AWS partition - non-dual-stack
		{
			name:         "Standard region non-dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: false,
			expected:     "https://s3.us-east-1.amazonaws.com",
		},
		// China partition - dual-stack
		{
			name:         "China region dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: true,
			expected:     "https://s3.dualstack.cn-north-1.amazonaws.com.cn",
		},
		// China partition - non-dual-stack
		{
			name:         "China region non-dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: false,
			expected:     "https://s3.cn-north-1.amazonaws.com.cn",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := ResolveS3Endpoint(tc.region, tc.useDualStack)

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, endpoint)
			}
		})
	}
}

func TestResolveKMSEndpoint(t *testing.T) {
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
			expectedError: "region is required to resolve KMS endpoint",
		},
		// Standard AWS partition - dual-stack
		{
			name:         "Standard region dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: true,
			expected:     "https://kms.us-east-1.api.aws",
		},
		// Standard AWS partition - non-dual-stack
		{
			name:         "Standard region non-dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: false,
			expected:     "https://kms.us-east-1.amazonaws.com",
		},
		// China partition - dual-stack
		{
			name:         "China region dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: true,
			expected:     "https://kms.cn-north-1.api.amazonwebservices.com.cn",
		},
		// China partition - non-dual-stack
		{
			name:         "China region non-dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: false,
			expected:     "https://kms.cn-north-1.amazonaws.com.cn",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := ResolveKMSEndpoint(tc.region, tc.useDualStack)

			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, endpoint)
			}
		})
	}
}

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
		// Standard AWS partition - non-dual-stack
		{
			name:         "Standard region non-dual-stack (us-east-1)",
			region:       "us-east-1",
			useDualStack: false,
			expected:     "https://logs.us-east-1.amazonaws.com",
		},
		// China partition - dual-stack
		{
			name:         "China region dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: true,
			expected:     "https://logs.cn-north-1.api.amazonwebservices.com.cn",
		},
		// China partition - non-dual-stack
		{
			name:         "China region non-dual-stack (cn-north-1)",
			region:       "cn-north-1",
			useDualStack: false,
			expected:     "https://logs.cn-north-1.amazonaws.com.cn",
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
