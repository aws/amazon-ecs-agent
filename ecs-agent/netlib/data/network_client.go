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

package data

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

type NetworkDataClient interface {
	GetNetworkNamespacesByTaskID(taskID string) ([]*tasknetworkconfig.NetworkNamespace, error)
	SaveNetworkNamespace(netNS *tasknetworkconfig.NetworkNamespace) error
	GetNetworkNamespace(netNSName string) (*tasknetworkconfig.NetworkNamespace, error)

	// AssignGeneveDstPort returns an unused destination port number for GENEVE interfaces.
	// By default for a particular VNI, it will return the default GENEVE destination port - 6081.
	// In case port 6081 is taken by another interface using the same VNI, it will chose a
	// random port from within the pre-configured range.
	AssignGeneveDstPort(vni string) (uint16, error)

	// ReleaseGeneveDstPort tells the client that the port is no longer in use by the interface having
	// the mentioned VNI. The port could be reused later.
	ReleaseGeneveDstPort(port uint16, vni string) error
}
