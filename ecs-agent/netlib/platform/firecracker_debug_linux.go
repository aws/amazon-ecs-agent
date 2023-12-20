package platform

import "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

type firecrackerDebug struct {
	firecraker
}

func (fc *firecrackerDebug) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	return fc.common.createDNSConfig(taskID, true, netNS)
}
