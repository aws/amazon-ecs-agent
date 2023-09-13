package status

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
			str:    "NONE",
		},
		{
			status: NetworkReady,
			str:    "NONE",
		},
		{
			status: NetworkDeleted,
			str:    "NONE",
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

