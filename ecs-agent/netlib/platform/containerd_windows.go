//go:build windows
// +build windows

package platform

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
)

type containerd struct {
	nsUtil ecscni.NetNSUtil
}

func NewPlatform(
	platformString string,
	nsUtil ecscni.NetNSUtil) (API, error) {
	return nil, nil
}

func (c *containerd) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nil, nil
}

func (c *containerd) CreateNetNS(netNSName string) error {
	return nil
}

func (c *containerd) DeleteNetNS(netnsName string) error {
	return nil
}

func (c *containerd) CreateDNSConfig(taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error {
	return nil
}

func (c *containerd) GetNetNSPath(netNSName string) string {
	return ""
}
