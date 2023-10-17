//go:build !windows && unit
// +build !windows,unit

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

package netlib

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"
	mock_platform "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNewNetworkBuilder(t *testing.T) {
	nbi, err := NewNetworkBuilder(platform.WarmpoolPlatform)
	nb := nbi.(*networkBuilder)
	require.NoError(t, err)
	require.NotNil(t, nb.platformAPI)

	nbi, err = NewNetworkBuilder("invalid-platform")
	require.Error(t, err)
	require.Nil(t, nbi)
}

// TestNetworkBuilder_BuildTaskNetworkConfiguration verifies for all known use cases,
// the network builder is able to translate the input task payload into the desired
// network data models.
func TestNetworkBuilder_BuildTaskNetworkConfiguration(t *testing.T) {
	t.Run("containerd-default", getTestFunc(getSingleNetNSAWSVPCTestData))
	t.Run("containerd-multi-interface", getTestFunc(getSingleNetNSMultiIfaceAWSVPCTestData))
	t.Run("containerd-multi-netns", getTestFunc(getMultiNetNSMultiIfaceAWSVPCTestData))
}

func TestNetworkBuilder_Start(t *testing.T) {
	t.Run("awsvpc", testNetworkBuilder_StartAWSVPC)
}

// getTestFunc returns a test function that verifies the capability of the networkBuilder
// to translate a given input task payload into desired network data models.
func getTestFunc(dataGenF func(string) (input *ecsacs.Task, expected tasknetworkconfig.TaskNetworkConfig)) func(*testing.T) {

	return func(t *testing.T) {
		// Create a networkBuilder for the warmpool platform.
		netBuilder, err := NewNetworkBuilder(platform.WarmpoolPlatform, nil, nil, "")
		require.NoError(t, err)

		// Generate input task payload and a reference to verify the output with.
		taskPayload, expectedConfig := dataGenF(taskID)

		// Invoke networkBuilder function for building the task network config.
		actualConfig, err := netBuilder.BuildTaskNetworkConfiguration(taskID, taskPayload)
		require.NoError(t, err)

		// Convert the obtained output and the reference data into json data to make it
		// easier to compare.
		expected, err := json.Marshal(expectedConfig)
		require.NoError(t, err)
		actual, err := json.Marshal(actualConfig)
		require.NoError(t, err)

		require.Equal(t, string(expected), string(actual))
	}
}

// getSingleNetNSAWSVPCTestData returns a task payload and a task network config
// to be used the input and reference result for tests. The reference object will
// has only one network namespace and network interface.
func getSingleNetNSAWSVPCTestData(testTaskID string) (*ecsacs.Task, tasknetworkconfig.TaskNetworkConfig) {
	enis, netIfs := getTestInterfacesData()
	taskPayload := &ecsacs.Task{
		NetworkMode:              aws.String(ecs.NetworkModeAwsvpc),
		ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{enis[0]},
	}

	netNSName := fmt.Sprintf(netNSNamePattern, testTaskID, eniName)
	netNSPath := netNSPathDir + netNSName
	taskNetConfig := tasknetworkconfig.TaskNetworkConfig{
		NetworkMode: ecs.NetworkModeAwsvpc,
		NetworkNamespaces: []*tasknetworkconfig.NetworkNamespace{
			{
				Name:  netNSName,
				Path:  netNSPath,
				Index: 0,
				NetworkInterfaces: []*networkinterface.NetworkInterface{
					&netIfs[0],
				},
				KnownState:   status.NetworkNone,
				DesiredState: status.NetworkReadyPull,
			},
		},
	}

	return taskPayload, taskNetConfig
}

// getSingleNetNSMultiIfaceAWSVPCTestData returns test data for EKS like use cases.
func getSingleNetNSMultiIfaceAWSVPCTestData(testTaskID string) (*ecsacs.Task, tasknetworkconfig.TaskNetworkConfig) {
	taskPayload, taskNetConfig := getSingleNetNSAWSVPCTestData(testTaskID)
	enis, netIfs := getTestInterfacesData()
	secondIFPayload := enis[1]
	secondIF := &netIfs[1]
	taskPayload.ElasticNetworkInterfaces = append(taskPayload.ElasticNetworkInterfaces, secondIFPayload)
	netNS := taskNetConfig.NetworkNamespaces[0]
	netNS.NetworkInterfaces = append(netNS.NetworkInterfaces, secondIF)

	return taskPayload, taskNetConfig
}

// getMultiNetNSMultiIfaceAWSVPCTestData returns test data for multiple netns and net interface cases.
func getMultiNetNSMultiIfaceAWSVPCTestData(testTaskID string) (*ecsacs.Task, tasknetworkconfig.TaskNetworkConfig) {
	ifName1 := "primary-eni"
	ifName2 := "secondary-eni"
	enis, netIfs := getTestInterfacesData()
	enis[0].Name = aws.String(ifName1)
	enis[1].Name = aws.String(ifName2)

	netIfs[0].Name = ifName1
	netIfs[1].Name = ifName2
	netIfs[1].Default = true

	taskPayload := &ecsacs.Task{
		NetworkMode:              aws.String(ecs.NetworkModeAwsvpc),
		ElasticNetworkInterfaces: enis,
		Containers: []*ecsacs.Container{
			{
				NetworkInterfaceNames: []*string{aws.String(ifName1)},
			},
			{
				NetworkInterfaceNames: []*string{aws.String(ifName2)},
			},
			{
				NetworkInterfaceNames: []*string{aws.String(ifName1)},
			},
			{
				NetworkInterfaceNames: []*string{aws.String(ifName2)},
			},
		},
	}

	primaryNetNSName := fmt.Sprintf(netNSNamePattern, testTaskID, ifName1)
	primaryNetNSPath := netNSPathDir + primaryNetNSName
	secondaryNetNSName := fmt.Sprintf(netNSNamePattern, testTaskID, ifName2)
	secondaryNetNSPath := netNSPathDir + secondaryNetNSName

	taskNetConfig := tasknetworkconfig.TaskNetworkConfig{
		NetworkMode: ecs.NetworkModeAwsvpc,
		NetworkNamespaces: []*tasknetworkconfig.NetworkNamespace{
			{
				Name:  primaryNetNSName,
				Path:  primaryNetNSPath,
				Index: 0,
				NetworkInterfaces: []*networkinterface.NetworkInterface{
					&netIfs[0],
				},
				KnownState:   status.NetworkNone,
				DesiredState: status.NetworkReadyPull,
			},
			{
				Name:  secondaryNetNSName,
				Path:  secondaryNetNSPath,
				Index: 1,
				NetworkInterfaces: []*networkinterface.NetworkInterface{
					&netIfs[1],
				},
				KnownState:   status.NetworkNone,
				DesiredState: status.NetworkReadyPull,
			},
		},
	}

	return taskPayload, taskNetConfig
}

func getTestInterfacesData() ([]*ecsacs.ElasticNetworkInterface, []networkinterface.NetworkInterface) {
	// interfacePayloads have multiple interfaces as they are sent by ACS
	// that can be used as input data for tests.
	interfacePayloads := []*ecsacs.ElasticNetworkInterface{
		{
			Ec2Id:             aws.String(eniID),
			MacAddress:        aws.String(eniMAC),
			PrivateDnsName:    aws.String(dnsName),
			DomainNameServers: []*string{aws.String(nameServer)},
			Index:             aws.Int64(0),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(ipv4Addr),
				},
			},
			Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
				{
					Address: aws.String(ipv6Addr),
				},
			},
			SubnetGatewayIpv4Address:     aws.String(subnetGatewayCIDR),
			InterfaceAssociationProtocol: aws.String(networkinterface.DefaultInterfaceAssociationProtocol),
			DomainName:                   []*string{aws.String(searchDomainName)},
		},
		{
			Ec2Id:             aws.String(eniID2),
			MacAddress:        aws.String(eniMAC2),
			PrivateDnsName:    aws.String(dnsName),
			DomainNameServers: []*string{aws.String(nameServer2)},
			Index:             aws.Int64(1),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(ipv4Addr2),
				},
			},
			Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
				{
					Address: aws.String(ipv6Addr2),
				},
			},
			SubnetGatewayIpv4Address:     aws.String(subnetGatewayCIDR2),
			InterfaceAssociationProtocol: aws.String(networkinterface.DefaultInterfaceAssociationProtocol),
			DomainName:                   []*string{aws.String(searchDomainName)},
		},
	}

	networkInterfaces := []networkinterface.NetworkInterface{
		{
			ID:         eniID,
			MacAddress: eniMAC,
			Name:       eniName,
			IPV4Addresses: []*networkinterface.IPV4Address{
				{
					Primary: true,
					Address: ipv4Addr,
				},
			},
			IPV6Addresses: []*networkinterface.IPV6Address{
				{
					Address: ipv6Addr,
				},
			},
			SubnetGatewayIPV4Address:     subnetGatewayCIDR,
			DomainNameServers:            []string{nameServer},
			DomainNameSearchList:         []string{searchDomainName},
			PrivateDNSName:               dnsName,
			InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
			Index:                        int64(0),
			Default:                      true,
			KnownStatus:                  status.NetworkNone,
			DesiredStatus:                status.NetworkReadyPull,
		},
		{
			ID:         eniID2,
			MacAddress: eniMAC2,
			Name:       eniName2,
			IPV4Addresses: []*networkinterface.IPV4Address{
				{
					Primary: true,
					Address: ipv4Addr2,
				},
			},
			IPV6Addresses: []*networkinterface.IPV6Address{
				{
					Address: ipv6Addr2,
				},
			},
			SubnetGatewayIPV4Address:     subnetGatewayCIDR2,
			DomainNameServers:            []string{nameServer2},
			DomainNameSearchList:         []string{searchDomainName},
			PrivateDNSName:               dnsName,
			InterfaceAssociationProtocol: networkinterface.DefaultInterfaceAssociationProtocol,
			Index:                        int64(1),
			KnownStatus:                  status.NetworkNone,
			DesiredStatus:                status.NetworkReadyPull,
		},
	}

	return interfacePayloads, networkInterfaces
}

// testNetworkBuilder_StartAWSVPC verifies that the expected platform API calls
// are made by the network builder while configuring each network namespace.
// The test includes all known network configuration for a netns in AWSVPC mode.
func testNetworkBuilder_StartAWSVPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	platformAPI := mock_platform.NewMockAPI(ctrl)
	netBuilder := &networkBuilder{
		platformAPI: platformAPI,
	}

	// Single ENI use case without AppMesh and service connect configs.
	_, taskNetConfig := getSingleNetNSAWSVPCTestData(taskID)
	netNS := taskNetConfig.GetPrimaryNetNS()
	require.NotNil(t, netNS)

	netNS.KnownState = status.NetworkNone
	netNS.DesiredState = status.NetworkReadyPull
	t.Run("single-eni-default", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with AppMesh config and desired state = READY_PULL.
	// In this case, the appmesh configuration should not be executed.
	netNS.AppMeshConfig = &appmesh.AppMesh{
		// Placeholder data.
		ContainerName: "appmesh-envoy",
	}
	t.Run("single-eni-appmesh-readypull", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with AppMesh config and desired state = READY.
	// The appmesh configuration should get executed now.
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReady
	t.Run("single-eni-appmesh-ready", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with ServiceConnect and desired state = READY.
	// In this case, the ServiceConnect configuration should be executed.
	netNS.AppMeshConfig = nil
	netNS.ServiceConnectConfig = &serviceconnect.ServiceConnectConfig{
		ServiceConnectContainerName: "ecs-service-connect",
	}
	t.Run("single-eni-serviceconnect-ready", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single ENI with ServiceConnect and desired state = READY_PULL.
	// In this case, the ServiceConnect configuration should not be executed.
	netNS.KnownState = status.NetworkReadyPull
	netNS.DesiredState = status.NetworkReady
	t.Run("single-eni-serviceconnect-readypull", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Single netns with multi interface case.
	_, taskNetConfig = getSingleNetNSMultiIfaceAWSVPCTestData(taskID)
	netNS = taskNetConfig.GetPrimaryNetNS()
	t.Run("multi-eni-default", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})

	// Desired state = DELETED. There should be no expected calls to
	// platform APIs for this case.
	netNS.DesiredState = status.NetworkDeleted
	t.Run("deleted", func(*testing.T) {
		gomock.InOrder(
			getExpectedCalls_StartAWSVPC(ctx, platformAPI, netNS)...,
		)
		netBuilder.Start(ctx, ecs.NetworkModeAwsvpc, taskID, netNS)
	})
}

// getExpectedCalls_StartAWSVPC takes a netns configuration as input and
// generates a list of expected `gomock` calls to the mock platform API
// which the network builder will invoke during the test runs. The calls
// generated will specifically used to test the `Start` method of the
// network builder.
func getExpectedCalls_StartAWSVPC(
	ctx context.Context,
	platformAPI *mock_platform.MockAPI,
	netNS *tasknetworkconfig.NetworkNamespace,
) []*gomock.Call {
	// Start() should not be invoked when desired state = DELETED.
	if netNS.DesiredState == status.NetworkDeleted {
		return nil
	}

	var calls []*gomock.Call
	// Network namespace creation and DNS config files creation is to happen
	// only while transitioning from NONE to READY_PULL.
	if netNS.KnownState == status.NetworkNone &&
		netNS.DesiredState == status.NetworkReadyPull {
		calls = append(calls,
			platformAPI.EXPECT().CreateNetNS(netNS.Path).Return(nil).Times(1),
			platformAPI.EXPECT().CreateDNSConfig(taskID, netNS).Return(nil).Times(1))
	}

	// For each interface inside the netns, the network builder needs to invoke the
	// `ConfigureInterface` platformAPI.
	for _, iface := range netNS.NetworkInterfaces {
		calls = append(calls,
			platformAPI.EXPECT().ConfigureInterface(ctx, netNS.Path, iface).Return(nil).Times(1))
	}

	// AppMesh/ServiceConnect configurations are executed only during the READY_PULL -> READY transitions.
	if netNS.KnownState == status.NetworkReadyPull &&
		netNS.DesiredState == status.NetworkReady {
		// AppMesh and ServiceConnect configuration happens only if the configuration data is present.
		if netNS.AppMeshConfig != nil {
			calls = append(calls, platformAPI.EXPECT().ConfigureAppMesh(ctx, netNS.Path, netNS.AppMeshConfig).
				Return(nil).Times(1))
		}
		if netNS.ServiceConnectConfig != nil {
			calls = append(calls, platformAPI.EXPECT().ConfigureServiceConnect(ctx, netNS.Path,
				netNS.GetPrimaryInterface(), netNS.ServiceConnectConfig).Return(nil).Times(1))
		}
	}

	return calls
}
