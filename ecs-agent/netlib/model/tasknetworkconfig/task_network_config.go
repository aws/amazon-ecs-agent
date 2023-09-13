package tasknetworkconfig

import ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

type TaskNetworkConfig struct {
	NetworkNamespaces []*NetworkNamespace
	NetworkMode       string
}

func (tnc *TaskNetworkConfig) GetPrimaryInterface() *ni.NetworkInterface {
	return tnc.GetPrimaryNetNS().GetPrimaryInterface()
}

func (tnc *TaskNetworkConfig) GetPrimaryNetNS() *NetworkNamespace {
	for _, netns := range tnc.NetworkNamespaces {
		if netns.Index == 0 {
			return netns
		}
	}

	return nil
}
