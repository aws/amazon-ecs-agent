// +build linux,unit

// Copyright 2014-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ecscni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api/eni"
	mock_libcni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks_libcni"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSetupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSENIPluginName, net.Network.Type, "first plugin should be eni")
			}),
		// Bridge plugin was called second
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "second plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
	)

	_, err = ecscniClient.SetupNS(context.TODO(), &Config{AdditionalLocalRoutes: additionalRoutes}, time.Second)
	assert.NoError(t, err)
}

func TestSetupNSTrunk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBranchENIPluginName, net.Network.Type, "first plugin should be eni")
			}),
		// Bridge plugin was called last
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "second plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
	)

	_, err = ecscniClient.SetupNS(context.TODO(), &Config{InterfaceAssociationProtocol: eni.VLANInterfaceAssociationProtocol,
		AdditionalLocalRoutes: additionalRoutes, SubnetGatewayIPV4Address: "172.31.1.1/20"}, time.Second)
	assert.NoError(t, err)
}

func TestSetupNSAppMeshEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSENIPluginName, net.Network.Type, "first plugin should be eni")
			}),
		// Bridge plugin was called second
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "second plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
				assert.Len(t, bridgeConfig.IPAM.IPV4Routes, 3, "default route plus two extra routes")
			}),
		// AppMesh plugin was called third
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSAppMeshPluginName, net.Network.Type, "third plugin should be app mesh")
			}),
	)

	_, err = ecscniClient.SetupNS(context.TODO(), &Config{AdditionalLocalRoutes: additionalRoutes, AppMeshCNIEnabled: true}, time.Second)
	assert.NoError(t, err)
}

func TestSetupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// ENI plugin was called first
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, errors.New("timeout")).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
			}).MaxTimes(1),
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).MaxTimes(1),
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).MaxTimes(1),
	)

	_, err := ecscniClient.SetupNS(context.TODO(), &Config{}, time.Millisecond)
	assert.Error(t, err)
}

func TestCleanupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	err = ecscniClient.CleanupNS(context.TODO(), &Config{AdditionalLocalRoutes: additionalRoutes}, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSTrunk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// Bridge plugin was called first
		libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBridgePluginName, net.Network.Type, "first plugin should be bridge")
				var bridgeConfig BridgeConfig
				err := json.Unmarshal(net.Bytes, &bridgeConfig)
				assert.NoError(t, err, "unmarshal BridgeConfig")
			}),
		// ENI plugin was called second
		libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSBranchENIPluginName, net.Network.Type, "second plugin should be eni")
			}),
	)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	err = ecscniClient.CleanupNS(context.TODO(), &Config{InterfaceAssociationProtocol: eni.VLANInterfaceAssociationProtocol,
		AdditionalLocalRoutes: additionalRoutes, SubnetGatewayIPV4Address: "172.31.1.1/20"}, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSAppMeshEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(3)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	err = ecscniClient.CleanupNS(context.TODO(), &Config{AdditionalLocalRoutes: additionalRoutes, AppMeshCNIEnabled: true}, time.Second)
	assert.NoError(t, err)
}

func TestCleanupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both bridge and eni plugin
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(x interface{}, y interface{}, z interface{}) {
		}).Return(errors.New("timeout")).MaxTimes(3)

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Millisecond)
	defer cancel()
	err = ecscniClient.CleanupNS(ctx, &Config{AdditionalLocalRoutes: additionalRoutes}, time.Millisecond)
	assert.Error(t, err)
}

func TestReleaseIPInIPAM(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient(&Config{})
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := ecscniClient.ReleaseIPResource(context.TODO(), &Config{}, time.Second)
	assert.NoError(t, err)
}

// TestConstructENINetworkConfig tests createENINetworkConfig creates the correct
// configuration for eni plugin
func TestConstructENINetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		ENIID:                    "eni-12345678",
		ContainerID:              "containerid12",
		ContainerPID:             "pid",
		ENIIPV4Address:           "172.31.21.40",
		ENIIPV6Address:           "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		ENIMACAddress:            "02:7b:64:49:b1:40",
		BlockInstanceMetdata:     true,
		SubnetGatewayIPV4Address: "172.31.1.1/20",
	}

	_, eniNetworkConfig, err := ecscniClient.(*cniClient).createENINetworkConfig(config)
	assert.NoError(t, err, "construct eni network config failed")
	eniConfig := &ENIConfig{}
	err = json.Unmarshal(eniNetworkConfig.Bytes, eniConfig)
	assert.NoError(t, err, "unmarshal config from bytes failed")

	assert.Equal(t, config.ENIID, eniConfig.ENIID)
	assert.Equal(t, config.ENIIPV4Address, eniConfig.IPV4Address)
	assert.Equal(t, config.ENIIPV6Address, eniConfig.IPV6Address)
	assert.Equal(t, config.ENIMACAddress, eniConfig.MACAddress)
	assert.True(t, eniConfig.BlockInstanceMetdata)
	assert.Equal(t, config.SubnetGatewayIPV4Address, eniConfig.SubnetGatewayIPV4Address)
}

// TestConstructBranchENINetworkConfig tests createBranchENINetworkConfig creates the correct
// configuration for eni plugin
func TestConstructBranchENINetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		ENIID:                    "eni-12345678",
		ContainerID:              "containerid12",
		ContainerPID:             "pid",
		ENIIPV4Address:           "172.31.21.40",
		ENIIPV6Address:           "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		ENIMACAddress:            "02:7b:64:49:b1:40",
		BlockInstanceMetdata:     true,
		SubnetGatewayIPV4Address: "172.31.1.1/20",
	}

	_, eniNetworkConfig, err := ecscniClient.(*cniClient).createBranchENINetworkConfig(config)
	assert.NoError(t, err, "construct eni network config failed")
	branchENIConfig := &BranchENIConfig{}
	err = json.Unmarshal(eniNetworkConfig.Bytes, branchENIConfig)
	assert.NoError(t, err, "unmarshal config from bytes failed")

	assert.Equal(t, config.BranchVlanID, branchENIConfig.BranchVlanID)
	assert.Equal(t, "172.31.21.40/20", branchENIConfig.BranchIPAddress)
	assert.Equal(t, config.ENIMACAddress, branchENIConfig.BranchMACAddress)
	assert.Equal(t, "172.31.1.1", branchENIConfig.BranchGatewayIPAddress)
}

// TestConstructBridgeNetworkConfigWithoutIPAM tests createBridgeNetworkConfigWithoutIPAM creates the right configuration for bridge plugin
func TestConstructBridgeNetworkConfigWithoutIPAM(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		ContainerID:  "containerid12",
		ContainerPID: "pid",
		BridgeName:   "bridge-test1",
	}

	_, bridgeNetworkConfig, err := ecscniClient.(*cniClient).createBridgeNetworkConfigWithoutIPAM(config)
	assert.NoError(t, err, "construct bridge network config failed")
	bridgeConfig := &BridgeConfig{}
	err = json.Unmarshal(bridgeNetworkConfig.Bytes, bridgeConfig)

	assert.NoError(t, err, "unmarshal bridge config from bytes failed")
	assert.Equal(t, config.BridgeName, bridgeConfig.BridgeName)
	assert.Equal(t, IPAMConfig{}, bridgeConfig.IPAM)
}

// TestConstructAppMeshNetworkConfig tests createAppMeshConfig creates the correct
// configuration for app mesh plugin
func TestConstructAppMeshNetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		AppMeshCNIEnabled: true,
		IgnoredUID:        "1337",
		IgnoredGID:        "1448",
		ProxyIngressPort:  "15000",
		ProxyEgressPort:   "15001",
		AppPorts: []string{
			"9000",
		},
		EgressIgnoredPorts: []string{
			"9001",
		},
		EgressIgnoredIPs: []string{
			"169.254.169.254",
		},
	}

	_, appMeshNetworkConfig, err := ecscniClient.(*cniClient).createAppMeshConfig(config)

	assert.NoError(t, err, "construct eni network config failed")
	appMeshConfig := &AppMeshConfig{}
	err = json.Unmarshal(appMeshNetworkConfig.Bytes, appMeshConfig)
	assert.NoError(t, err, "unmarshal config from bytes failed")

	assert.Equal(t, config.IgnoredUID, appMeshConfig.IgnoredUID)
	assert.Equal(t, config.IgnoredGID, appMeshConfig.IgnoredGID)
	assert.Equal(t, config.ProxyIngressPort, appMeshConfig.ProxyIngressPort)
	assert.Equal(t, config.ProxyEgressPort, appMeshConfig.ProxyEgressPort)
	assert.Equal(t, len(config.ProxyEgressPort), len(appMeshConfig.ProxyEgressPort))
	assert.Equal(t, config.ProxyEgressPort[0], appMeshConfig.ProxyEgressPort[0])
	assert.Equal(t, len(config.EgressIgnoredIPs), len(appMeshConfig.EgressIgnoredIPs))
	assert.Equal(t, config.EgressIgnoredIPs[0], appMeshConfig.EgressIgnoredIPs[0])
}

func TestConstructIPAMNetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	config := &Config{
		ID:                       "02:7b:64:49:b1:40",
		ENIID:                    "eni-12345678",
		ContainerID:              "containerid12",
		ContainerPID:             "pid",
		ENIIPV4Address:           "172.31.21.40",
		ENIIPV6Address:           "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		ENIMACAddress:            "02:7b:64:49:b1:40",
		BlockInstanceMetdata:     true,
		SubnetGatewayIPV4Address: "172.31.1.1/20",
	}

	_, networkConfig, err := ecscniClient.(*cniClient).createIPAMNetworkConfig(config)
	assert.NoError(t, err, "construct network config failed")
	ipamNetworkConfig := &IPAMNetworkConfig{}
	err = json.Unmarshal(networkConfig.Bytes, ipamNetworkConfig)
	assert.NoError(t, err, "unmarshal config from bytes failed")

	assert.Equal(t, ECSIPAMPluginName, ipamNetworkConfig.Name)
	assert.Equal(t, ECSIPAMPluginName, ipamNetworkConfig.Type)
	assert.NotNil(t, ipamNetworkConfig.IPAM)
	assert.Equal(t, ECSIPAMPluginName, ipamNetworkConfig.IPAM.Type)
	assert.Equal(t, config.ENIMACAddress, ipamNetworkConfig.IPAM.ID)
	assert.Equal(t, 1, len(ipamNetworkConfig.IPAM.IPV4Routes))
	assert.Equal(t, TaskIAMRoleEndpoint, ipamNetworkConfig.IPAM.IPV4Routes[0].Dst.String())
}

// TestConstructBridgeNetworkConfigWithIPAM tests createBridgeNetworkConfigWithIPAM
// creates the correct configuration for bridge and ipam plugin
func TestConstructNetworkConfig(t *testing.T) {
	ecscniClient := NewClient(&Config{})

	additionalRoutesJson := `["169.254.172.1/32", "10.11.12.13/32"]`
	var additionalRoutes []cnitypes.IPNet
	err := json.Unmarshal([]byte(additionalRoutesJson), &additionalRoutes)
	assert.NoError(t, err)

	config := &Config{
		ContainerID:           "containerid12",
		ContainerPID:          "pid",
		BridgeName:            "bridge-test1",
		AdditionalLocalRoutes: additionalRoutes,
	}

	_, bridgeNetworkConfig, err := ecscniClient.(*cniClient).createBridgeNetworkConfigWithIPAM(config)
	assert.NoError(t, err, "construct bridge plugins configuration failed")

	bridgeConfig := &BridgeConfig{}
	err = json.Unmarshal(bridgeNetworkConfig.Bytes, bridgeConfig)
	assert.NoError(t, err, "unmarshal bridge config from bytes failed: %s", string(bridgeNetworkConfig.Bytes))

	assert.Equal(t, config.BridgeName, bridgeConfig.BridgeName)
	assert.Equal(t, ecsSubnet, bridgeConfig.IPAM.IPV4Subnet)
	assert.Equal(t, TaskIAMRoleEndpoint, bridgeConfig.IPAM.IPV4Routes[0].Dst.String())
}

func TestCNIPluginVersion(t *testing.T) {
	testCases := []struct {
		version *cniPluginVersion
		str     string
	}{
		{
			version: &cniPluginVersion{
				Version: "1",
				Dirty:   false,
				Hash:    "hash",
			},
			str: "hash-1",
		},
		{
			version: &cniPluginVersion{
				Version: "1",
				Dirty:   true,
				Hash:    "hash",
			},
			str: "@hash-1",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("version string %s", tc.str), func(t *testing.T) {
			assert.Equal(t, tc.str, tc.version.str())
		})
	}
}

// Asserts that CNI plugin version matches the expected version
func TestCNIPluginVersionNumber(t *testing.T) {
	versionStr := getCNIVersionString(t)
	assert.Equal(t, currentECSCNIVersion, versionStr)
}

// Asserts that CNI plugin version is upgraded when new commits are made to CNI plugin submodule
func TestCNIPluginVersionUpgrade(t *testing.T) {
	versionStr := getCNIVersionString(t)
	cmd := exec.Command("git", "submodule")
	versionInfo, err := cmd.Output()
	assert.NoError(t, err, "Error running the command: git submodule")
	versionInfoStrList := strings.Split(string(versionInfo), "\n")
	// If a new commit is added, version should be upgraded
	if currentECSCNIGitHash != strings.Split(versionInfoStrList[0], " ")[1] {
		assert.NotEqual(t, currentECSCNIVersion, versionStr)
	}
	assert.Equal(t, currentVPCCNIGitHash, strings.Split(versionInfoStrList[1], " ")[1])
}

// Returns the version in CNI plugin VERSION file as a string
func getCNIVersionString(t *testing.T) string {
	// ../../amazon-ecs-cni-plugins/VERSION
	versionFilePath := filepath.Clean(filepath.Join("..", "..", "amazon-ecs-cni-plugins", "VERSION"))
	versionStr, err := ioutil.ReadFile(versionFilePath)
	assert.NoError(t, err, "Error reading the CNI plugin version file")
	return strings.TrimSpace(string(versionStr))
}
