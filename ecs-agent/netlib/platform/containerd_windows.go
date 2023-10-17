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

//go:build windows
// +build windows

package platform

import (
	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

type containerd struct {
	nsUtil ecscni.NetNSUtil
}

func NewPlatform(
	platformString string,
	nsUtil ecscni.NetNSUtil) (API, error) {
	return nil, nil
}

func (c *containerd) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nil, nil
}

func (c *containerd) CreateNetNS(netNSPath string) error {
	return nil
}

func (c *containerd) DeleteNetNS(netNSName string) error {
	return nil
}

func (c *containerd) CreateDNSConfig(taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error {
	return nil
}

func (c *containerd) GetNetNSPath(netNSName string) string {
	return ""
}

func (c *containerd) ConfigureInterface(ctx context.Context, netNSPath string, iface *networkinterface.NetworkInterface) error {
	return nil
}

func (c *containerd) ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error {
	return nil
}

func (c *containerd) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return nil
}
