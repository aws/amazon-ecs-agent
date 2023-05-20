//go:build unit
// +build unit

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

package session

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	apieni "github.com/aws/amazon-ecs-agent/ecs-agent/api/eni"
)

const (
	eniMessageId      = "123"
	randomMAC         = "00:0a:95:9d:68:16"
	waitTimeoutMillis = 1000

	interfaceProtocol = "default"
	gatewayIpv4       = "192.168.1.1/24"
	ipv4Address       = "ipv4"
)

var testAttachTaskENIMessage = &ecsacs.AttachTaskNetworkInterfacesMessage{
	MessageId:            aws.String(eniMessageId),
	ClusterArn:           aws.String(testconst.ClusterName),
	ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
	ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
		{
			Ec2Id:                        aws.String("1"),
			MacAddress:                   aws.String(randomMAC),
			InterfaceAssociationProtocol: aws.String(interfaceProtocol),
			SubnetGatewayIpv4Address:     aws.String(gatewayIpv4),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(ipv4Address),
				},
			},
		},
	},
	TaskArn:       aws.String(testconst.TaskARN),
	WaitTimeoutMs: aws.Int64(waitTimeoutMillis),
}

// TestAttachENIEmptyMessage checks the validator against an
// empty AttachTaskNetworkInterfacesMessage
func TestAttachENIEmptyMessage(t *testing.T) {
	err := validateAttachTaskNetworkInterfacesMessage(nil)
	assert.EqualError(t, err, "Message is empty")
}

// TestAttachENIMessageWithNoMessageId checks the validator against an
// AttachTaskNetworkInterfacesMessage without a messageId
func TestAttachENIMessageWithNoMessageId(t *testing.T) {
	tempMessageId := testAttachTaskENIMessage.MessageId
	testAttachTaskENIMessage.MessageId = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "Message ID is not set")

	testAttachTaskENIMessage.MessageId = tempMessageId
}

// TestAttachENIMessageWithNoClusterArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a ClusterArn
func TestAttachENIMessageWithNoClusterArn(t *testing.T) {
	tempClusterArn := testAttachTaskENIMessage.ClusterArn
	testAttachTaskENIMessage.ClusterArn = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("clusterArn is not set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.ClusterArn = tempClusterArn
}

// TestAttachENIMessageWithNoContainerInstanceArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a ContainerInstanceArn
func TestAttachENIMessageWithNoContainerInstanceArn(t *testing.T) {
	tempContainerInstanceArn := testAttachTaskENIMessage.ContainerInstanceArn
	testAttachTaskENIMessage.ContainerInstanceArn = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("containerInstanceArn is not set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.ContainerInstanceArn = tempContainerInstanceArn
}

// TestAttachENIMessageWithNoInterfaces checks the validator against an
// AttachTaskNetworkInterfacesMessage without any interface
func TestAttachENIMessageWithNoInterfaces(t *testing.T) {
	tempENIs := testAttachTaskENIMessage.ElasticNetworkInterfaces
	testAttachTaskENIMessage.ElasticNetworkInterfaces = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("Incorrect number of ENIs for message ID %s. Obtained %d",
		aws.StringValue(testAttachTaskENIMessage.MessageId), len(testAttachTaskENIMessage.ElasticNetworkInterfaces)))

	testAttachTaskENIMessage.ElasticNetworkInterfaces = tempENIs
}

// TestAttachENIMessageWithMultipleInterfaceschecks checks the validator against an
// AttachTaskNetworkInterfacesMessage with multiple interfaces
func TestAttachENIMessageWithMultipleInterfaces(t *testing.T) {
	testAttachTaskENIMessage.ElasticNetworkInterfaces = append(testAttachTaskENIMessage.ElasticNetworkInterfaces,
		&ecsacs.ElasticNetworkInterface{
			Ec2Id:                        aws.String("2"),
			MacAddress:                   aws.String(randomMAC),
			InterfaceAssociationProtocol: aws.String(interfaceProtocol),
			SubnetGatewayIpv4Address:     aws.String(gatewayIpv4),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(ipv4Address),
				},
			},
		})

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("Incorrect number of ENIs for message ID %s. Obtained %d",
		aws.StringValue(testAttachTaskENIMessage.MessageId), len(testAttachTaskENIMessage.ElasticNetworkInterfaces)))

	// Remove appended ENI.
	testAttachTaskENIMessage.ElasticNetworkInterfaces =
		testAttachTaskENIMessage.ElasticNetworkInterfaces[:len(testAttachTaskENIMessage.ElasticNetworkInterfaces)-1]
}

// TestAttachENIMessageWithInvalidNetworkDetails checks the validator against an
// AttachTaskNetworkInterfacesMessage with invalid network details
func TestAttachENIMessageWithInvalidNetworkDetails(t *testing.T) {
	tempIpv4Addresses := testAttachTaskENIMessage.ElasticNetworkInterfaces[0].Ipv4Addresses
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].Ipv4Addresses = nil
	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "eni message validation: no ipv4 addresses in the message")
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].Ipv4Addresses = tempIpv4Addresses

	tempSubnetGatewayIpv4Address := testAttachTaskENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address = nil
	err = validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "eni message validation: no subnet gateway ipv4 address in the message")
	invalidSubnetGatewayIpv4Address := aws.String("0.0.0.INVALID")
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address = invalidSubnetGatewayIpv4Address
	err = validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("eni message validation: invalid subnet gateway ipv4 address %s",
		aws.StringValue(invalidSubnetGatewayIpv4Address)))
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address = tempSubnetGatewayIpv4Address

	tempMacAddress := testAttachTaskENIMessage.ElasticNetworkInterfaces[0].MacAddress
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].MacAddress = nil
	err = validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "eni message validation: empty eni mac address in the message")
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].MacAddress = tempMacAddress

	tempEc2Id := testAttachTaskENIMessage.ElasticNetworkInterfaces[0].Ec2Id
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].Ec2Id = nil
	err = validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "eni message validation: empty eni id in the message")
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].Ec2Id = tempEc2Id

	tempInterfaceAssociationProtocol := testAttachTaskENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol
	unsupportedInterfaceAssociationProtocol := aws.String("unsupported")
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol = unsupportedInterfaceAssociationProtocol
	err = validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("invalid interface association protocol: %s",
		aws.StringValue(unsupportedInterfaceAssociationProtocol)))
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol =
		aws.String(apieni.VLANInterfaceAssociationProtocol)
	err = validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "vlan interface properties missing")
	testAttachTaskENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol = tempInterfaceAssociationProtocol
}

// TestAttachENIMessageWithMissingTaskArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a task ARN
func TestAttachENIMessageWithMissingTaskArn(t *testing.T) {
	tempTaskArn := testAttachTaskENIMessage.TaskArn
	testAttachTaskENIMessage.TaskArn = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("taskArn is not set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.TaskArn = tempTaskArn
}

// TestAttachENIMessageWithMissingTimeout checks the validator against an
// AttachTaskNetworkInterfacesMessage without a wait timeout
func TestAttachENIMessageWithMissingTimeout(t *testing.T) {
	tempWaitTimeoutMs := testAttachTaskENIMessage.WaitTimeoutMs
	testAttachTaskENIMessage.WaitTimeoutMs = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("Invalid timeout set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.WaitTimeoutMs = tempWaitTimeoutMs
}
