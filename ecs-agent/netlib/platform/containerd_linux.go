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

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

// containerd implements platform API methods for non-firecrakcer infrastructure.
type containerd struct {
	common
}

func (c *containerd) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {

	return c.common.buildTaskNetworkConfiguration(taskID, taskPayload, false, nil)
}

func (c *containerd) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	return c.common.createDNSConfig(taskID, false, netNS)
}

func (c *containerd) ConfigureInterface(
	ctx context.Context, netNSPath string, iface *networkinterface.NetworkInterface) error {
	return c.common.configureInterface(ctx, netNSPath, iface)
}

func (c *containerd) ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error {
	return c.common.configureAppMesh(ctx, netNSPath, cfg)
}

func (c *containerd) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return c.common.configureServiceConnect(ctx, netNSPath, primaryIf, scConfig)
}
