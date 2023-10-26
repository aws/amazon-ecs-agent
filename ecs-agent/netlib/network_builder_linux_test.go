//go:build !windows && unit
// +build !windows,unit

package netlib

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"

	"github.com/aws/aws-sdk-go/aws"
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

// getTestFunc returns a test function that verifies the capability of the networkBuilder
// to translate a given input task payload into desired network data models.
func getTestFunc(dataGenF func(string) (input *ecsacs.Task, expected tasknetworkconfig.TaskNetworkConfig)) func(*testing.T) {

	return func(t *testing.T) {
		// Create a networkBuilder for the warmpool platform.
		netBuilder, err := NewNetworkBuilder(platform.WarmpoolPlatform)
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
