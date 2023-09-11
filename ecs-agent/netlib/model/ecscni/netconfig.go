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
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

const (
	DefaultInterfaceName = "eth0"
	DefaultENIName       = "eth1"

	PluginLogPath = "/var/log/ecs/ecs-cni-warmpool.log"
)

// PluginConfig is the general interface for a plugin's configuration
type PluginConfig interface {
	// String returns the human-readable information of the configuration
	String() string
	// InterfaceName returns the name of the interface to be configured
	InterfaceName() string
	// NSPath returns the path of the network namespace
	NSPath() string
	// PluginName returns the name of the plugin
	PluginName() string
	// CNIVersion returns the version of the cni spec
	CNIVersion() string
	// NetworkName returns the network name to be used by CNI plugin during network creation.
	// NetworkName is part of the network configuration required as per the CNI specifications.
	// https://github.com/containernetworking/cni/blob/master/SPEC.md
	NetworkName() string
	// ContainerID returns a plaintext identifier for a container. In our case we do not make use
	// of this field, although it is required to include a non-empty value for it since the
	// CNI framework enforces it.
	ContainerID() string
}

// CNIConfig defines the runtime configuration for invoking the plugin
type CNIConfig struct {
	NetNSPath      string `json:"-"`
	CNISpecVersion string `json:"cniVersion"`
	CNIPluginName  string `json:"type"`
}

func (cc *CNIConfig) String() string {
	return fmt.Sprintf("ns: %s, version: %s, plugin: %s",
		cc.NetNSPath, cc.CNISpecVersion, cc.CNIPluginName)
}

// ContainerID returns a plaintext identifier for a container. In our case we do not make use
// of this field, although it is required to include a non-empty value for it since the
// CNI framework enforces it. Hence we return a fixed string.
func (cc *CNIConfig) ContainerID() string {
	return "container-id"
}

// NetworkName returns a plaintext identifier which should be unique across all network
// configurations on a host (or other administrative domain). In our case we do not make use
// of this field, although it is required to include a non-empty value for it since the
// CNI framework enforces it. Hence we return a fixed string.
func (cc *CNIConfig) NetworkName() string {
	return "network-name"
}

// BuildNetworkConfig constructs the network configuration follow the format of libcni
func BuildNetworkConfig(cfg PluginConfig) (*libcni.NetworkConfig, error) {
	nc := &types.NetConf{
		Type:       cfg.PluginName(),
		CNIVersion: cfg.CNIVersion(),
		Name:       cfg.NetworkName(),
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal the plugin configuration")
	}

	netconfig := &libcni.NetworkConfig{
		Network: nc,
		Bytes:   data,
	}

	return netconfig, nil
}

// BuildRuntimeConfig constructs the runtime configuration following the format of libcni.
func BuildRuntimeConfig(cfg PluginConfig) *libcni.RuntimeConf {
	return &libcni.RuntimeConf{
		NetNS:       cfg.NSPath(),
		IfName:      cfg.InterfaceName(),
		ContainerID: cfg.ContainerID(),
	}
}
