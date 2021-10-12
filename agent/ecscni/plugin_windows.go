//go:build windows

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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"

	"github.com/aws/amazon-ecs-agent/agent/utils/retry"

	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/pkg/errors"
)

var (
	// vpcCNIPluginPath is the path of the cni plugin's log file.
	// The default value of log file is C:\ProgramData\Amazon\ECS\log\cni\vpc-eni.log
	vpcCNIPluginPath = filepath.Join(utils.DefaultIfBlank(os.Getenv("ProgramData"), `C:\ProgramData`),
		`Amazon\ECS\log\cni\vpc-eni.log`)
)

// newCNIGuard returns a new instance of CNI guard for the CNI client.
func newCNIGuard() cniGuard {
	return &guard{
		mutex: &sync.Mutex{},
	}
}

// setupNS is the called by SetupNS to setup the task namespace by invoking ADD for given CNI configurations.
// For Windows, we will retry the setup before conceding error.
func (client *cniClient) setupNS(ctx context.Context, cfg *Config) (*current.Result, error) {
	var result *current.Result
	var err error
	backoff := retry.NewExponentialBackoff(setupNSBackoffMin, setupNSBackoffMax,
		setupNSBackoffJitter, setupNSBackoffMultiple)

	for count := 0; count < setupNSMaxRetryCount; count++ {
		result, err = client.doSetupNS(ctx, cfg)
		if err == nil {
			return result, nil
		}
		if count < setupNSMaxRetryCount-1 {
			time.Sleep(backoff.Duration())
		}
		seelog.Errorf("[ECSCNI] Namespace setup failed due to error: %v. Retry count is %d.", err, count)
	}
	return nil, err
}

// doSetupNS invokes the CNI plugins to setup the task network namespace.
func (client *cniClient) doSetupNS(ctx context.Context, cfg *Config) (*current.Result, error) {
	seelog.Debugf("[ECSCNI] Setting up the container namespace %s", cfg.ContainerID)

	var ecsBridgeResult cnitypes.Result
	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       cfg.ContainerNetNS,
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

		// We save the result from ecs-bridge setup invocation of the plugin.
		if strings.EqualFold(ECSBridgeNetworkName, cniNetworkConfig.Network.Name) &&
			cniNetworkConfig.Network.Type == ECSVPCENIPluginExecutable {
			ecsBridgeResult = result
		}

		seelog.Debugf("[ECSCNI] Completed adding network %s type %s in the container namespace %s",
			cniNetworkConfig.Network.Name,
			cniNetworkConfig.Network.Type,
			cfg.ContainerID)
	}

	seelog.Debugf("[ECSCNI] Completed setting up the container namespace: %s", cfg.ContainerID)

	if _, err := ecsBridgeResult.GetAsVersion(currentCNISpec); err != nil {
		seelog.Warnf("[ECSCNI] Unable to convert result to spec version %s; error: %v; result is of version: %s",
			currentCNISpec, err, ecsBridgeResult.Version())
		return nil, err
	}
	var curResult *current.Result
	curResult, ok := ecsBridgeResult.(*current.Result)
	if !ok {
		return nil, errors.Errorf(
			"cni setup: unable to convert result to expected version '%s'",
			ecsBridgeResult.String())
	}

	return curResult, nil
}

// ReleaseIPResource marks the ip available in the ipam db
// This method is not required in Windows. HNS takes care of IP management.
func (client *cniClient) ReleaseIPResource(ctx context.Context, cfg *Config, timeout time.Duration) error {
	return nil
}

// Version is the version number of the repository.
// GitShortHash is the short hash of the Git HEAD.
// Built is the build time stamp.
type cniPluginVersion struct {
	Version      string `json:"version"`
	GitShortHash string `json:"gitShortHash"`
	Built        string `json:"built"`
}

// str generates a string version of the CNI plugin version
func (version *cniPluginVersion) str() string {
	return version.GitShortHash + "-" + version.Version
}
