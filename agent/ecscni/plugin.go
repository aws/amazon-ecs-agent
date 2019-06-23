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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/pkg/errors"
)

const (
	currentCNISpec = "0.3.1"
	// ECSCNIVersion, ECSCNIGitHash, VPCCNIGitHash needs to be updated every time CNI plugin is updated
	currentECSCNIVersion      = "2019.06.0"
	currentECSCNIGitHash      = "91ccefc8864ec14a32bd2b9d7e7de3060b685383"
	currentVPCCNIGitHash      = "cdd89b926ed29fe8ad3a229cafd65119a2833a3b"
	vpcCNIPluginPath          = "/log/vpc-branch-eni.log"
	vpcCNIPluginInterfaceType = "vlan"
)

// CNIClient defines the method of setting/cleaning up container namespace
type CNIClient interface {
	// Version returns the version of the plugin
	Version(string) (string, error)
	// Capabilities returns the capabilities supported by a plugin
	Capabilities(string) ([]string, error)
	// SetupNS sets up the namespace of container
	SetupNS(context.Context, *Config, time.Duration) (*current.Result, error)
	// CleanupNS cleans up the container namespace
	CleanupNS(context.Context, *Config, time.Duration) error
	// ReleaseIPResource marks the ip available in the ipam db
	ReleaseIPResource(context.Context, *Config, time.Duration) error
}

// cniClient is the client to call plugin and setup the network
type cniClient struct {
	pluginsPath string
	libcni      libcni.CNI
}

// NewClient creates a client of ecscni which is used to invoke the plugin
func NewClient(cfg *Config) CNIClient {
	libcniConfig := &libcni.CNIConfig{
		Path: []string{cfg.PluginsPath},
	}

	return &cniClient{
		pluginsPath: cfg.PluginsPath,
		libcni:      libcniConfig,
	}
}

func (client *cniClient) init() {
	// Set environment variables for CNI plugins.
	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	os.Setenv("VPC_CNI_LOG_LEVEL", logger.GetLevel())
	os.Setenv("VPC_CNI_LOG_FILE", vpcCNIPluginPath)
}

// SetupNS sets up the network namespace of a task by invoking the given CNI network configurations.
func (client *cniClient) SetupNS(
	ctx context.Context,
	cfg *Config,
	timeout time.Duration) (*current.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type output struct {
		result *current.Result
		err    error
	}
	return client.setupNS(ctx, cfg)
}

func (client *cniClient) setupNS(ctx context.Context, cfg *Config) (*current.Result, error) {
	seelog.Debugf("[ECSCNI] Setting up the container namespace %s", cfg.ContainerID)

	var result cnitypes.Result
	var err error

	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
	}

	// Execute all CNI network configurations serially, in the given order.
	for _, networkConfig := range cfg.NetworkConfigs {
		seelog.Debugf("[ECSCNI] Adding network %s type %s in the container namespace %s",
			networkConfig.Network.Name,
			networkConfig.Network.Type,
			cfg.ContainerID)

		result, err = client.libcni.AddNetwork(ctx, networkConfig, &runtimeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "add network failed")
		}

		seelog.Debugf("[ECSCNI] Completed adding network %s type %s in the container namespace %s",
			networkConfig.Network.Name,
			networkConfig.Network.Type,
			cfg.ContainerID)
	}

	seelog.Debugf("[ECSCNI] Completed setting up the container namespace: %s", result.String())

	if _, err = result.GetAsVersion(currentCNISpec); err != nil {
		seelog.Warnf("[ECSCNI] Unable to convert result to spec version %s; error: %v; result is of version: %s",
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
func (client *cniClient) CleanupNS(
	ctx context.Context,
	cfg *Config,
	timeout time.Duration) error {

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return client.cleanupNS(ctx, cfg)
}

func (client *cniClient) cleanupNS(ctx context.Context, cfg *Config) error {
	seelog.Debugf("[ECSCNI] Cleaning up the container namespace %s", cfg.ContainerID)

	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
	}

	// Execute all CNI network configurations serially, in the reverse order.
	for i := len(cfg.NetworkConfigs) - 1; i >= 0; i-- {
		networkConfig := cfg.NetworkConfigs[i]

		seelog.Debugf("[ECSCNI] Deleting network %s type %s in the container namespace %s",
			networkConfig.Network.Name,
			networkConfig.Network.Type,
			cfg.ContainerID)

		err := client.libcni.DelNetwork(ctx, networkConfig, &runtimeConfig)
		if err != nil {
			return errors.Wrap(err, "delete network failed")
		}

		seelog.Debugf("[ECSCNI] Completed deleting network %s type %s in the container namespace %s",
			networkConfig.Network.Name,
			networkConfig.Network.Type,
			cfg.ContainerID)
	}

	seelog.Debugf("[ECSCNI] Completed cleaning up the container namespace %s", cfg.ContainerID)

	return nil
}

// ReleaseIPResource marks the ip available in the ipam db
func (client *cniClient) ReleaseIPResource(ctx context.Context, cfg *Config, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(netnsFormat, cfg.ContainerPID),
	}

	seelog.Debugf("[ECSCNI] Releasing the ip resource from ipam db, id: [%s], ip: [%v]", cfg.ID, cfg.IPAMV4Address)
	os.Setenv("ECS_CNI_LOGLEVEL", logger.GetLevel())
	defer os.Unsetenv("ECS_CNI_LOGLEVEL")

	ifName, networkConfig, err := NewIPAMNetworkConfig(cfg)
	if err != nil {
		return err
	}

	runtimeConfig.IfName = ifName

	return client.libcni.DelNetwork(ctx, networkConfig, &runtimeConfig)
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
