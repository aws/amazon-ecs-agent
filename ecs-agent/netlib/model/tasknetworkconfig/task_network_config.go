package tasknetworkconfig

import ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

// TaskNetworkConfig is the top level network data structure associated with a task.
type TaskNetworkConfig struct {
	NetworkNamespaces []*NetworkNamespace
	NetworkMode       string
}

// GetPrimaryInterface returns the interface with index 0 inside the network namespace
// with index 0 associated with the task's network config.
func (tnc *TaskNetworkConfig) GetPrimaryInterface() *ni.NetworkInterface {
	if tnc != nil && tnc.GetPrimaryNetNS() != nil {
		return tnc.GetPrimaryNetNS().GetPrimaryInterface()
	}
	return nil
}

// GetPrimaryNetNS returns the netns with index 0 associated with the task's network config.
func (tnc *TaskNetworkConfig) GetPrimaryNetNS() *NetworkNamespace {
	for _, netns := range tnc.NetworkNamespaces {
		if netns.Index == 0 {
			return netns
		}
	}

	return nil
}
