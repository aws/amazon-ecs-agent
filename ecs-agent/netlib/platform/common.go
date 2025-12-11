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

package platform

import (
	"context"
	"errors"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	"github.com/containernetworking/cni/pkg/types"
)

const (
	// Identifiers for each platform we support.
	WarmpoolDebugPlatform    = "ec2-debug-warmpool"
	FirecrackerDebugPlatform = "ec2-debug-firecracker"
	WarmpoolPlatform         = "warmpool"
	FirecrackerPlatform      = "firecracker"
	ManagedPlatform          = "managed-instance"
	ManagedDebugPlatform     = "ec2-debug-managed-instance"
)

// executeCNIPlugin executes CNI plugins with the given network configs and a timeout context.
func (c *common) executeCNIPlugin(
	ctx context.Context,
	add bool,
	cniNetConf ...ecscni.PluginConfig,
) ([]*types.Result, error) {
	var timeout time.Duration
	var results []*types.Result
	var err error

	if add {
		timeout = nsSetupTimeoutDuration
	} else {
		timeout = nsCleanupTimeoutDuration
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, cfg := range cniNetConf {
		if add {
			var addResult types.Result
			addResult, err = c.cniClient.Add(ctx, cfg)
			if addResult != nil {
				results = append(results, &addResult)
			}
		} else {
			err = c.cniClient.Del(ctx, cfg)
		}

		if err != nil {
			break
		}
	}

	return results, err
}

// interfacesMACToName lists all network interfaces on the host inside the default
// netns and returns a mac address to device name map.
func (c *common) interfacesMACToName() (map[string]string, error) {
	links, err := c.net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Build a map of interface MAC address to name on the host.
	macToName := make(map[string]string)
	for _, link := range links {
		macToName[link.HardwareAddr.String()] = link.Name
	}

	return macToName, nil
}

// HandleHostMode by default we do not want to support host mode.
func (c *common) HandleHostMode() error {
	return errors.New("invalid platform for host mode")
}

// ConfigureDaemonNetNS configures a network namespace for workloads running as daemons.
// This is an internal networking mode available in EMI (ECS Managed Instances) only.
func (c *common) ConfigureDaemonNetNS(netNS *tasknetworkconfig.NetworkNamespace) error {
	return errors.New("daemon network namespaces are not supported in this platform")
}

// StopDaemonNetNS stops and cleans up a daemon network namespace.
// This is an internal networking mode available in EMI (ECS Managed Instances) only.
func (c *common) StopDaemonNetNS(ctx context.Context, netNS *tasknetworkconfig.NetworkNamespace) error {
	return errors.New("daemon network namespaces are not supported in this platform")
}
