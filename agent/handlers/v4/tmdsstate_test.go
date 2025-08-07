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

package v4

import (
	"testing"

	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
	tmdsv4 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v4/state"

	"github.com/stretchr/testify/assert"
)

func TestSortContainersCNIPauseFirst(t *testing.T) {
	// Local constants for container types used in tests
	const (
		cniPause        = "CNI_PAUSE"
		normal          = "NORMAL"
		emptyHostVolume = "EMPTY_HOST_VOLUME"
	)

	testCases := []struct {
		name  string
		input []string // Container types
		want  []string // Expected order of types
	}{
		{
			name:  "CNI_PAUSE containers appear first",
			input: []string{normal, cniPause, normal, cniPause},
			want:  []string{cniPause, cniPause, normal, normal},
		},
		{
			name:  "only CNI_PAUSE containers",
			input: []string{cniPause, cniPause},
			want:  []string{cniPause, cniPause},
		},
		{
			name:  "only normal containers",
			input: []string{normal, normal},
			want:  []string{normal, normal},
		},
		{
			name:  "empty list",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "mixed types",
			input: []string{normal, cniPause, emptyHostVolume},
			want:  []string{cniPause, normal, emptyHostVolume},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert input to ContainerResponse slice
			containers := make([]tmdsv4.ContainerResponse, len(tc.input))
			for i, containerType := range tc.input {
				containers[i] = tmdsv4.ContainerResponse{
					ContainerResponse: &v2.ContainerResponse{Type: containerType},
				}
			}

			// Sort the containers
			sortContainersCNIPauseFirst(containers)

			// Extract types from result
			result := make([]string, len(containers))
			for i, container := range containers {
				result[i] = container.ContainerResponse.Type
			}

			// Verify the result
			assert.Equal(t, tc.want, result)
		})
	}
}
