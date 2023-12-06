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

	"github.com/pkg/errors"
)

const (
	networkNamespaceBucketName = "networkNamespaceConfiguration"
	portBucketName             = "port"

	// geneveDstPortMin and geneveDstPortMax specifies the range from which the GENEVE destination port will be selected.
	// The range is currently set from 6081 to 6151 which gives us 70 ports per baremetal host to access the same subnet.
	// The goal while choosing these values were to keep the range wide enough so as to avoid port exhaustion and narrow
	// enough to reduce the attack surface since the same port range has to be opened up in the security group
	// assigned to the connectivity provider ENI. A rough calculation that explains why 70 ports were chosen as the range is:
	// Our largest customers have been seen to launch somewhat about 10,000 tasks around a given time. Ideally there
	// are 3 AZs per region with 2 cells. Each cell has 8 instances. We will also establish tunnels between the target
	// subnet and at least 3 random connectivity providers per cell. This means we'll be able to spread customer
	// tasks across at least 16 hosts and across 6 tunnels per AZ. Assuming even distribution, then this puts us
	// at 69 uVMS per host accessing the same subnet (10000 / [6 * 8 * 3]).
	// We start with 6081 since that is the default destination port for GENEVE interfaces.
	geneveDstPortMin = 6081
	geneveDstPortMax = 6151

	// geneveDstPortDefault is the default port to be assigned if not taken already.
	geneveDstPortDefault = 6081
)

var (
	trueValue       = []byte("1")
	errNoValue      = errors.New("all values in the given range are currently in use")
	errInvalidInput = errors.New("invalid random number requested")
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
