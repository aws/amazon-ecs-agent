// +build windows

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
	"time"

	"github.com/cihub/seelog"
	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/pkg/errors"
)

// setupNS is the called by SetupNS to setup the task namespace by invoking ADD for given CNI configurations
func (client *cniClient) setupNS(ctx context.Context, cfg *Config) (*current.Result, error) {
	seelog.Debugf("[ECSCNI] Setting up the container namespace %s", cfg.ContainerID)

	runtimeConfig := libcni.RuntimeConf{
		ContainerID: cfg.ContainerID,
		NetNS:       cfg.ContainerNetNS,
		// This field is not used in the windows plugin
		// However, we cannot keep it blank as it is expected by plugin due to its generic nature
		// Therefore, we will pass dummy value here
		IfName: TaskENIBridgeNetworkPrefix,
	}

	// Execute all CNI network configurations serially, in the given order.
	for _, networkConfig := range cfg.NetworkConfigs {
		cniNetworkConfig := networkConfig.CNINetworkConfig
		seelog.Debugf("[ECSCNI] Adding network %s type %s in the container namespace %s",
			cniNetworkConfig.Network.Name,
			cniNetworkConfig.Network.Type,
			cfg.ContainerID)
		_, err := client.libcni.AddNetwork(ctx, cniNetworkConfig, &runtimeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "add network failed")
		}

		seelog.Debugf("[ECSCNI] Completed adding network %s type %s in the container namespace %s",
			cniNetworkConfig.Network.Name,
			cniNetworkConfig.Network.Type,
			cfg.ContainerID)
	}

	seelog.Debugf("[ECSCNI] Completed setting up the container namespace: %s", cfg.ContainerID)

	return nil, nil
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
