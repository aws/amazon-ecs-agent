package tasknetworkconfig

import (
	"github.com/stretchr/testify/assert"
	
	"testing"
)

func TestTaskNetworkConfig_GetPrimaryInterface(t *testing.T) {
	testNetConfig := getTestTaskNetworkConfig()
	assert.Equal(t, primaryInterfaceName, testNetConfig.GetPrimaryInterface().Name)

	testNetConfig = &TaskNetworkConfig{
		NetworkNamespaces: []*NetworkNamespace{},
	}
	assert.Nil(t, testNetConfig.GetPrimaryInterface())
}

func TestTaskNetworkConfig_GetPrimaryNetNS(t *testing.T) {
	testNetConfig := getTestTaskNetworkConfig()
	assert.Equal(t, primaryNetNSName, testNetConfig.GetPrimaryNetNS().Name)

	testNetConfig = &TaskNetworkConfig{}
	assert.Nil(t, testNetConfig.GetPrimaryNetNS())
}
