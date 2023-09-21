//go:build unit
// +build unit

package tasknetworkconfig

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetworkNamespace_GetPrimaryInterface(t *testing.T) {
	netns := &NetworkNamespace{
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
	}
	assert.Equal(t, primaryInterfaceName, netns.GetPrimaryInterface().Name)

	netns = &NetworkNamespace{}
	assert.Empty(t, netns.GetPrimaryInterface())
}
