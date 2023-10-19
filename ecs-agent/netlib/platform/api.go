package platform

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

// API declares a set of methods that requires platform specific implementations.
type API interface {
	// BuildTaskNetworkConfiguration translates network data in task payload sent by ACS
	// into the task network configuration data structure internal to the agent.
	BuildTaskNetworkConfiguration(
		taskID string,
		taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error)

	// CreateNetNS creates a network namespace with the specified name.
	CreateNetNS(netNSName string) error

	// DeleteNetNS deletes the specified network namespace.
	DeleteNetNS(netnsName string) error

	// CreateDNSConfig creates the following DNS config files depending on the
	// task network configuration:
	// 1. resolv.conf
	// 2. hosts
	// 3. hostname
	// These files are then copied into desired locations so that containers will
	// have access to the accurate DNS configuration information.
	CreateDNSConfig(taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error

	// GetNetNSPath returns the path of a network namespace.
	GetNetNSPath(netNSName string) string
}
