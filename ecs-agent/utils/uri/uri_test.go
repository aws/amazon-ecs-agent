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

package uri

import (
	"testing"
)

func TestParseURI(t *testing.T) {
	testCases := []struct {
		name                   string
		uri                    string
		expectedMatchPattern   bool
		expectedMatchDualstack bool
		expectedFIPS           bool
	}{
		// Standard ECR image patterns - non-FIPS
		{
			name:                   "Standard ECR commercial",
			uri:                    "123456789012.dkr.ecr.us-west-2.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Standard ECR China",
			uri:                    "123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Standard ECR GovCloud",
			uri:                    "123456789012.dkr.ecr.us-gov-west-1.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Standard ECR ISO",
			uri:                    "123456789012.dkr.ecr.us-iso-east-1.c2s.ic.gov/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Standard ECR ISOB",
			uri:                    "123456789012.dkr.ecr.us-isob-east-1.sc2s.sgov.gov/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Standard Starport",
			uri:                    "123456789012.dkr.starport.us-west-2.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},

		// Standard ECR image patterns - with FIPS
		{
			name:                   "Standard ECR commercial with FIPS",
			uri:                    "123456789012.dkr.ecr-fips.us-west-2.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           true,
		},
		{
			name:                   "Standard ECR GovCloud with FIPS",
			uri:                    "123456789012.dkr.ecr-fips.us-gov-west-1.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           true,
		},
		{
			name:                   "Standard Starport with FIPS",
			uri:                    "123456789012.dkr.starport-fips.us-west-2.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   true,
			expectedMatchDualstack: false,
			expectedFIPS:           true,
		},

		// Dual-stack ECR image patterns - non-FIPS
		{
			name:                   "Dual-stack ECR commercial",
			uri:                    "123456789012.dkr-ecr.us-west-2.on.aws/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           false,
		},
		{
			name:                   "Dual-stack ECR China",
			uri:                    "123456789012.dkr-ecr.cn-north-1.on.amazonwebservices.com.cn/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           false,
		},
		{
			name:                   "Dual-stack ECR ISO",
			uri:                    "123456789012.dkr-ecr.us-iso-west-1.on.aws.ic.gov/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           false,
		},
		{
			name:                   "Dual-stack ECR ISOB",
			uri:                    "123456789012.dkr-ecr.us-isob-east-1.on.aws.scloud/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           false,
		},
		{
			name:                   "Dual-stack ECR ISOF",
			uri:                    "123456789012.dkr-ecr.us-isof-south-1.on.aws.hci.ic.gov/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           false,
		},
		{
			name:                   "Dual-stack ECR ISOE",
			uri:                    "123456789012.dkr-ecr.eu-isoe-west-1.on.cloud-aws.adc-e.uk/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           false,
		},

		// Dual-stack ECR image patterns - with FIPS
		{
			name:                   "Dual-stack ECR commercial with FIPS",
			uri:                    "123456789012.dkr-ecr-fips.us-west-2.on.aws/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           true,
		},
		{
			name:                   "Dual-stack ECR GovCloud with FIPS",
			uri:                    "123456789012.dkr-ecr-fips.us-gov-west-1.on.aws/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: true,
			expectedFIPS:           true,
		},

		// Edge cases and non-matching patterns
		{
			name:                   "Non-ECR Docker Hub image",
			uri:                    "docker.io/library/ubuntu:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Non-ECR private registry",
			uri:                    "private-registry.example.com/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Invalid ECR URI format",
			uri:                    "123456789012.ecr.us-west-2.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Invalid ECR Dual-stack URI format",
			uri:                    "123456789012.ecr-dkr.us-west-2.on.aws/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "URI with FIPS keyword but invalid format",
			uri:                    "ecr-fips.amazonaws.com/my-repo:latest",
			expectedMatchPattern:   false,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
		{
			name:                   "Empty URI",
			uri:                    "",
			expectedMatchPattern:   false,
			expectedMatchDualstack: false,
			expectedFIPS:           false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matchPattern, matchDualstack, isFIPS := ParseURI(tc.uri, ECRImagePattern, ECRDualStackImagePattern)

			if matchPattern != tc.expectedMatchPattern {
				t.Errorf("Expected matchPattern to be %v, got %v for URI: %s", tc.expectedMatchPattern, matchPattern, tc.uri)
			}

			if matchDualstack != tc.expectedMatchDualstack {
				t.Errorf("Expected matchDualstack to be %v, got %v for URI: %s", tc.expectedMatchDualstack, matchDualstack, tc.uri)
			}

			if isFIPS != tc.expectedFIPS {
				t.Errorf("Expected isFIPS to be %v, got %v for URI: %s", tc.expectedFIPS, isFIPS, tc.uri)
			}
		})
	}
}
