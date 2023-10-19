package platform

import (
	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

// containerd implements platform API methods for non-firecrakcer infrastructure.
type containerd struct {
	common
}

func (c *containerd) CreateNetNS(netNSName string) error {
	return nil
}

func (c *containerd) ConfigureNamespaces(
	ctx context.Context,
	netNamespaces []*tasknetworkconfig.NetworkNamespace,
	networkMode string) error {
	return nil
}

func (c *containerd) DeleteNetNS(netnsName string) error {
	return nil
}

func (c *containerd) CreateDNSConfig(taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error {
	return nil
}
