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

package ecscni

import (
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
)

const (
	// CNIPluginsPath is the default path where cni binaries are located
	CNIPluginsPath = "/ecs/cni"
	// CNIVersion is the default CNI spec version to use when invoke the plugins
	CNIVersion = "0.3.0"
	// VersionCommand is the command used to get the version of plugin
	VersionCommand = "--version"
	// defaultEthName is the name of veth pair name in the container namespace
	defaultEthName = "eth0"
	// defaultBridgeName is the default name of bridge created for container to
	// communicate with ecs-agent
	defaultBridgeName = "ecs-bridge"
	// netnsFormat is used to construct the path to cotainer network namespace
	netnsFormat = "/proc/%s/ns/net"
	// ECSSubnet is the available ip addresses to use for task networking
	ECSSubnet = "169.254.172.0/22"
	// TaskIAMRoleEndpoint is the endpoint of ecs-agent exposes credentials for
	// task IAM role
	TaskIAMRoleEndpoint = "169.254.170.2/32"
	// NetworkName is the name of the network set by the cni plugins
	NetworkName = "ecs-task-network"
)

type IPAMConfig struct {
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
	BridgeName   string     `json:"bridge"`
	IsGW         bool       `json:"isGateway"`
	IsDefaultGW  bool       `json:"isDefaultGateway"`
	ForceAddress bool       `json:"forceAddress"`
	IPMasq       bool       `json:"ipMasq"`
	MTU          int        `json:"mtu"`
	HairpinMode  bool       `json:"hairpinMode"`
	IPAM         IPAMConfig `json:"ipam,omitempty"`
}

type ENIConfig struct {
	Type        string `json:"type,omitempty"`
	CNIVersion  string `json:"cniVersion,omitempty"`
	ENIID       string `json:"eni"`
	IPV4Address string `json:"ipv4-address"`
	IPV6Address string `json:"ipv6-address, omitempty"`
	MACAddress  string `json:"mac"`
}

type Config struct {
	PluginPath             string
	MinSupportedCNIVersion string
	ENIID                  string
	ContainerID            string
	ContainerPID           string
	ENIIPV4Address         string
	ENIIPV6Address         string
	ENIMACAddress          string
	BridgeName             string
	IPAMV4Address          string
	VethName               string
}

type cniClient struct {
	pluginsPath string
	cniVersion  string
	subnet      string
	pluginPath  string
	cniVersion  string
	subnet      string
	libcni      libcni.CNI
}

type CNIClient interface {
	Version(string) (string, error)
	SetupNS(*Config) error
	CleanupNS(*Config) error
}
