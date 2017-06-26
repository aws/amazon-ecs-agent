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

	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

// cniClient is the client to call plugin and setup the network
type cniClient struct {
	pluginsPath string
	cniVersion  string
	subnet      string
	libcni      libcni.CNI
}

// CNIClient defines the method of setting/cleaning up container namespace
type CNIClient interface {
	Version(string) (string, error)
	SetupNS(*Config) error
	CleanupNS(*Config) error
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
	result, err := client.libcni.AddNetworkList(netConfigList, cns)
	if err != nil {
		return err
	}

	seelog.Debugf("Set up container namespace done: %v", result)
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
	return client.libcni.DelNetworkList(netConfigList, cns)
}

// constructNetworkConfig creates configuration for eni, ipam and bridge plugin
func (client *cniClient) constructNetworkConfig(cfg *Config) (*libcni.NetworkConfigList, error) {
	_, dst, err := net.ParseCIDR(TaskIAMRoleEndpoint)
	if err != nil {
		return nil, err
	}

	ipamConf := IPAMConfig{
		Type:        ipamPluginName,
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
		Type:       bridgePluginName,
		CNIVersion: client.cniVersion,
		BridgeName: bridgeName,
		IPAM:       ipamConf,
	}

	eniConf := ENIConfig{
		Type:        eniPluginName,
		CNIVersion:  client.cniVersion,
		ENIID:       cfg.ENIID,
		IPV4Address: cfg.ENIIPV4Address,
		MACAddress:  cfg.ENIMACAddress,
		IPV6Address: cfg.ENIIPV6Address,
	}

	bridgeConfBytes, err := json.Marshal(bridgeConf)
	if err != nil {
		seelog.Errorf("Marshal bridge configuration failed, error: %v", err)
		return nil, err
	}
	plugins := []*libcni.NetworkConfig{
		&libcni.NetworkConfig{
			Network: &types.NetConf{
				Type: bridgePluginName,
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
			Type: eniPluginName,
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

	// Check if the plugin execute file exists
	_, err := os.Stat(file)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(file, versionCommand)
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "output: %s", output)
	}

	version := &struct {
		Version string `json:"version"`
	}{}
	err = json.Unmarshal(output, version)
	if err != nil {
		return "", errors.Wrapf(err, "ecscni: Unmarshal version from string: %s", output)
	}

	return version.Version, nil
}
