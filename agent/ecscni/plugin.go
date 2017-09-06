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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

// CNIClient defines the method of setting/cleaning up container namespace
type CNIClient interface {
	Version(string) (string, error)
	Capabilities(string) ([]string, error)
	SetupNS(*Config) error
	CleanupNS(*Config) error
}

// cniClient is the client to call plugin and setup the network
type cniClient struct {
	pluginsPath string
	cniVersion  string
	subnet      string
	libcni      libcni.CNI
}

// NewClient creates a client of ecscni which is used to invoke the plugin
func NewClient(cfg *Config) CNIClient {
	libcniConfig := &libcni.CNIConfig{
		Path: []string{cfg.PluginsPath},
	}

	return &cniClient{
		pluginsPath: cfg.PluginsPath,
		cniVersion:  cfg.MinSupportedCNIVersion,
		subnet:      ecsSubnet,
		libcni:      libcniConfig,
	}
}

// SetupNS will set up the namespace of container, including create the bridge
// and the veth pair, move the eni to container namespace, setup the routes
func (client *cniClient) SetupNS(cfg *Config) error {
	cns := &libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
		IfName:      defaultEthName,
	}

	netConfigList, err := client.constructNetworkConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "cni invocation: Failed to construct network configuration for confirguring namespace")
	}

	seelog.Debugf("Starting setup the ENI (%s) in container namespace: %s", cfg.ENIID, cfg.ContainerID)
	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	defer os.Unsetenv("ECS_CNI_LOGLEVEL")
	result, err := client.libcni.AddNetworkList(netConfigList, cns)
	if err != nil {
		return err
	}

	seelog.Debugf("Set up container namespace done: %s", result.String())
	return nil
}

// CleanupNS will clean up the container namespace, including remove the veth
// pair, release the ip address in ipam
func (client *cniClient) CleanupNS(cfg *Config) error {
	cns := &libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
		IfName:      defaultEthName,
	}

	netConfigList, err := client.constructNetworkConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "cni invocation: Failed to construct network configuration to clean up namespace")
	}

	seelog.Debugf("Starting clean up the container namespace: %s", cfg.ContainerID)
	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	defer os.Unsetenv("ECS_CNI_LOGLEVEL")
	return client.libcni.DelNetworkList(netConfigList, cns)
}

// constructNetworkConfig creates configuration for eni, ipam and bridge plugin
func (client *cniClient) constructNetworkConfig(cfg *Config) (*libcni.NetworkConfigList, error) {
	_, dst, err := net.ParseCIDR(TaskIAMRoleEndpoint)
	if err != nil {
		return nil, err
	}

	ipamConf := IPAMConfig{
		Type:        ECSIPAMPluginName,
		CNIVersion:  client.cniVersion,
		IPV4Subnet:  client.subnet,
		IPV4Address: cfg.IPAMV4Address,
		ID:          cfg.ID,
		IPV4Routes: []*types.Route{
			{
				Dst: *dst,
			},
		},
	}

	bridgeName := defaultBridgeName
	if cfg.BridgeName != "" {
		bridgeName = cfg.BridgeName
	}
	bridgeConf := BridgeConfig{
		Type:       ECSBridgePluginName,
		CNIVersion: client.cniVersion,
		BridgeName: bridgeName,
		IPAM:       ipamConf,
	}

	eniConf := ENIConfig{
		Type:                 ECSENIPluginName,
		CNIVersion:           client.cniVersion,
		ENIID:                cfg.ENIID,
		IPV4Address:          cfg.ENIIPV4Address,
		MACAddress:           cfg.ENIMACAddress,
		IPV6Address:          cfg.ENIIPV6Address,
		BlockInstanceMetdata: cfg.BlockInstanceMetdata,
	}

	bridgeConfBytes, err := json.Marshal(bridgeConf)
	if err != nil {
		seelog.Errorf("Marshal bridge configuration failed, error: %v", err)
		return nil, err
	}
	plugins := []*libcni.NetworkConfig{
		&libcni.NetworkConfig{
			Network: &types.NetConf{
				Type: ECSBridgePluginName,
			},
			Bytes: bridgeConfBytes,
		},
	}

	eniConfBytes, err := json.Marshal(eniConf)
	if err != nil {
		seelog.Errorf("Marshal eni configuration error: %v", err)
		return nil, err
	}
	plugins = append(plugins, &libcni.NetworkConfig{
		Network: &types.NetConf{
			Type: ECSENIPluginName,
		},
		Bytes: eniConfBytes,
	})

	netconf := &libcni.NetworkConfigList{
		CNIVersion: client.cniVersion,
		Plugins:    plugins,
	}
	return netconf, nil
}

// Version returns the version of the plugin
func (client *cniClient) Version(name string) (string, error) {
	file := filepath.Join(client.pluginsPath, name)

	// Check if the plugin file exists before executing it
	_, err := os.Stat(file)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(file, versionCommand)
	versionInfo, err := cmd.Output()
	if err != nil {
		return "", err
	}

	version := &cniPluginVersion{}
	// versionInfo is of the format
	// {"version":"2017.06.0","dirty":true,"gitShortHash":"226db36"}
	// Unmarshal this
	err = json.Unmarshal(versionInfo, version)
	if err != nil {
		return "", errors.Wrapf(err, "ecscni: Unmarshal version from string: %s", versionInfo)
	}

	return version.str(), nil
}

// cniPluginVersion is used to convert the JSON output of the
// '--version' command into a string
type cniPluginVersion struct {
	Version string `json:"version"`
	Dirty   bool   `json:"dirty"`
	Hash    string `json:"gitShortHash"`
}

// str generates a string version of the CNI plugin version
// Example:
// {"version":"2017.06.0","dirty":true,"gitShortHash":"226db36"} => @226db36-2017.06.0
// {"version":"2017.06.0","dirty":false,"gitShortHash":"326db36"} => 326db36-2017.06.0
func (version *cniPluginVersion) str() string {
	ver := ""
	if version.Dirty {
		ver = "@"
	}
	return ver + version.Hash + "-" + version.Version
}

// Capabilities returns the capabilities supported by a plugin
func (client *cniClient) Capabilities(name string) ([]string, error) {
	file := filepath.Join(client.pluginsPath, name)

	// Check if the plugin file exists before executing it
	_, err := os.Stat(file)
	if err != nil {
		return nil, errors.Wrapf(err, "ecscni: unable to describe file info for '%s'", file)
	}

	cmd := exec.Command(file, capabilitiesCommand)
	capabilitiesInfo, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "ecscni: failed invoking capabilities command for '%s'", name)
	}

	capabilities := &struct {
		Capabilities []string `json:"capabilities"`
	}{}
	err = json.Unmarshal(capabilitiesInfo, capabilities)
	if err != nil {
		return nil, errors.Wrapf(err, "ecscni: failed to unmarshal capabilities for '%s' from string: %s", name, capabilitiesInfo)
	}

	return capabilities.Capabilities, nil
}
