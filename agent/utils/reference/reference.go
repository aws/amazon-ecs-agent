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

// Utilities for container image reference strings.
package reference

import (
	"github.com/docker/distribution/reference"
	"github.com/opencontainers/go-digest"
)

// Helper function to parse an image reference and get digest from it if found.
// The caller must check that the returned digest is non-empty before using it.
func GetDigestFromImageRef(imageRef string) digest.Digest {
	parsedRef, err := reference.Parse(imageRef)
	if err != nil {
		return ""
	}
	switch v := parsedRef.(type) {
	case reference.Digested:
		return v.Digest()
	default:
		return ""
	}
}
