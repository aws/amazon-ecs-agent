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
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"
)

type common struct {
	nsUtil    ecscni.NetNSUtil
	cniClient ecscni.CNI
}

func NewPlatform(
	platformString string,
	volumeAccessor volume.VolumeAccessor,
	stateDBDirectory string,
	netWrapper netwrapper.Net) (API, error) {
	return nil, nil
}

func (c *common) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nil, nil
}

func (c *common) CreateNetNS(netNSPath string) error {
	return nil
}

func (c *common) DeleteNetNS(netNSPath string) error {
	return nil
}

func (c *common) CreateDNSConfig(taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error {
	return nil
}

func (c *common) GetNetNSPath(netNSName string) string {
	return ""
}

func (c *common) ConfigureInterface(ctx context.Context, netNSPath string, iface *networkinterface.NetworkInterface) error {
	return nil
}

func (c *common) ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error {
	return nil
}

func (c *common) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return nil
}
