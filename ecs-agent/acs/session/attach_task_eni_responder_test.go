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

var testAttachTaskENIMessage = &ecsacs.AttachTaskNetworkInterfacesMessage{
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
	TaskArn:       aws.String(testconst.TaskARN),
	WaitTimeoutMs: aws.Int64(testconst.WaitTimeoutMillis),
}

// TestAttachTaskENIEmptyMessage checks the validator against an
// empty AttachTaskNetworkInterfacesMessage
func TestAttachTaskENIEmptyMessage(t *testing.T) {
	err := validateAttachTaskNetworkInterfacesMessage(nil)
	assert.EqualError(t, err, "Message is empty")
}

// TestAttachTaskENIMessageWithNoMessageId checks the validator against an
// AttachTaskNetworkInterfacesMessage without a messageId
func TestAttachTaskENIMessageWithNoMessageId(t *testing.T) {
	tempMessageId := testAttachTaskENIMessage.MessageId
	testAttachTaskENIMessage.MessageId = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, "Message ID is not set")

	testAttachTaskENIMessage.MessageId = tempMessageId
}

// TestAttachTaskENIMessageWithNoClusterArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a ClusterArn
func TestAttachTaskENIMessageWithNoClusterArn(t *testing.T) {
	tempClusterArn := testAttachTaskENIMessage.ClusterArn
	testAttachTaskENIMessage.ClusterArn = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("clusterArn is not set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.ClusterArn = tempClusterArn
}

// TestAttachTaskENIMessageWithNoContainerInstanceArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a ContainerInstanceArn
func TestAttachTaskENIMessageWithNoContainerInstanceArn(t *testing.T) {
	tempContainerInstanceArn := testAttachTaskENIMessage.ContainerInstanceArn
	testAttachTaskENIMessage.ContainerInstanceArn = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("containerInstanceArn is not set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.ContainerInstanceArn = tempContainerInstanceArn
}

// TestAttachTaskENIMessageWithNoInterfaces checks the validator against an
// AttachTaskNetworkInterfacesMessage without any interface
func TestAttachTaskENIMessageWithNoInterfaces(t *testing.T) {
	tempENIs := testAttachTaskENIMessage.ElasticNetworkInterfaces
	testAttachTaskENIMessage.ElasticNetworkInterfaces = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("No ENIs for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.ElasticNetworkInterfaces = tempENIs
}

// TestAttachTaskENIMessageWithMultipleInterfaceschecks checks the validator against an
// AttachTaskNetworkInterfacesMessage with multiple interfaces
func TestAttachTaskENIMessageWithMultipleInterfaces(t *testing.T) {
	testAttachTaskENIMessage.ElasticNetworkInterfaces = append(testAttachTaskENIMessage.ElasticNetworkInterfaces,
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

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.NoError(t, err)

	// Remove appended ENI.
	testAttachTaskENIMessage.ElasticNetworkInterfaces =
		testAttachTaskENIMessage.ElasticNetworkInterfaces[:len(testAttachTaskENIMessage.ElasticNetworkInterfaces)-1]
}

// TestAttachTaskENIMessageWithInvalidNetworkDetails checks the validator against an
// AttachTaskNetworkInterfacesMessage with invalid network details
func TestAttachTaskENIMessageWithInvalidNetworkDetails(t *testing.T) {
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

// TestAttachTaskENIMessageWithMissingTaskArn checks the validator against an
// AttachTaskNetworkInterfacesMessage without a task ARN
func TestAttachTaskENIMessageWithMissingTaskArn(t *testing.T) {
	tempTaskArn := testAttachTaskENIMessage.TaskArn
	testAttachTaskENIMessage.TaskArn = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("taskArn is not set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.TaskArn = tempTaskArn
}

// TestAttachTaskENIMessageWithMissingTimeout checks the validator against an
// AttachTaskNetworkInterfacesMessage without a wait timeout
func TestAttachTaskENIMessageWithMissingTimeout(t *testing.T) {
	tempWaitTimeoutMs := testAttachTaskENIMessage.WaitTimeoutMs
	testAttachTaskENIMessage.WaitTimeoutMs = nil

	err := validateAttachTaskNetworkInterfacesMessage(testAttachTaskENIMessage)
	assert.EqualError(t, err, fmt.Sprintf("Invalid timeout set for message ID %s",
		aws.StringValue(testAttachTaskENIMessage.MessageId)))

	testAttachTaskENIMessage.WaitTimeoutMs = tempWaitTimeoutMs
}
