package ecscni

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/assert"
)

// mock_libcni is intended to mock the libcni from containernetworking/cni package
// it's only used to test
type mock_libcni struct {
	cniConfigList *libcni.NetworkConfigList
	runtimeConfig *libcni.RuntimeConf
}

func (cni *mock_libcni) AddNetworkList(net *libcni.NetworkConfigList, rt *libcni.RuntimeConf) (types.Result, error) {
	cni.cniConfigList = net
	cni.runtimeConfig = rt

	return nil, nil
}

func (cni *mock_libcni) DelNetworkList(net *libcni.NetworkConfigList, rt *libcni.RuntimeConf) error {
	cni.cniConfigList = net
	cni.runtimeConfig = rt

	return nil
}

func (mock_libcni) AddNetwork(net *libcni.NetworkConfig, rt *libcni.RuntimeConf) (types.Result, error) {
	return nil, nil
}

func (mock_libcni) DelNetwork(net *libcni.NetworkConfig, rt *libcni.RuntimeConf) error {
	return nil
}

// TestSetupConfig tests the ecscni has create correct configuration for
// bridge/eni/ipam plugin to set up the contaienr namespace
func TestSetupConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})
	mockLibcni := &mock_libcni{}
	ecscniClient.(*cniClient).libcni = mockLibcni

	setupConfig := &Config{
		ENIID:          "eni-12345678",
		ContainerID:    "containerid12",
		ContainerPID:   "pid",
		ENIIPV4Address: "172.31.21.40",
		ENIIPV6Address: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		ENIMACAddress:  "02:7b:64:49:b1:40",
		BridgeName:     "bridge-test1",
		VethName:       "eth3",
	}

	ecscniClient.SetupNS(setupConfig)
	networkConfigList := mockLibcni.cniConfigList
	runtimeConfig := mockLibcni.runtimeConfig

	assert.Equal(t, 2, len(networkConfigList.Plugins), "setupNS should call bridge plugin and eni plugin")
	assert.Equal(t, NetworkName, networkConfigList.Name)
	assert.Equal(t, ecscniClient.(*cniClient).cniVersion, networkConfigList.CNIVersion)

	assert.Equal(t, setupConfig.ContainerID, runtimeConfig.ContainerID)
	assert.Equal(t, fmt.Sprintf(netnsFormat, setupConfig.ContainerPID), runtimeConfig.NetNS)
	assert.Equal(t, setupConfig.VethName, runtimeConfig.IfName)

	bridgeConfig := &BridgeConfig{}
	eniConfig := &ENIConfig{}

	for _, plugin := range networkConfigList.Plugins {
		var err error
		if plugin.Network.Type == "bridge" {
			err = json.Unmarshal(plugin.Bytes, bridgeConfig)
		} else if plugin.Network.Type == "eni" {
			err = json.Unmarshal(plugin.Bytes, eniConfig)
		}
		assert.NoError(t, err, "unmarshal config from bytes failed")
	}

	assert.Equal(t, setupConfig.BridgeName, bridgeConfig.BridgeName)
	assert.True(t, bridgeConfig.IsGW)
	assert.Equal(t, ECSSubnet, bridgeConfig.IPAM.IPV4Subnet)
	assert.Equal(t, TaskIAMRoleEndpoint, bridgeConfig.IPAM.IPV4Routes[0].Dst.String())

	assert.Equal(t, setupConfig.ENIID, eniConfig.ENIID)
	assert.Equal(t, setupConfig.ENIIPV4Address, eniConfig.IPV4Address)
	assert.Equal(t, setupConfig.ENIIPV6Address, eniConfig.IPV6Address)
	assert.Equal(t, setupConfig.ENIMACAddress, eniConfig.MACAddress)
}

// TestSetupConfig tests the ecscni has create correct configuration for
// bridge/eni/ipam plugin to clean up the contaienr namespace
func TestCleanupConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})
	mockLibcni := &mock_libcni{}
	ecscniClient.(*cniClient).libcni = mockLibcni

	setupConfig := &Config{
		ENIID:         "eni-12345678",
		ContainerID:   "containerid12",
		ContainerPID:  "pid",
		IPAMV4Address: "169.254.170.20",
		ENIMACAddress: "02:7b:64:49:b1:40",
		BridgeName:    "bridge-test1",
		VethName:      "eth3",
	}

	ecscniClient.SetupNS(setupConfig)
	networkConfigList := mockLibcni.cniConfigList
	runtimeConfig := mockLibcni.runtimeConfig

	assert.Equal(t, 2, len(networkConfigList.Plugins), "setupNS should call bridge plugin and eni plugin")
	assert.Equal(t, NetworkName, networkConfigList.Name)
	assert.Equal(t, ecscniClient.(*cniClient).cniVersion, networkConfigList.CNIVersion)

	assert.Equal(t, setupConfig.ContainerID, runtimeConfig.ContainerID)
	assert.Equal(t, fmt.Sprintf(netnsFormat, setupConfig.ContainerPID), runtimeConfig.NetNS)
	assert.Equal(t, setupConfig.VethName, runtimeConfig.IfName)

	bridgeConfig := &BridgeConfig{}
	eniConfig := &ENIConfig{}

	for _, plugin := range networkConfigList.Plugins {
		var err error
		if plugin.Network.Type == "bridge" {
			err = json.Unmarshal(plugin.Bytes, bridgeConfig)
		} else if plugin.Network.Type == "eni" {
			err = json.Unmarshal(plugin.Bytes, eniConfig)
		}
		assert.NoError(t, err, "unmarshal config from bytes failed")
	}

	assert.Equal(t, setupConfig.BridgeName, bridgeConfig.BridgeName)
	assert.True(t, bridgeConfig.IsGW)
	assert.Equal(t, ECSSubnet, bridgeConfig.IPAM.IPV4Subnet)
	assert.Equal(t, TaskIAMRoleEndpoint, bridgeConfig.IPAM.IPV4Routes[0].Dst.String())
	assert.Equal(t, setupConfig.IPAMV4Address, bridgeConfig.IPAM.IPV4Address)

	assert.Equal(t, setupConfig.ENIID, eniConfig.ENIID)
	assert.Equal(t, setupConfig.ENIIPV4Address, eniConfig.IPV4Address)
	assert.Equal(t, setupConfig.ENIIPV6Address, eniConfig.IPV6Address)
	assert.Equal(t, setupConfig.ENIMACAddress, eniConfig.MACAddress)
}
