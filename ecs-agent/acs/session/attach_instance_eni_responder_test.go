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

var testAttachInstanceENIMessage = &ecsacs.AttachInstanceNetworkInterfacesMessage{
	MessageId:            aws.String(testconst.MessageID),
	ClusterArn:           aws.String(testconst.ClusterName),
	ContainerInstanceArn: aws.String(testconst.ContainerInstanceARN),
	ElasticNetworkInterfaces: []*ecsacs.ElasticNetworkInterface{
		{
			Ec2Id:                        aws.String("1"),
			MacAddress:                   aws.String(testconst.RandomMAC),
			InterfaceAssociationProtocol: aws.String(testconst.InterfaceProtocol),
			SubnetGatewayIpv4Address:     aws.String(testconst.GatewayIPv4),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(testconst.IPv4Address),
				},
			},
		},
	},
	WaitTimeoutMs: aws.Int64(testconst.WaitTimeoutMillis),
}

// TestAttachInstanceENIEmptyMessage checks the validator against an
// empty AttachInstanceNetworkInterfacesMessage
func TestAttachInstanceENIEmptyMessage(t *testing.T) {
	err := validateAttachInstanceNetworkInterfacesMessage(nil)
	assert.EqualError(t, err, "Message is empty")
}

// TestAttachInstanceENIMessageWithNoMessageId checks the validator against an
// AttachInstanceNetworkInterfacesMessage without a messageId
func TestAttachInstanceENIMessageWithNoMessageId(t *testing.T) {
	tempMessageId := testAttachInstanceENIMessage.MessageId
	testAttachInstanceENIMessage.MessageId = nil

	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, "Message ID is not set")

	testAttachInstanceENIMessage.MessageId = tempMessageId
}

// TestAttachInstanceENIMessageWithNoClusterArn checks the validator against an
// AttachInstanceNetworkInterfacesMessage without a ClusterArn
func TestAttachInstanceENIMessageWithNoClusterArn(t *testing.T) {
	tempClusterArn := testAttachInstanceENIMessage.ClusterArn
	testAttachInstanceENIMessage.ClusterArn = nil

	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("clusterArn is not set for message ID %s",
		aws.StringValue(testAttachInstanceENIMessage.MessageId)))

	testAttachInstanceENIMessage.ClusterArn = tempClusterArn
}

// TestAttachInstanceENIMessageWithNoContainerInstanceArn checks the validator against an
// AttachInstanceNetworkInterfacesMessage without a ContainerInstanceArn
func TestAttachInstanceENIMessageWithNoContainerInstanceArn(t *testing.T) {
	tempContainerInstanceArn := testAttachInstanceENIMessage.ContainerInstanceArn
	testAttachInstanceENIMessage.ContainerInstanceArn = nil

	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("containerInstanceArn is not set for message ID %s",
		aws.StringValue(testAttachInstanceENIMessage.MessageId)))

	testAttachInstanceENIMessage.ContainerInstanceArn = tempContainerInstanceArn
}

// TestAttachInstanceENIMessageWithNoInterfaces checks the validator against an
// AttachInstanceNetworkInterfacesMessage without any interface
func TestAttachInstanceENIMessageWithNoInterfaces(t *testing.T) {
	tempENIs := testAttachInstanceENIMessage.ElasticNetworkInterfaces
	testAttachInstanceENIMessage.ElasticNetworkInterfaces = nil

	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("No ENIs for message ID %s",
		aws.StringValue(testAttachInstanceENIMessage.MessageId)))

	testAttachInstanceENIMessage.ElasticNetworkInterfaces = tempENIs
}

// TestAttachInstanceENIMessageWithMultipleInterfaceschecks checks the validator against an
// AttachInstanceNetworkInterfacesMessage with multiple interfaces
func TestAttachInstanceENIMessageWithMultipleInterfaces(t *testing.T) {
	testAttachInstanceENIMessage.ElasticNetworkInterfaces = append(
		testAttachInstanceENIMessage.ElasticNetworkInterfaces,
		&ecsacs.ElasticNetworkInterface{
			Ec2Id:                        aws.String("2"),
			MacAddress:                   aws.String(testconst.RandomMAC),
			InterfaceAssociationProtocol: aws.String(testconst.InterfaceProtocol),
			SubnetGatewayIpv4Address:     aws.String(testconst.GatewayIPv4),
			Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
				{
					Primary:        aws.Bool(true),
					PrivateAddress: aws.String(testconst.IPv4Address),
				},
			},
		})

	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.NoError(t, err)

	// Remove appended ENI.
	testAttachInstanceENIMessage.ElasticNetworkInterfaces =
		testAttachInstanceENIMessage.ElasticNetworkInterfaces[:len(testAttachInstanceENIMessage.ElasticNetworkInterfaces)-1]
}

// TestAttachInstanceENIMessageWithInvalidNetworkDetails checks the validator against an
// AttachInstanceNetworkInterfacesMessage with invalid network details
func TestAttachInstanceENIMessageWithInvalidNetworkDetails(t *testing.T) {
	tempIpv4Addresses := testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].Ipv4Addresses
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].Ipv4Addresses = nil
	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, "eni message validation: no ipv4 addresses in the message")
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].Ipv4Addresses = tempIpv4Addresses

	tempSubnetGatewayIpv4Address := testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address = nil
	err = validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, "eni message validation: no subnet gateway ipv4 address in the message")
	invalidSubnetGatewayIpv4Address := aws.String("0.0.0.INVALID")
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address = invalidSubnetGatewayIpv4Address
	err = validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("eni message validation: invalid subnet gateway ipv4 address %s",
		aws.StringValue(invalidSubnetGatewayIpv4Address)))
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].SubnetGatewayIpv4Address = tempSubnetGatewayIpv4Address

	tempMacAddress := testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].MacAddress
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].MacAddress = nil
	err = validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, "eni message validation: empty eni mac address in the message")
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].MacAddress = tempMacAddress

	tempEc2Id := testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].Ec2Id
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].Ec2Id = nil
	err = validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, "eni message validation: empty eni id in the message")
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].Ec2Id = tempEc2Id

	tempInterfaceAssociationProtocol :=
		testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol
	unsupportedInterfaceAssociationProtocol := aws.String("unsupported")
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol =
		unsupportedInterfaceAssociationProtocol
	err = validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("invalid interface association protocol: %s",
		aws.StringValue(unsupportedInterfaceAssociationProtocol)))
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol =
		aws.String(apieni.VLANInterfaceAssociationProtocol)
	err = validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, "vlan interface properties missing")
	testAttachInstanceENIMessage.ElasticNetworkInterfaces[0].InterfaceAssociationProtocol =
		tempInterfaceAssociationProtocol
}

// TestAttachInstanceENIMessageWithMissingTimeout checks the validator against an
// AttachInstanceNetworkInterfacesMessage without a wait timeout
func TestAttachInstanceENIMessageWithMissingTimeout(t *testing.T) {
	tempWaitTimeoutMs := testAttachInstanceENIMessage.WaitTimeoutMs
	testAttachInstanceENIMessage.WaitTimeoutMs = nil

	err := validateAttachInstanceNetworkInterfacesMessage(testAttachInstanceENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("Invalid timeout set for message ID %s",
		aws.StringValue(testAttachInstanceENIMessage.MessageId)))

	testAttachInstanceENIMessage.WaitTimeoutMs = tempWaitTimeoutMs
}
