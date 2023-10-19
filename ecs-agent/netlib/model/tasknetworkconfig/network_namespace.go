package tasknetworkconfig

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
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

	KnownState   status.NetworkStatus
	DesiredState status.NetworkStatus
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
	}

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
func (ns NetworkNamespace) GetPrimaryInterface() *networkinterface.NetworkInterface {
	for _, ni := range ns.NetworkInterfaces {
		if ni.Default {
			return ni
		}
	}
	return nil
}
