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

	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

// API declares a set of methods that requires platform specific implementations.
type API interface {
	// BuildTaskNetworkConfiguration translates network data in task payload sent by ACS
	// into the task network configuration data structure internal to the agent.
	BuildTaskNetworkConfiguration(
		taskID string,
		taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error)

	// HandleHostMode returns error if host mode is not enabled for the platform.
	HandleHostMode() error

	// CreateNetNS creates a network namespace with the specified path.
	CreateNetNS(netNSPath string) error

	// DeleteNetNS deletes the specified network namespace.
	DeleteNetNS(netNSPath string) error

	// CreateDNSConfig creates the following DNS config files depending on the
	// network namespace configuration:
	// 1. resolv.conf
	// 2. hosts
	// 3. hostname
	// These files are then copied into desired locations so that containers will
	// have access to the accurate DNS configuration information.
	CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error

	// DeleteDNSConfig deletes the directory at /etc/netns/<netns-name> and all its files.
	DeleteDNSConfig(netNSName string) error

	// GetNetNSPath returns the path of a network namespace.
	GetNetNSPath(netNSName string) string

	// ConfigureInterface configures an interface inside a network namespace
	// for it to be able to serve traffic.
	ConfigureInterface(
		ctx context.Context,
		netNSPath string,
		iface *networkinterface.NetworkInterface,
		netDAO netlibdata.NetworkDataClient,
	) error

	// ConfigureAppMesh configures AppMesh specific rules inside the task network namespace
	// to enable the AppMesh feature.
	ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error

	// ConfigureServiceConnect configures Service Connect specific rules inside the task network namespace
	// to enable the ServiceConnect feature.
	ConfigureServiceConnect(
		ctx context.Context,
		netNSPath string,
		primaryIf *networkinterface.NetworkInterface,
		scConfig *serviceconnect.ServiceConnectConfig,
	) error
}

// Config contains platform-specific data.
type Config struct {
	// Name specifies which platform to use (Linux, Windows, ec2-debug, etc).
	Name string
	// ResolvConfPath specifies path to resolv.conf file for DNS config.
	// Different platforms may have different paths for this file.
	ResolvConfPath string
}
