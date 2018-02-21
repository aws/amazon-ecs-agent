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
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/pkg/errors"
)

const currentCNISpec = "0.3.1"

// CNIClient defines the method of setting/cleaning up container namespace
type CNIClient interface {
	// Version returns the version of the plugin
	Version(string) (string, error)
	// Capabilities returns the capabilities supported by a plugin
	Capabilities(string) ([]string, error)
	// SetupNS sets up the namespace of container
	SetupNS(*Config) (*current.Result, error)
	// CleanupNS cleans up the container namespace
	CleanupNS(*Config) error
	// ReleaseIPResource marks the ip available in the ipam db
	ReleaseIPResource(*Config) error
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
func (client *cniClient) SetupNS(cfg *Config) (*current.Result, error) {
	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
	}

	seelog.Debugf("Starting setup the ENI (%s) in container namespace: %s", cfg.ENIID, cfg.ContainerID)
	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	defer os.Unsetenv("ECS_CNI_LOGLEVEL")

	result, err := client.eniAdd(runtimeConfig, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "cni setup: invoke eni plugin failed")
	}
	seelog.Debugf("Ecscni: Set up eni done: %s", result.String())

	result, err = client.bridgeAdd(runtimeConfig, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "cni setup: invoke bridge plugin failed")
	}
	seelog.Debugf("Ecscni: Set up container namespace done: %s", result.String())
	if _, err = result.GetAsVersion(currentCNISpec); err != nil {
		seelog.Warnf("Ecscni: Unable to convert result to spec version %s; error: %v; result is of version: %s",
			currentCNISpec, err, result.Version())
		return nil, err
	}
	var curResult *current.Result
	curResult, ok := result.(*current.Result)
	if !ok {
		return nil, errors.Errorf(
			"cni setup: unable to convert result to expected version '%s'",
			result.String())
	}

	return curResult, nil
}

// CleanupNS will clean up the container namespace, including remove the veth
// pair and stop the dhclient
func (client *cniClient) CleanupNS(cfg *Config) error {
	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
	}

	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	defer os.Unsetenv("ECS_CNI_LOGLEVEL")
	seelog.Debugf("Ecscni: Starting clean up the container namespace: %s", cfg.ContainerID)
	// clean up the network namespace is separate from releasing the IP from IPAM
	err := client.bridgeDel(runtimeConfig, cfg)
	if err != nil {
		return errors.Wrap(err, "cni cleanup: invoke bridge plugin failed")
	}
	seelog.Debugf("Ecscni: bridge cleanup done: %s", cfg.ContainerID)

	err = client.eniDel(runtimeConfig, cfg)
	if err != nil {
		return errors.Wrap(err, "cni cleanup: invoke eni plugin failed")
	}
	seelog.Debugf("Ecscni: container namespace cleanup done: %s", cfg.ContainerID)
	return nil
}

// ReleaseIPResource marks the ip available in the ipam db
func (client *cniClient) ReleaseIPResource(cfg *Config) error {
	runtimeConfig := &libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
		IfName:      defaultVethName,
	}

	ipamConfig, err := client.createIPAMNetworkConfig(cfg)
	if err != nil {
		return err
	}

	seelog.Debugf("Releasing the ip resource from ipam db, id: [%s], ip: [%v]", cfg.ID, cfg.IPAMV4Address)
	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	defer os.Unsetenv("ECS_CNI_LOGLEVEL")
	return client.libcni.DelNetwork(ipamConfig, runtimeConfig)
}

// bridgeAdd invokes the bridge and ipam plugin to set up the bridge,
// also it sets the veth pair and route in container namespace
func (client *cniClient) bridgeAdd(runtimeConfig libcni.RuntimeConf, cfg *Config) (cnitypes.Result, error) {
	deviceName, networkConfig, err := client.createBridgeNetworkConfigWithIPAM(cfg)
	if err != nil {
		return nil, err
	}
	runtimeConfig.IfName = deviceName

	return client.libcni.AddNetwork(networkConfig, &runtimeConfig)
}

// eniAdd invokes the eni plugin to setup the eni in container namespace
func (client *cniClient) eniAdd(runtimeConfig libcni.RuntimeConf, cfg *Config) (cnitypes.Result, error) {
	deviceName, networkConfig, err := client.createENINetworkConfig(cfg)
	if err != nil {
		return nil, err
	}

	runtimeConfig.IfName = deviceName
	return client.libcni.AddNetwork(networkConfig, &runtimeConfig)
}

// bridgeDel invokes the bridge plugin to clean up the container namespace
func (client *cniClient) bridgeDel(runtimeConfig libcni.RuntimeConf, cfg *Config) error {
	deviceName, networkConfig, err := client.createBridgeNetworkConfigWithoutIPAM(cfg)
	if err != nil {
		return err
	}
	runtimeConfig.IfName = deviceName

	return client.libcni.DelNetwork(networkConfig, &runtimeConfig)
}

// eniDel invokes the eni plugin to clean up the eni in container namespace
func (client *cniClient) eniDel(runtimeConfig libcni.RuntimeConf, cfg *Config) error {
	deviceName, networkConfig, err := client.createENINetworkConfig(cfg)
	if err != nil {
		return err
	}

	runtimeConfig.IfName = deviceName
	return client.libcni.DelNetwork(networkConfig, &runtimeConfig)
}

// createBridgeNetworkConfigWithIPAM creates the config of bridge for ADD command, where
// bridge plugin acquires the IP and route information from IPAM
func (client *cniClient) createBridgeNetworkConfigWithIPAM(cfg *Config) (string, *libcni.NetworkConfig, error) {
	// Create the bridge config first
	bridgeConfig := client.createBridgeConfig(cfg)

	// Create the ipam config
	ipamConfig, err := client.createIPAMConfig(cfg)
	if err != nil {
		return "", nil, err
	}

	bridgeConfig.IPAM = ipamConfig

	networkConfig, err := client.constructNetworkConfig(bridgeConfig, ECSBridgePluginName)
	return defaultVethName, networkConfig, err
}

// createBridgeNetworkConfigWithoutIPAM creates the config of the bridge for removal
func (client *cniClient) createBridgeNetworkConfigWithoutIPAM(cfg *Config) (string, *libcni.NetworkConfig, error) {
	networkConfig, err := client.constructNetworkConfig(client.createBridgeConfig(cfg), ECSBridgePluginName)
	return defaultVethName, networkConfig, err
}

func (client *cniClient) createBridgeConfig(cfg *Config) BridgeConfig {
	bridgeName := defaultBridgeName
	if len(cfg.BridgeName) != 0 {
		bridgeName = cfg.BridgeName
	}

	bridgeConfig := BridgeConfig{
		Type:       ECSBridgePluginName,
		CNIVersion: client.cniVersion,
		BridgeName: bridgeName,
	}

	return bridgeConfig
}

// constructNetworkConfig takes in the config from agent and construct the configuration
// that's accepted by the libcni
func (client *cniClient) constructNetworkConfig(cfg interface{}, plugin string) (*libcni.NetworkConfig, error) {
	configBytes, err := json.Marshal(cfg)
	if err != nil {
		seelog.Errorf("Marshal configuration for plugin %s failed, error: %v", plugin, err)
		return nil, err
	}
	networkConfig := &libcni.NetworkConfig{
		Network: &cnitypes.NetConf{
			Type: plugin,
		},
		Bytes: configBytes,
	}

	return networkConfig, nil
}

func (client *cniClient) createENINetworkConfig(cfg *Config) (string, *libcni.NetworkConfig, error) {
	eniConf := ENIConfig{
		Type:                 ECSENIPluginName,
		CNIVersion:           client.cniVersion,
		ENIID:                cfg.ENIID,
		IPV4Address:          cfg.ENIIPV4Address,
		MACAddress:           cfg.ENIMACAddress,
		IPV6Address:          cfg.ENIIPV6Address,
		BlockInstanceMetdata: cfg.BlockInstanceMetdata,
	}
	networkConfig, err := client.constructNetworkConfig(eniConf, ECSENIPluginName)
	return defaultENIName, networkConfig, err
}

// createIPAMNetworkConfig constructs the ipam configuration accepted by libcni
func (client *cniClient) createIPAMNetworkConfig(cfg *Config) (*libcni.NetworkConfig, error) {
	ipamConfig, err := client.createIPAMConfig(cfg)
	if err != nil {
		return nil, err
	}

	ipamNetworkConfig := IPAMNetworkConfig{
		Name:       ECSIPAMPluginName,
		CNIVersion: client.cniVersion,
		IPAM:       ipamConfig,
	}
	return client.constructNetworkConfig(ipamNetworkConfig, ECSIPAMPluginName)
}

func (client *cniClient) createIPAMConfig(cfg *Config) (IPAMConfig, error) {
	_, dst, err := net.ParseCIDR(TaskIAMRoleEndpoint)
	if err != nil {
		return IPAMConfig{}, err
	}

	routes := []*cnitypes.Route{
		{
			Dst: *dst,
		},
	}
	for _, route := range cfg.AdditionalLocalRoutes {
		seelog.Debugf("Adding an additional route for %s", route)
		ipNetRoute := (net.IPNet)(route)
		routes = append(routes, &cnitypes.Route{Dst: ipNetRoute})
	}

	ipamConfig := IPAMConfig{
		Type:        ECSIPAMPluginName,
		CNIVersion:  client.cniVersion,
		IPV4Subnet:  client.subnet,
		IPV4Address: cfg.IPAMV4Address,
		ID:          cfg.ID,
		IPV4Routes:  routes,
	}

	return ipamConfig, nil
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
		return "", errors.Wrapf(err, "ecscni: unmarshal version from string: %s", versionInfo)
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
