// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package ecs_cni

import "github.com/containernetworking/cni/pkg/types"

const (
	CNI_PATH            = "/ecs/cni"
	CNIVersion          = "0.3.0"
	VersionCommand      = "--version"
	defaultEthName      = "eth0"
	defaultBridgeName   = "ecs-bridge"
	netnsFormat         = "/proc/%s/ns/net"
	ECSSubnet           = "169.254.172.0/22"
	TaskIAMRoleEndpoint = "169.254.170.2/32"
	NetworkName         = "ecs-task-network"
)

type IpamConfig struct {
	Type        string         `json:"type,omitempty"`
	CNIVersion  string         `json:"cniVersion,omitempty"`
	IPV4Subnet  string         `json:"ipv4-subnet,omitempty"`
	IPV4Address string         `json:"ipv4-address,omitempty"`
	IPV4Gateway string         `json:"ipv4-gateway,omitempty"`
	IPV4Routes  []*types.Route `json:"ipv4-routes,omitempty"`
}

type BridgeConfig struct {
	Type         string     `json:"type,omitempty"`
	CNIVersion   string     `json:"cniVersion,omitempty"`
	BrName       string     `json:"bridge"`
	IsGW         bool       `json:"isGateway"`
	IsDefaultGW  bool       `json:"isDefaultGateway"`
	ForceAddress bool       `json:"forceAddress"`
	IPMasq       bool       `json:"ipMasq"`
	MTU          int        `json:"mtu"`
	HairpinMode  bool       `json:"hairpinMode"`
	IPAM         IpamConfig `json:"ipam,omitempty"`
}

type EniConfig struct {
	Type        string `json:"type,omitempty"`
	CNIVersion  string `json:"cniVersion,omitempty"`
	ENIID       string `json:"eni"`
	IPV4Address string `json:"ipv4-address"`
	IPV6Address string `json:"ipv6-address, omitempty"`
	MACAddress  string `json:"mac"`
}

type Config struct {
	PluginPath     string
	CniVersion     string
	ENIID          string
	ContainerID    string
	ContainerPID   string
	ENIIPV4Address string
	ENIIPV6Address string
	ENIMACAddress  string
	BridgeName     string
	IPAMV4Address  string
	VethName       string
}

type cniClient struct {
	PluginPath string
	CNIVersion string
	Subnet     string
}

type CNIClient interface {
	Version(string) (string, error)
	SetupNS(*Config) error
	CleanupNS(*Config) error
}
