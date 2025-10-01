package platform

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

// containerdDebug implements platform API methods for non-firecrakcer infrastructure.
type containerdDebug struct {
	containerd
}

func (c *containerdDebug) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	// SC tasks will get DNS config from control plane
	if netNS.ServiceConnectConfig != nil {
		return c.createDNSConfig(taskID, false, netNS)
	}
	return c.createDNSConfig(taskID, true, netNS)
}
