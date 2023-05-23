//go:build windows && unit
// +build windows,unit

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

package ecscni

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	mock_libcni "github.com/aws/amazon-ecs-agent/agent/ecscni/mocks_libcni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/eni"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	eniID                       = "eni-12345678"
	eniIPV4Address              = "172.31.21.40"
	eniMACAddress               = "02:7b:64:49:b1:40"
	eniSubnetGatewayIPV4Address = "172.31.1.1/20"
)

// getNetworkConfig creates and returns a sample network config for task namespace setup.
func getNetworkConfig() *Config {
	config := &Config{
		NetworkConfigs: []*NetworkConfig{},
	}

	eniNetworkConfig, _ := NewVPCENIPluginConfigForTaskNSSetup(getTestENI(), config)
	ecsBridgeConfig, _ := NewVPCENIPluginConfigForECSBridgeSetup(config)

	config.NetworkConfigs = append(config.NetworkConfigs,
		&NetworkConfig{
			IfName:           "eth0",
			CNINetworkConfig: eniNetworkConfig,
		},
		&NetworkConfig{
			IfName:           "eth1",
			CNINetworkConfig: ecsBridgeConfig,
		},
	)
	return config
}

// getTestENI returns a sample ENI for the task.
func getTestENI() *eni.ENI {
	return &eni.ENI{
		ID:       eniID,
		LinkName: eniID,
		IPV4Addresses: []*eni.ENIIPV4Address{
			{Address: eniIPV4Address, Primary: true},
		},
		MacAddress:               eniMACAddress,
		SubnetGatewayIPV4Address: eniSubnetGatewayIPV4Address,
	}
}

// TestSetupNS is used to test if the namespace is setup properly as per the provided configuration
func TestSetupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// vpc-eni plugin called to setup task namespace.
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSVPCENIPluginExecutable, net.Network.Type, "plugin should be vpc-eni")
			}).Times(2),
	)

	config := getNetworkConfig()
	_, err := ecscniClient.SetupNS(context.TODO(), config, time.Second)

	assert.NoError(t, err)
}

// TestSetupNSTimeout tests the behavior when CNI plugin invocation returns an error.
func TestSetupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Override the maximum retry timeout for the tests
	setupNSBackoffMax = setupNSBackoffMin

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// vpc-eni plugin will be called.
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, errors.New("timeout")).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
			}).MaxTimes(setupNSMaxRetryCount),
	)

	config := getNetworkConfig()
	_, err := ecscniClient.SetupNS(context.TODO(), config, time.Millisecond)

	assert.Error(t, err)
}

// TestSetupNSWithRetry tests the behavior when CNI plugin invocation returns an error and then succeeds in retry.
func TestSetupNSWithRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	gomock.InOrder(
		// First invocation of the plugin for setupNS will fail and the same will succeed in retry.
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, errors.New("timeout")).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
			}),
		libcniClient.EXPECT().AddNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(&current.Result{}, nil).Do(
			func(ctx context.Context, net *libcni.NetworkConfig, rt *libcni.RuntimeConf) {
				assert.Equal(t, ECSVPCENIPluginExecutable, net.Network.Type, "first plugin should be vpc-eni")
			}).Times(2),
	)

	config := getNetworkConfig()
	_, err := ecscniClient.SetupNS(context.TODO(), config, setupNSBackoffMax)

	assert.NoError(t, err)
}

// TestCleanupNS tests the cleanup of the task namespace when CleanupNS is called.
func TestCleanupNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)

	config := getNetworkConfig()
	err := ecscniClient.CleanupNS(context.TODO(), config, time.Second)

	assert.NoError(t, err)
}

// TestCleanupNSTimeout tests the behavior of CleanupNS when we get an error from CNI invocation
func TestCleanupNSTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ecscniClient := NewClient("")
	libcniClient := mock_libcni.NewMockCNI(ctrl)
	ecscniClient.(*cniClient).libcni = libcniClient

	// This will be called for both the endpoints.
	libcniClient.EXPECT().DelNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(x interface{}, y interface{}, z interface{}) {
		}).Return(errors.New("timeout")).MaxTimes(2)

	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Millisecond)
	defer cancel()

	config := getNetworkConfig()
	err := ecscniClient.CleanupNS(ctx, config, time.Millisecond)

	assert.Error(t, err)
}

// TestConstructNetworkConfig tests if we create an appropriate config for setting up task namespace.
func TestConstructNetworkConfig(t *testing.T) {
	config := getNetworkConfig()
	taskENI := getTestENI()

	// Config for task ns setup to use task ENI.
	taskENIConfig := &VPCENIPluginConfig{}
	err := json.Unmarshal(config.NetworkConfigs[0].CNINetworkConfig.Bytes, taskENIConfig)
	require.NoError(t, err, "unmarshal task eni config from bytes failed")

	subnet := strings.Split(eniSubnetGatewayIPV4Address, "/")
	ipv4Addr := fmt.Sprintf("%s/%s", taskENI.GetPrimaryIPv4Address(), subnet[1])
	assert.Equal(t, ECSVPCENIPluginName, taskENIConfig.Type)
	assert.Equal(t, TaskHNSNetworkNamePrefix, config.NetworkConfigs[0].CNINetworkConfig.Network.Name)
	assert.Equal(t, eniID, taskENIConfig.ENIName)
	assert.EqualValues(t, []string{ipv4Addr}, taskENIConfig.ENIIPAddresses)
	assert.EqualValues(t, taskENI.MacAddress, taskENIConfig.ENIMACAddress)
	assert.EqualValues(t, []string{subnet[0]}, taskENIConfig.GatewayIPAddresses)
	assert.False(t, taskENIConfig.UseExistingNetwork)

	// Config for ecs-bridge endpoint setup for the task.
	ecsBridgeConfig := &VPCENIPluginConfig{}
	err = json.Unmarshal(config.NetworkConfigs[1].CNINetworkConfig.Bytes, ecsBridgeConfig)
	require.NoError(t, err, "unmarshal ecs-bridge config from bytes failed")
	assert.Equal(t, ECSVPCENIPluginName, ecsBridgeConfig.Type)
	assert.Equal(t, ECSBridgeNetworkName, config.NetworkConfigs[1].CNINetworkConfig.Network.Name)
	assert.True(t, ecsBridgeConfig.UseExistingNetwork)
}

// TestCNIPluginVersion tests if the string generated by version is correct
func TestCNIPluginVersion(t *testing.T) {
	testCases := []struct {
		version *cniPluginVersion
		str     string
	}{
		{
			version: &cniPluginVersion{
				Version:      "1",
				GitShortHash: "abcd",
				Built:        "July",
			},
			str: "abcd-1",
		},
		{
			version: &cniPluginVersion{
				Version:      "1",
				GitShortHash: "abcdef",
				Built:        "June",
			},
			str: "abcdef-1",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("version string %s", tc.str), func(t *testing.T) {
			assert.Equal(t, tc.str, tc.version.str())
		})
	}
}
