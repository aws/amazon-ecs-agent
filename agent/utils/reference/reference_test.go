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
package reference

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFromDigest(t *testing.T) {
	tcs := []struct {
		image          string
		expectedDigest string
	}{
		{"", ""},
		{"c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966", ""},
		{"sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966", ""},
		{"ubuntu", ""},
		{"ubuntu:latest", ""},
		{
			"ubuntu@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			"c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
		{"public.ecr.aws/library/ubuntu", ""},
		{"public.ecr.aws/library/ubuntu:latest", ""},
		{
			"public.ecr.aws/library/ubuntu@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			"c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.image, func(t *testing.T) {
			dgst := GetDigestFromImageRef(tc.image)
			if tc.expectedDigest == "" {
				assert.Empty(t, dgst)
			} else {
				expected, err := digest.Parse("sha256:" + tc.expectedDigest)
				require.NoError(t, err)
				assert.Equal(t, expected, dgst)
			}
		})
	}
}
