package tasknetworkconfig

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

	"github.com/pkg/errors"
)

// TaskNetworkConfig is the top level network data structure associated with a task.
type TaskNetworkConfig struct {
	NetworkNamespaces []*NetworkNamespace
	NetworkMode       string
}

func New(networkMode string, netNSs ...*NetworkNamespace) (*TaskNetworkConfig, error) {
	if networkMode != ecs.NetworkModeAwsvpc &&
		networkMode != ecs.NetworkModeBridge &&
		networkMode != ecs.NetworkModeHost &&
		networkMode != ecs.NetworkModeNone {
		return nil, errors.New("invalid network mode: " + networkMode)
	}

	return &TaskNetworkConfig{
		NetworkNamespaces: netNSs,
		NetworkMode:       networkMode,
	}, nil
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
