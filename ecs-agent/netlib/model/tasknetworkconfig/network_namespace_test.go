//go:build unit
// +build unit

package tasknetworkconfig

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetworkNamespace_GetPrimaryInterface(t *testing.T) {
	testCases := []struct {
		netns     *NetworkNamespace
		primaryNI string
	}{
		{
			netns: &NetworkNamespace{
				NetworkInterfaces: []*networkinterface.NetworkInterface{
					{
						Index: 1,
						Name:  secondaryInterfaceName,
					},
					{
						Index: 0,
						Name:  primaryInterfaceName,
					},
				},
			},
			primaryNI: primaryInterfaceName,
		},
		{
			netns:     &NetworkNamespace{},
			primaryNI: "",
		},
	}

	assert.Equal(t, tc.primaryNI, testCases[0].netns.GetPrimaryInterface().Name)
	assert.Nil(t, testCases[1].netns.GetPrimaryInterface())
}
