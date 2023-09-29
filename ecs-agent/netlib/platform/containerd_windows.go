//go:build windows
// +build windows

package platform

type containerd struct {
	nsUtil ecscni.NetNSUtil
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
