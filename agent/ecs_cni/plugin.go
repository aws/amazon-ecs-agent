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

// NewClient creates a client of ecs_cni which is used to invoke the plugin
func NewClient(cfg *Config) CNIClient {
	pluginPath := CNI_PATH
	if cfg.PluginPath != "" {
		pluginPath = cfg.PluginPath
	}

	cniVersion := CNIVersion
	if cfg.CniVersion != "" {
		cniVersion = cfg.CniVersion
	}

	return &cniClient{
		PluginPath: pluginPath,
		CNIVersion: cniVersion,
		Subnet:     ECSSubnet,
	}
}

// SetupNS will set up the namespace of container, including create the bridge
// and the veth pair, move the eni to container namespace, setup the routes
func (client *cniClient) SetupNS(cfg *Config) error {
	vethName := defaultEthName
	if cfg.VethName != "" {
		vethName = cfg.VethName
	}

	cns := &libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
		IfName:      vethName,
	}

	netConfigList, err := client.constructNetworkConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "setupNS ecs_cni: Construct network configuration failed")
	}

	cniConfig := libcni.CNIConfig{
		Path: []string{client.PluginPath},
	}

	seelog.Debugf("Starting setup the container namespace: %s", cfg.ContainerID)
	result, err := cniConfig.AddNetworkList(netConfigList, cns)
	if err != nil {
		return err
	}

	seelog.Debugf("Set up container namespace done: %v", result)

	return nil
}

// CleanupNS will clean up the container namespace, including remove the veth
// pair, release the ip address in ipam
func (client *cniClient) CleanupNS(cfg *Config) error {
	vethName := defaultEthName
	if cfg.VethName != "" {
		vethName = cfg.VethName
	}

	cns := &libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
		IfName:      vethName,
	}

	netConfigList, err := client.constructNetworkConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "cleanupNS ecs_cni: Construct network configuration failed")
	}

	cniConfig := libcni.CNIConfig{
		Path: []string{client.PluginPath},
	}

	seelog.Debugf("Starting clean up the container namespace: %s", cfg.ContainerID)
	err = cniConfig.DelNetworkList(netConfigList, cns)
	if err != nil {
		return err
	}

	return nil
}

// constructNetworkConfig creates configuration for eni, ipam and bridge plugin
func (client *cniClient) constructNetworkConfig(cfg *Config) (*libcni.NetworkConfigList, error) {
	_, dst, _ := net.ParseCIDR(TaskIAMRoleEndpoint)
	ipamConf := IpamConfig{
		Type:        "ipam",
		CNIVersion:  client.CNIVersion,
		IPV4Subnet:  client.Subnet,
		IPV4Address: cfg.IPAMV4Address,
		IPV4Routes: []*types.Route{
			{
				Dst: *dst,
			},
		},
	}

	brName := defaultBridgeName
	if cfg.BridgeName != "" {
		brName = cfg.BridgeName
	}
	bridgeConf := BridgeConfig{
		Type:       "bridge",
		CNIVersion: client.CNIVersion,
		BrName:     brName,
		IsGW:       true,
		IPAM:       ipamConf,
	}

	eniConf := EniConfig{
		Type:        "eni",
		CNIVersion:  client.CNIVersion,
		ENIID:       cfg.ENIID,
		IPV4Address: cfg.ENIIPV4Address,
		MACAddress:  cfg.ENIMACAddress,
		IPV6Address: cfg.ENIMACAddress,
	}

	bridgeConfBytes, err := json.Marshal(bridgeConf)
	if err != nil {
		seelog.Errorf("Marshal bridge configuration failed, error: %v", err)
		return nil, err
	}
	plugins := []*libcni.NetworkConfig{
		&libcni.NetworkConfig{
			Network: &types.NetConf{
				Type: "bridge",
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
			Type: "eni",
		},
		Bytes: eniConfBytes,
	})

	netconf := &libcni.NetworkConfigList{
		Name:       NetworkName,
		CNIVersion: client.CNIVersion,
		Plugins:    plugins,
	}
	return netconf, nil
}

// Version returns the version of the plugin
func (client *cniClient) Version(name string) (string, error) {
	file := filepath.Join(client.PluginPath, name)

	// Check if the plugin execute file exists
	_, err := os.Stat(file)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(file, VersionCommand)
	versionInfo, err := cmd.Output()
	if err != nil {
		return "", err
	}

	version := &struct {
		Version string `json::"version"`
	}{}
	err = json.Unmarshal(versionInfo, version)
	if err != nil {
		return "", errors.Wrapf(err, "Version ecs_cni: Unmarshal version from string: %s", versionInfo)
	}

	return version.Version, nil
}
