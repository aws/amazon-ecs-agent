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

func TestGetDigestFromRepoDigests(t *testing.T) {
	testDigest1 := "sha256:c5b1261d6d3e43071626931fc004f70149baeba2c8ec672bd4f27761f8e1ad6b"
	testDigest2 := "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966"
	tcs := []struct {
		name           string
		repoDigests    []string
		imageRef       string
		expectedDigest string
		expectedError  string
	}{
		{
			name: "repo digest matching imageRef is used - dockerhub",
			repoDigests: []string{
				"public.ecr.aws/library/alpine@" + testDigest1,
				"alpine@" + testDigest2,
			},
			imageRef:       "alpine",
			expectedDigest: testDigest2,
		},
		{
			name: "docker.io prefix",
			repoDigests: []string{
				"alpine@" + testDigest2,
			},
			imageRef:       "docker.io/alpine",
			expectedDigest: testDigest2,
		},
		{
			name: "docker.io with library prefix",
			repoDigests: []string{
				"alpine@" + testDigest2,
			},
			imageRef:       "docker.io/library/alpine",
			expectedDigest: testDigest2,
		},
		{
			name: "docker.io with non-standard image",
			repoDigests: []string{
				"repo/image@" + testDigest2,
			},
			imageRef:       "docker.io/repo/image",
			expectedDigest: testDigest2,
		},
		{
			name: "registry-1.docker.io prefix",
			repoDigests: []string{
				"registry-1.docker.io/library/alpine@" + testDigest2,
			},
			imageRef:       "registry-1.docker.io/library/alpine",
			expectedDigest: testDigest2,
		},
		{
			name: "repo digest matching imageRef is used - ecr",
			repoDigests: []string{
				"public.ecr.aws/library/alpine@" + testDigest1,
				"alpine@" + testDigest2,
			},
			imageRef:       "public.ecr.aws/library/alpine",
			expectedDigest: testDigest1,
		},
		{
			name: "tags in image ref are ignored - dockerhub",
			repoDigests: []string{
				"public.ecr.aws/library/alpine@" + testDigest1,
				"alpine@" + testDigest2,
			},
			imageRef:       "alpine:edge",
			expectedDigest: testDigest2,
		},
		{
			name: "tags in image ref are ignored - ecr",
			repoDigests: []string{
				"public.ecr.aws/library/alpine@" + testDigest1,
				"alpine@" + testDigest2,
			},
			imageRef:       "public.ecr.aws/library/alpine:edge",
			expectedDigest: testDigest1,
		},
		{
			name:          "invalid image ref",
			imageRef:      "invalid format",
			expectedError: "failed to parse image reference 'invalid format': invalid reference format",
		},
		{
			name:          "image ref not named",
			imageRef:      "",
			expectedError: "failed to parse image reference '': invalid reference format",
		},
		{
			name:           "invalid repo digests are skipped",
			imageRef:       "alpine",
			repoDigests:    []string{"invalid format", "alpine@" + testDigest1},
			expectedDigest: testDigest1,
		},
		{
			name:          "no repo digests match image ref",
			imageRef:      "public.ecr.aws/library/alpine",
			repoDigests:   []string{"alpine@" + testDigest1},
			expectedError: "found no repo digest matching 'public.ecr.aws/library/alpine'",
		},
		{
			name:           "image reference can be canonical",
			repoDigests:    []string{"alpine@" + testDigest1},
			imageRef:       "alpine@" + testDigest2,
			expectedDigest: testDigest1,
		},
		{
			name:          "non-canonical repo digests are skipped",
			repoDigests:   []string{"alpine"},
			imageRef:      "alpine",
			expectedError: "found no repo digest matching 'alpine'",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			dgst, err := GetDigestFromRepoDigests(tc.repoDigests, tc.imageRef)
			if tc.expectedError == "" {
				require.NoError(t, err)
				expected, err := digest.Parse(tc.expectedDigest)
				require.NoError(t, err)
				assert.Equal(t, expected, dgst)
			} else {
				require.EqualError(t, err, tc.expectedError)
			}
		})
	}
}

func TestGetCanonicalRef(t *testing.T) {
	tcs := []struct {
		name           string
		imageRef       string
		manifestDigest string
		expected       string
		expectedError  string
	}{
		{
			name:           "invalid digest",
			imageRef:       "alpine",
			manifestDigest: "invalid digest",
			expectedError:  "failed to parse image digest 'invalid digest': invalid checksum digest format",
		},
		{
			name:           "invalid image reference format",
			imageRef:       "invalid reference",
			manifestDigest: "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			expectedError:  "failed to parse image reference 'invalid reference': invalid reference format",
		},
		{
			name:           "no tag",
			imageRef:       "alpine",
			manifestDigest: "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			expected:       "alpine@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
		{
			name:           "has tag",
			imageRef:       "alpine:latest",
			manifestDigest: "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			expected:       "alpine:latest@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
		{
			name:           "image reference's digest is overwritten",
			imageRef:       "alpine@sha256:c5b1261d6d3e43071626931fc004f70149baeba2c8ec672bd4f27761f8e1ad6b",
			manifestDigest: "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			expected:       "alpine@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
		{
			name:           "no tag ecr",
			imageRef:       "public.ecr.aws/library/alpine",
			manifestDigest: "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			expected:       "public.ecr.aws/library/alpine@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
		{
			name:           "has tag ecr",
			imageRef:       "public.ecr.aws/library/alpine:latest",
			manifestDigest: "sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
			expected:       "public.ecr.aws/library/alpine:latest@sha256:c3839dd800b9eb7603340509769c43e146a74c63dca3045a8e7dc8ee07e53966",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			canonicalRef, err := GetCanonicalRef(tc.imageRef, tc.manifestDigest)
			if tc.expectedError == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, canonicalRef.String())
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}
