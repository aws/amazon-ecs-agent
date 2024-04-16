package platform

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

// containerdDebug implements platform API methods for non-firecrakcer infrastructure.
type containerdDebug struct {
	containerd
}

func (c *containerdDebug) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	return c.common.createDNSConfig(taskID, true, netNS)
}
