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
package state

import (
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/response"
	v2 "github.com/aws/amazon-ecs-agent/ecs-agent/tmds/handlers/v2"
)

const (
	ClockStatusSynchronized    = "SYNCHRONIZED"
	ClockStatusNotSynchronized = "NOT_SYNCHRONIZED"
)

// TaskResponse is the v4 Task response. It augments the v4 Container response
// with the v2 task response object.
type TaskResponse struct {
	*v2.TaskResponse
	Containers              []ContainerResponse     `json:"Containers,omitempty"`
	VPCID                   string                  `json:"VPCID,omitempty"`
	ServiceName             string                  `json:"ServiceName,omitempty"`
	ClockDrift              ClockDrift              `json:"ClockDrift,omitempty"`
	EphemeralStorageMetrics EphemeralStorageMetrics `json:"EphemeralStorageMetrics,omitempty"`
}

// Instance's clock drift status
type ClockDrift struct {
	ClockErrorBound            float64    `json:"ClockErrorBound,omitempty"`
	ReferenceTimestamp         *time.Time `json:"ReferenceTimestamp,omitempty"`
	ClockSynchronizationStatus string     `json:"ClockSynchronizationStatus,omitempty"`
}

// EphemeralStorageMetrics struct that is specific to the TMDS response. This struct will show customers the
// disk utilization and reservation metrics in MiBs to match the units used in other fields in TMDS.
type EphemeralStorageMetrics struct {
	UtilizedMiBs int64 `json:"Utilized"`
	ReservedMiBs int64 `json:"Reserved"`
}

// ContainerResponse is the v4 Container response. It augments the v4 Network response
// with the v2 container response object.
type ContainerResponse struct {
	*v2.ContainerResponse
	Networks []Network `json:"Networks,omitempty"`
}

// Network is the v4 Network response. It adds a bunch of information about network
// interface(s) on top of what is supported by v4.
// See `NetworkInterfaceProperties` for more details.
type Network struct {
	response.Network
	// NetworkInterfaceProperties specifies additional properties of the network
	// of the network interface that are exposed via the metadata server.
	// We currently populate this only for the `awsvpc` networking mode.
	NetworkInterfaceProperties
}

// NetworkInterfaceProperties represents additional properties we may want to expose via
// task metadata about the network interface that's attached to the container/task. We
// mostly use this to populate data about ENIs for tasks launched with `awsvpc` mode.
type NetworkInterfaceProperties struct {
	// AttachmentIndex reflects the `index` specified by the customer (if any)
	// while creating the task with `awsvpc` mode.
	AttachmentIndex *int `json:"AttachmentIndex,omitempty"`
	// MACAddress is the MAC address of the network interface.
	MACAddress string `json:"MACAddress,omitempty"`
	// IPv4SubnetCIDRBlock is the IPv4 CIDR address block associated with the interface's subnet.
	IPV4SubnetCIDRBlock string `json:"IPv4SubnetCIDRBlock,omitempty"`
	// IPv6SubnetCIDRBlock is the IPv6 CIDR address block associated with the interface's subnet.
	IPv6SubnetCIDRBlock string `json:"IPv6SubnetCIDRBlock,omitempty"`
	// DomainNameServers specifies the nameserver IP addresses for the network interface.
	DomainNameServers []string `json:"DomainNameServers,omitempty"`
	// DomainNameSearchList specifies the search list for the domain name lookup for
	// the network interface.
	DomainNameSearchList []string `json:"DomainNameSearchList,omitempty"`
	// PrivateDNSName is the dns name assigned to this network interface.
	PrivateDNSName string `json:"PrivateDNSName,omitempty"`
	// SubnetGatewayIPV4Address is the IPv4 gateway address for the network interface.
	SubnetGatewayIPV4Address string `json:"SubnetGatewayIpv4Address,omitempty"`
}
