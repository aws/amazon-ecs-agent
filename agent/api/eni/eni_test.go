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

package eni

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

const (
	defaultDNS         = "169.254.169.253"
	customDNS          = "10.0.0.2"
	customSearchDomain = "us-west-2.compute.internal"
)

// TestENIFromACS tests the eni information was correctly read from the acs
func TestENIFromACS(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		AttachmentArn: aws.String("arn"),
		Ec2Id:         aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String("ipv4"),
			},
		},
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String("ipv6")},
		},
		MacAddress:        aws.String("mac"),
		DomainNameServers: []*string{aws.String(defaultDNS), aws.String(customDNS)},
		DomainName:        []*string{aws.String(customSearchDomain)},
		PrivateDnsName:    aws.String("ip.region.compute.internal"),
	}
	eni, err := ENIFromACS(acsENI)
	assert.NoError(t, err)
	assert.NotNil(t, eni)
	assert.Equal(t, aws.StringValue(acsENI.Ec2Id), eni.ID)
	assert.Len(t, eni.IPV4Addresses, 1)
	assert.Len(t, eni.GetIPV4Addresses(), 1)
	assert.Equal(t, aws.StringValue(acsENI.Ipv4Addresses[0].PrivateAddress), eni.IPV4Addresses[0].Address)
	assert.Equal(t, aws.BoolValue(acsENI.Ipv4Addresses[0].Primary), eni.IPV4Addresses[0].Primary)
	assert.Equal(t, aws.StringValue(acsENI.MacAddress), eni.MacAddress)
	assert.Len(t, eni.IPV6Addresses, 1)
	assert.Len(t, eni.GetIPV6Addresses(), 1)
	assert.Equal(t, aws.StringValue(acsENI.Ipv6Addresses[0].Address), eni.IPV6Addresses[0].Address)
	assert.Len(t, eni.DomainNameServers, 2)
	assert.Equal(t, defaultDNS, eni.DomainNameServers[0])
	assert.Equal(t, customDNS, eni.DomainNameServers[1])
	assert.Len(t, eni.DomainNameSearchList, 1)
	assert.Equal(t, customSearchDomain, eni.DomainNameSearchList[0])
	assert.Equal(t, aws.StringValue(acsENI.PrivateDnsName), eni.PrivateDNSName)
}

// TestValidateENIFromACS tests the validation of enis from acs
func TestValidateENIFromACS(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		AttachmentArn: aws.String("arn"),
		Ec2Id:         aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String("ipv4"),
			},
		},
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String("ipv6")},
		},
		MacAddress: aws.String("mac"),
	}
	err := ValidateTaskENI(acsENI)
	assert.NoError(t, err)

	acsENI.Ipv6Addresses = nil
	err = ValidateTaskENI(acsENI)
	assert.NoError(t, err)

	acsENI.Ipv4Addresses = nil
	err = ValidateTaskENI(acsENI)
	assert.Error(t, err)
}

func TestInvalidENIInterfaceVlanPropertyMissing(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		InterfaceAssociationProtocol: aws.String(VLANInterfaceAssociationProtocol),
		AttachmentArn:                aws.String("arn"),
		Ec2Id:                        aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String("ipv4"),
			},
		},
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String("ipv6"),
			},
		},
		MacAddress: aws.String("mac"),
	}

	err := ValidateTaskENI(acsENI)
	assert.Error(t, err)

}

func TestInvalidENIInvalidInterfaceAssociationProtocol(t *testing.T) {
	acsENI := &ecsacs.ElasticNetworkInterface{
		InterfaceAssociationProtocol: aws.String("no-eni"),
		AttachmentArn:                aws.String("arn"),
		Ec2Id:                        aws.String("ec2id"),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String("ipv4"),
			},
		},
		Ipv6Addresses: []*ecsacs.IPv6AddressAssignment{
			{
				Address: aws.String("ipv6"),
			},
		},
		MacAddress: aws.String("mac"),
	}
	err := ValidateTaskENI(acsENI)
	assert.Error(t, err)
}
