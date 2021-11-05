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

package execcmd

import (
	"strconv"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/stretchr/testify/assert"
)

func TestGetSessionWorkersLimit(t *testing.T) {
	var tests = []struct {
		sessionLimit  int
		expectedLimit int
	}{
		{
			sessionLimit:  -2,
			expectedLimit: 2,
		},
		{
			sessionLimit:  0,
			expectedLimit: 2,
		},
		{
			sessionLimit:  1,
			expectedLimit: 1,
		},
		{
			sessionLimit:  2,
			expectedLimit: 2,
		},
	}
	for _, tc := range tests {
		ma := apicontainer.ManagedAgent{
			Properties: map[string]string{
				"sessionLimit": strconv.Itoa(tc.sessionLimit),
			},
		}
		limit := getSessionWorkersLimit(ma)
		assert.Equal(t, tc.expectedLimit, limit)
	}
}
