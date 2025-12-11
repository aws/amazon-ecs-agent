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

package tasknetworkconfig

import (
	"sort"
	"sync"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// NetworkNamespace is model representing each network namespace.
type NetworkNamespace struct {
	Name  string
	Path  string
	Index int

	// NetworkMode represents the network mode for this namespace.
	// Supported values: awsvpc (default), daemon-bridge (managed-instances only).
	NetworkMode types.NetworkMode

	// NetworkInterfaces represents ENIs or any kind of network interface associated the particular netns.
	NetworkInterfaces []*networkinterface.NetworkInterface

	// AppMeshConfig holds AppMesh related parameters for the particular netns.
	AppMeshConfig *appmesh.AppMesh

	// ServiceConnectConfig holds ServiceConnect related parameters for the particular netns.
	ServiceConnectConfig *serviceconnect.ServiceConnectConfig

	KnownState   status.NetworkStatus
	DesiredState status.NetworkStatus

	Mutex sync.Mutex `json:"-"`
}

func NewNetworkNamespace(
	netNSName string,
	netNSPath string,
	index int,
	proxyConfig *ecsacs.ProxyConfiguration,
	networkInterfaces ...*networkinterface.NetworkInterface) (*NetworkNamespace, error) {
	netNS := &NetworkNamespace{
		Name:              netNSName,
		Path:              netNSPath,
		Index:             index,
		NetworkInterfaces: networkInterfaces,
		KnownState:        status.NetworkNone,
		DesiredState:      status.NetworkReadyPull,
		NetworkMode:       types.NetworkModeAwsvpc,
	}

	// Sort interfaces as per their index values in ascending order.
	sort.Slice(netNS.NetworkInterfaces, func(i, j int) bool {
		return netNS.NetworkInterfaces[i].Index < netNS.NetworkInterfaces[j].Index
	})

	var err error
	if proxyConfig != nil {
		netNS.AppMeshConfig, err = appmesh.AppMeshFromACS(proxyConfig)
		if err != nil {
			return nil, err
		}
	}

	return netNS, nil
}

// GetPrimaryInterface returns the network interface that has the index value of 0 within
// the network namespace.
func (ns *NetworkNamespace) GetPrimaryInterface() *networkinterface.NetworkInterface {
	for _, ni := range ns.NetworkInterfaces {
		if ni.Default {
			return ni
		}
	}
	return nil
}

// IsPrimary returns true if the netns index is zero. This indicates that the primary interface of the task
// will be inside this netns. Image pulls, secret pulls, container logging, etc will happen over the
// primary netns.
func (ns *NetworkNamespace) IsPrimary() bool {
	return ns.Index == 0
}

// GetInterfaceByIndex returns the interface in the netns that has the specified index.
func (ns *NetworkNamespace) GetInterfaceByIndex(idx int64) *networkinterface.NetworkInterface {
	for _, iface := range ns.NetworkInterfaces {
		if iface.Index == idx {
			return iface
		}
	}

	return nil
}

// WithNetworkMode sets the NetworkMode field
func (ns *NetworkNamespace) WithNetworkMode(mode types.NetworkMode) *NetworkNamespace {
	ns.NetworkMode = mode
	return ns
}
