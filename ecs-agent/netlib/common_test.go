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
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	"github.com/aws/aws-sdk-go/aws"
)

const (
	taskID             = "random-task-id"
	eniMAC             = "f0:5c:89:a3:ab:01"
	eniName            = "f05c89a3ab01"
	eniMAC2            = "f0:5c:89:a3:ab:02"
	eniName2           = "f05c89a3ab02"
	trunkMAC           = "f0:5c:89:a3:ab:03"
	vlanID             = "133"
	eniID              = "eni-abdf1234"
	eniID2             = "eni-abdf12342"
	dnsName            = "amazon.com"
	nameServer         = "10.1.0.2"
	nameServer2        = "10.2.0.2"
	ipv4Addr           = "10.1.0.196"
	ipv4Addr2          = "10.2.0.196"
	ipv6Addr           = "2600:1f13:4d9:e611:9009:ac97:1ab4:17d1"
	ipv6Addr2          = "2600:1f13:4d9:e611:9009:ac97:1ab4:17d2"
	subnetGatewayCIDR  = "10.1.0.1/24"
	subnetGatewayCIDR2 = "10.2.0.1/24"
	netNSNamePattern   = "%s-%s"
	searchDomainName   = "us-west-2.test.compute.internal"
	netNSPathDir       = "/var/run/netns/"
	tunnelID           = "1a2b3c"
	destinationIP      = "10.176.1.19"
	primaryIfaceName   = "primary"
	secondaryIfaceName = "secondary"
	vethIfaceName      = "veth"
)

// getSingleNetNSAWSVPCTestData returns a task payload and a task network config
// to be used the input and reference result for tests. The reference object will
// has only one network namespace and network interface.
func getSingleNetNSAWSVPCTestData(testTaskID string) (*ecsacs.Task, tasknetworkconfig.TaskNetworkConfig) {
	enis, netIfs := getTestInterfacesData_Containerd()
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
	enis, netIfs := getTestInterfacesData_Containerd()
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
	enis, netIfs := getTestInterfacesData_Containerd()
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

func getTestInterfacesData_Containerd() ([]*ecsacs.ElasticNetworkInterface, []networkinterface.NetworkInterface) {
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
			DeviceName:                   "eth1",
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
			DeviceName:                   "eth2",
		},
	}

	return interfacePayloads, networkInterfaces
}

// getV2NTestData returns a test task payload with a V2N interface to be used as test input and the
// task network config object as the expected output.
func getV2NTestData(testTaskID string) (*ecsacs.Task, tasknetworkconfig.TaskNetworkConfig) {
	enis, netIfs := getTestInterfacesData_Firecracker()
	taskPayload := &ecsacs.Task{
		NetworkMode:              aws.String(ecs.NetworkModeAwsvpc),
		ElasticNetworkInterfaces: enis,
		Containers: []*ecsacs.Container{
			{
				NetworkInterfaceNames: []*string{aws.String(primaryIfaceName)},
			},
			{
				NetworkInterfaceNames: []*string{aws.String(primaryIfaceName)},
			},
			{
				NetworkInterfaceNames: []*string{aws.String(secondaryIfaceName), aws.String(vethIfaceName)},
			},
			{
				NetworkInterfaceNames: []*string{aws.String(secondaryIfaceName), aws.String(vethIfaceName)},
			},
		},
	}

	netNSName := fmt.Sprintf(netNSNamePattern, testTaskID, primaryIfaceName)
	netNSPath := netNSPathDir + netNSName

	taskNetConfig := tasknetworkconfig.TaskNetworkConfig{
		NetworkMode: ecs.NetworkModeAwsvpc,
		NetworkNamespaces: []*tasknetworkconfig.NetworkNamespace{
			{
				Name:              netNSName,
				Path:              netNSPath,
				Index:             0,
				NetworkInterfaces: netIfs,
				KnownState:        status.NetworkNone,
				DesiredState:      status.NetworkReadyPull,
			},
		},
	}

	return taskPayload, taskNetConfig
}

func getTestInterfacesData_Firecracker() ([]*ecsacs.ElasticNetworkInterface, []*networkinterface.NetworkInterface) {
	// interfacePayloads have multiple interfaces as they are sent by ACS
	// that can be used as input data for tests.
	interfacePayloads := []*ecsacs.ElasticNetworkInterface{
		{
			Name:              aws.String(primaryIfaceName),
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
			InterfaceAssociationProtocol: aws.String(networkinterface.VLANInterfaceAssociationProtocol),
			DomainName:                   []*string{aws.String(searchDomainName)},
			InterfaceVlanProperties: &ecsacs.NetworkInterfaceVlanProperties{
				TrunkInterfaceMacAddress: aws.String(trunkMAC),
				VlanId:                   aws.String(vlanID),
			},
		},
		{
			Name:                         aws.String(secondaryIfaceName),
			PrivateDnsName:               aws.String(dnsName),
			DomainNameServers:            []*string{aws.String(nameServer2)},
			Index:                        aws.Int64(1),
			SubnetGatewayIpv4Address:     aws.String(subnetGatewayCIDR2),
			InterfaceAssociationProtocol: aws.String(networkinterface.V2NInterfaceAssociationProtocol),
			DomainName:                   []*string{aws.String(searchDomainName)},
			InterfaceTunnelProperties: &ecsacs.NetworkInterfaceTunnelProperties{
				TunnelId:           aws.String(tunnelID),
				InterfaceIpAddress: aws.String(destinationIP),
			},
		},
		{
			Name:                         aws.String(vethIfaceName),
			InterfaceAssociationProtocol: aws.String(networkinterface.VETHInterfaceAssociationProtocol),
			InterfaceVethProperties: &ecsacs.NetworkInterfaceVethProperties{
				PeerInterface: aws.String("primary"),
			},
		},
	}

	// networkInterfaces represents the desired structure of the network interfaces list
	// in the task network configuration object for the payload above.
	networkInterfaces := []*networkinterface.NetworkInterface{
		{
			ID:         eniID,
			MacAddress: eniMAC,
			Name:       primaryIfaceName,
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
			InterfaceAssociationProtocol: networkinterface.VLANInterfaceAssociationProtocol,
			Index:                        int64(0),
			Default:                      true,
			KnownStatus:                  status.NetworkNone,
			DesiredStatus:                status.NetworkReadyPull,
			InterfaceVlanProperties: &networkinterface.InterfaceVlanProperties{
				TrunkInterfaceMacAddress: trunkMAC,
				VlanID:                   vlanID,
			},
			DeviceName: "eth1.133",
		},
		{
			Name: secondaryIfaceName,
			IPV4Addresses: []*networkinterface.IPV4Address{
				{
					Address: networkinterface.DefaultGeneveInterfaceIPAddress,
				},
			},
			SubnetGatewayIPV4Address:     networkinterface.DefaultGeneveInterfaceGateway,
			DomainNameServers:            []string{nameServer2},
			DomainNameSearchList:         []string{searchDomainName},
			InterfaceAssociationProtocol: networkinterface.V2NInterfaceAssociationProtocol,
			Index:                        int64(1),
			KnownStatus:                  status.NetworkNone,
			DesiredStatus:                status.NetworkReadyPull,
			TunnelProperties: &networkinterface.TunnelProperties{
				ID:                   tunnelID,
				DestinationIPAddress: destinationIP,
			},
			GuestNetNSName: secondaryIfaceName,
		},
		{
			Name:                         vethIfaceName,
			InterfaceAssociationProtocol: networkinterface.VETHInterfaceAssociationProtocol,
			DomainNameServers:            []string{nameServer},
			DomainNameSearchList:         []string{searchDomainName},
			VETHProperties: &networkinterface.VETHProperties{
				PeerInterfaceName: primaryIfaceName,
			},
			GuestNetNSName: secondaryIfaceName,
			KnownStatus:    status.NetworkNone,
			DesiredStatus:  status.NetworkReadyPull,
		},
	}

	return interfacePayloads, networkInterfaces
}
