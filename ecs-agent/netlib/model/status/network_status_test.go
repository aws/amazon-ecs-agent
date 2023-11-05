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

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNetworkStatus verifies the corresponding string values of each status
// is appropriate.
func TestNetworkStatus(t *testing.T) {
	testCases := []struct {
		status NetworkStatus
		str    string
	}{
		{
			status: NetworkNone,
			str:    "NONE",
		},
		{
			status: NetworkReadyPull,
			str:    "READY_PULL",
		},
		{
			status: NetworkReady,
			str:    "READY",
		},
		{
			status: NetworkDeleted,
			str:    "DELETED",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.str, func(t *testing.T) {
			assert.Equal(t, tc.str, (&tc.status).String())
		})
	}
}

// TestNetworkStatusOrder verifies that order of statuses are as required.
func TestNetworkStatusOrder(t *testing.T) {
	assert.True(t, NetworkNone.ENIStatusBackwards(NetworkReadyPull))
	assert.True(t, NetworkReadyPull.ENIStatusBackwards(NetworkReady))
	assert.True(t, NetworkReady.ENIStatusBackwards(NetworkDeleted))

	assert.False(t, NetworkReadyPull.ENIStatusBackwards(NetworkNone))
	assert.False(t, NetworkReady.ENIStatusBackwards(NetworkReadyPull))
	assert.False(t, NetworkDeleted.ENIStatusBackwards(NetworkReady))
}
