package tasknetworkconfig

import ni "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"

const (
	primaryNetNSName       = "primary-netns"
	primaryNetNSPath       = "primary-path"
	secondaryNetNSName     = "secondary-netns"
	primaryInterfaceName   = "primary-interface"
	secondaryInterfaceName = "secondary-interface"
)

func getTestTaskNetworkConfig() *TaskNetworkConfig {
	return &TaskNetworkConfig{
		NetworkNamespaces: getTestNetworkNamespaces(),
	}
}

func getTestNetworkNamespaces() []*NetworkNamespace {
	return []*NetworkNamespace{
		{
			Name:              secondaryNetNSName,
			Index:             1,
			NetworkInterfaces: getTestNetworkInterfaces(),
		},
		{
			Name:              primaryNetNSName,
			Index:             0,
			NetworkInterfaces: getTestNetworkInterfaces(),
		},
	}
}

func getTestNetworkInterfaces() []*ni.NetworkInterface {
	return []*ni.NetworkInterface{
		{
			Name:    secondaryInterfaceName,
			Default: false,
			Index:   1,
		},
		{
			Name:    primaryInterfaceName,
			Default: true,
			Index:   0,
		},
	}
}
