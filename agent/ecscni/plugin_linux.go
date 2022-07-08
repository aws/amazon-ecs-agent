//go:build linux
// +build linux

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

package ecscni

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/pkg/errors"
)

const (
	vpcCNIPluginInterfaceType = "vlan"
	// vpcCNIPluginPath is the path of the cni plugin's log file.
	vpcCNIPluginPath = "/log/vpc-branch-eni.log"
)

// newCNIGuard returns a new instance of CNI guard for the CNI client.
// It is supported only on Windows.
func newCNIGuard() cniGuard {
	return &guard{
		mutex: nil,
	}
}

// setupNS is the called by SetupNS to setup the task namespace by invoking ADD for given CNI configurations
func (client *cniClient) setupNS(ctx context.Context, cfg *Config) (*current.Result, error) {
	seelog.Debugf("[ECSCNI] Setting up the container namespace %s", cfg.ContainerID)

	var bridgeResult cnitypes.Result
	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(NetnsFormat, cfg.ContainerPID),
	}

	// Execute all CNI network configurations serially, in the given order.
	for _, networkConfig := range cfg.NetworkConfigs {
		cniNetworkConfig := networkConfig.CNINetworkConfig
		seelog.Debugf("[ECSCNI] Adding network %s type %s in the container namespace %s",
			cniNetworkConfig.Network.Name,
			cniNetworkConfig.Network.Type,
			cfg.ContainerID)
		runtimeConfig.IfName = networkConfig.IfName
		result, err := client.libcni.AddNetwork(ctx, cniNetworkConfig, &runtimeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "add network failed")
		}
		// Save the result object from the bridge plugin execution. We need this later
		// for inferring what IPv4 address was used to bring up the veth pair for task.
		if cniNetworkConfig.Network.Type == ECSBridgePluginName {
			bridgeResult = result
		}
		seelog.Debugf("[ECSCNI] Completed adding network %s type %s in the container namespace %s",
			cniNetworkConfig.Network.Name,
			cniNetworkConfig.Network.Type,
			cfg.ContainerID)
	}

	seelog.Debugf("[ECSCNI] Completed setting up the container namespace: %s", cfg.ContainerID)

	if bridgeResult == nil {
		// Not every netns setup involves ECS Bridge Plugin
		return nil, nil
	}
	if _, err := bridgeResult.GetAsVersion(currentCNISpec); err != nil {
		seelog.Warnf("[ECSCNI] Unable to convert result to spec version %s; error: %v; result is of version: %s",
			currentCNISpec, err, bridgeResult.Version())
		return nil, err
	}
	var curResult *current.Result
	curResult, ok := bridgeResult.(*current.Result)
	if !ok {
		return nil, errors.Errorf(
			"cni setup: unable to convert result to expected version '%v'", bridgeResult)
	}

	return curResult, nil
}

// ReleaseIPResource marks the ip available in the ipam db
func (client *cniClient) ReleaseIPResource(ctx context.Context, cfg *Config, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       fmt.Sprintf(NetnsFormat, cfg.ContainerPID),
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
