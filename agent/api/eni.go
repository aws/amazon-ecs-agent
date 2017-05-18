// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
	"github.com/aws/aws-sdk-go/aws"
)

type ENIAttachment struct {
	TaskArn          string              `json:"taskarn"`
	AttachmentArn    string              `json:"attachmentArn"`
	AttachStatusSent bool                `json:"attachSent"`
	MacAddress       string              `json:"macAddress"`
	Status           ENIAttachmentStatus `json:"status"`
	sentStatusLock   sync.RWMutex
}

type ENI struct {
	ID            string `json:"ec2Id"`
	IPV4Addresses []*ENIIPV4Address
	IPV6Addresses []*ENIIPV6Address
	MacAddress    string
}

type ENIIPV4Address struct {
	Primary bool
	Address string
}

type ENIIPV6Address struct {
	Address string
}

// ENIFromACS read the information from acs message and create the ENI object
func ENIFromACS(acsenis []*ecsacs.ElasticNetworkInterface) []*ENI {
	var enis []*ENI
	// Only one eni should be associated with the task
	// Only one ipv4 should be associated with the eni
	// ONly one ipv6 should be associated with the eni
	if len(acsenis) != 1 {
		return nil
	} else if len(acsenis[0].Ipv4Addresses) != 1 {
		return nil
	} else if len(acsenis[0].Ipv6Addresses) != 1 {
		return nil
	}

	for _, acseni := range acsenis {
		var ipv4 []*ENIIPV4Address
		var ipv6 []*ENIIPV6Address

		// Read ipv4 address information of the eni
		for _, ec2Ipv4 := range acseni.Ipv4Addresses {
			ipv4 = append(ipv4, &ENIIPV4Address{
				Primary: aws.BoolValue(ec2Ipv4.Primary),
				Address: aws.StringValue(ec2Ipv4.PrivateAddress),
			})
		}

		// Read ipv6 address information of the eni
		for _, ec2Ipv6 := range acseni.Ipv6Addresses {
			ipv6 = append(ipv6, &ENIIPV6Address{
				Address: aws.StringValue(ec2Ipv6.Address),
			})
		}

		enis = append(enis, &ENI{
			ID:            aws.StringValue(acseni.Ec2Id),
			IPV4Addresses: ipv4,
			IPV6Addresses: ipv6,
			MacAddress:    aws.StringValue(acseni.MacAddress),
		})
	}

	return enis
}

func (eni *ENIAttachment) GetStatusSent() bool {
	eni.sentStatusLock.RLock()
	defer eni.sentStatusLock.RLock()

	return eni.AttachStatusSent
}

func (eni *ENIAttachment) SetStatusSent() {
	eni.sentStatusLock.Lock()
	defer eni.sentStatusLock.RLock()

	eni.AttachStatusSent = true
}
