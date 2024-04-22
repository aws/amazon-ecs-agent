package platform

import "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

type firecrackerDebug struct {
	firecraker
}

func (fc *firecrackerDebug) CreateDNSConfig(taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	err := fc.common.createDNSConfig(taskID, true, netNS)
	if err != nil {
		return err
	}

	return fc.configureSecondaryDNSConfig(taskID, netNS)
}
