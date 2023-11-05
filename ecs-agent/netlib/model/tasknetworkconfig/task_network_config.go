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
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/pkg/errors"
)

// TaskNetworkConfig is the top level network data structure associated with a task.
type TaskNetworkConfig struct {
	NetworkNamespaces []*NetworkNamespace
	NetworkMode       string
}

func New(networkMode string, netNSs ...*NetworkNamespace) (*TaskNetworkConfig, error) {
	if networkMode != ecs.NetworkModeAwsvpc &&
		networkMode != ecs.NetworkModeBridge &&
		networkMode != ecs.NetworkModeHost &&
		networkMode != ecs.NetworkModeNone {
		return nil, errors.New("invalid network mode: " + networkMode)
	}

	return &TaskNetworkConfig{
		NetworkNamespaces: netNSs,
		NetworkMode:       networkMode,
	}, nil
}

// GetPrimaryInterface returns the interface with index 0 inside the network namespace
// with index 0 associated with the task's network config.
func (tnc *TaskNetworkConfig) GetPrimaryInterface() *ni.NetworkInterface {
	if tnc != nil && tnc.GetPrimaryNetNS() != nil {
		return tnc.GetPrimaryNetNS().GetPrimaryInterface()
	}
	return nil
}

// GetPrimaryNetNS returns the netns with index 0 associated with the task's network config.
func (tnc *TaskNetworkConfig) GetPrimaryNetNS() *NetworkNamespace {
	for _, netns := range tnc.NetworkNamespaces {
		if netns.Index == 0 {
			return netns
		}
	}

	return nil
}
