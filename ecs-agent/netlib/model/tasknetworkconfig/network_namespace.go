package tasknetworkconfig

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
)

// NetworkNamespace is model representing each network namespace.
type NetworkNamespace struct {
	Name  string
	Path  string
	Index int

	// NetworkInterfaces represents ENIs or any kind of network interface associated the particular netns.
	NetworkInterfaces []*networkinterface.NetworkInterface

	// AppMeshConfig holds AppMesh related parameters for the particular netns.
	AppMeshConfig *appmesh.AppMesh

	// TODO: Add Service Connect model here once it is moved under the netlib package.

	KnownState   string
	DesiredState string
}

// GetPrimaryInterface returns the network interface that has the index value of 0 within
// the network namespace.
func (ns NetworkNamespace) GetPrimaryInterface() *networkinterface.NetworkInterface {
	for _, ni := range ns.NetworkInterfaces {
		if ni.Index == 0 {
			return ni
		}
	}
	return nil
}
