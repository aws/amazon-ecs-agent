//go:build !windows
// +build !windows

package platform

import (
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

const (
	// Identifiers for each platform we support.
	WarmpoolDebugPlatform    = "ec2-debug-warmpool"
	FirecrackerDebugPlatform = "ec2-debug-firecracker"
	WarmpoolPlatform         = "warmpool"
	FirecrackerPlatform      = "firecracker"

	// indexHighValue is a placeholder value used while finding
	// interface with lowest index in from the ACS payload.
	// It is assigned 100 because it is an unrealistically high
	// value for interface index.
	indexHighValue = 100
)

// common will be embedded within every implementation of the platform API.
// It contains all fields and methods that can be commonly used by all
// platforms.
type common struct {
	nsUtil ecscni.NetNSUtil
}

// NewPlatform creates an implementation of the platform API depending on the
// platform type where the agent is executing.
func NewPlatform(
	platformString string,
	nsUtil ecscni.NetNSUtil) (API, error) {
	commonPlatform := common{
		nsUtil: nsUtil,
	}

	// TODO: implement remaining platforms - FoF, ECS on EC2.
	switch platformString {
	case WarmpoolPlatform:
		return &containerd{
			common: commonPlatform,
		}, nil
	}
	return nil, errors.New("invalid platform: " + platformString)
}

// BuildTaskNetworkConfiguration translates network data in task payload sent by ACS
// into the task network configuration data structure internal to the agent.
func (c *common) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	mode := aws.StringValue(taskPayload.NetworkMode)
	var netNSs []*tasknetworkconfig.NetworkNamespace
	var err error
	switch mode {
	case ecs.NetworkModeAwsvpc:
		netNSs, err = c.buildAWSVPCNetworkNamespaces(taskID, taskPayload)
		if err != nil {
			return nil, errors.Wrap(err, "failed to translate network configuration")
		}
	case ecs.NetworkModeBridge:
		return nil, errors.New("not implemented")
	case ecs.NetworkModeHost:
		return nil, errors.New("not implemented")
	case ecs.NetworkModeNone:
		return nil, errors.New("not implemented")
	default:
		return nil, errors.New("invalid network mode: " + mode)
	}

	return &tasknetworkconfig.TaskNetworkConfig{
		NetworkNamespaces: netNSs,
		NetworkMode:       mode,
	}, nil
}

func (c *common) GetNetNSPath(netNSName string) string {
	return c.nsUtil.GetNetNSPath(netNSName)
}

// buildAWSVPCNetworkNamespaces returns list of NetworkNamespace which will be used to
// create the task's network configuration. All cases except those for FoF is covered by
// this method. FoF requires a separate specific implementation because the network setup
// is different due to the presence of the microVM.
// Use cases covered by this method are:
//  1. Single interface, network namespace (the only externally available config).
//  2. Single netns, multiple interfaces (For a non-managed multi-ENI experience. Eg EKS use case).
//  3. Multiple netns, multiple interfaces (future use case for internal customer who need
//     a managed multi-ENI experience).
func (c *common) buildAWSVPCNetworkNamespaces(taskID string,
	taskPayload *ecsacs.Task) ([]*tasknetworkconfig.NetworkNamespace, error) {
	if len(taskPayload.ElasticNetworkInterfaces) == 0 {
		return nil, errors.New("interfaces list cannot be empty")
	}
	// If task payload has only one interface, the network configuration is
	// straight forward. It will have only one network namespace containing
	// the corresponding network interface.
	// Empty Name fields in network interface names indicate that all
	// interfaces share the same network namespace. This use case is
	// utilized by certain internal teams like EKS on Fargate.
	if len(taskPayload.ElasticNetworkInterfaces) == 1 ||
		aws.StringValue(taskPayload.ElasticNetworkInterfaces[0].Name) == "" {
		primaryNetNS, err := c.buildSingleNSNetConfig(taskID,
			0,
			taskPayload.ElasticNetworkInterfaces,
			taskPayload.ProxyConfiguration)
		if err != nil {
			return nil, err
		}

		return []*tasknetworkconfig.NetworkNamespace{primaryNetNS}, nil
	}

	// Create a map for easier lookup of ENIs by their names.
	ifNameMap := make(map[string]*ecsacs.ElasticNetworkInterface, len(taskPayload.ElasticNetworkInterfaces))
	for _, iface := range taskPayload.ElasticNetworkInterfaces {
		ifNameMap[networkinterface.GetInterfaceName(iface)] = iface
	}

	// Proxy configuration is not supported yet in a multi-ENI / multi-NetNS task.
	if taskPayload.ProxyConfiguration != nil {
		return nil, errors.New("unexpected proxy config found")
	}

	// The number of network namespaces required to create depends on the
	// number of unique interface names list across all container definitions
	// in the task payload. Meaning if two containers are linked with the same
	// set of network interface names, both those containers share the same namespace.
	// If not, they reside in two different namespaces. Also, an interface can only
	// belong to one NetworkNamespace object.

	var netNSs []*tasknetworkconfig.NetworkNamespace
	nsIndex := 0
	// Loop through each container definition and their network interfaces.
	for _, container := range taskPayload.Containers {
		// ifaces holds all interfaces associated with a particular container.
		var ifaces []*ecsacs.ElasticNetworkInterface
		for _, ifNameP := range container.NetworkInterfaceNames {
			ifName := aws.StringValue(ifNameP)
			if iface := ifNameMap[ifName]; iface != nil {
				ifaces = append(ifaces, iface)
				// Remove ENI from map to indicate that the ENI is assigned to
				// a namespace.
				delete(ifNameMap, ifName)
			} else {
				// If the ENI does not exist in the lookup map, it means the ENI
				// is already assigned to a namespace. The container will be run
				// in the same namespace.
				break
			}
		}

		if len(ifaces) == 0 {
			continue
		}

		netNS, err := c.buildSingleNSNetConfig(taskID, nsIndex, ifaces, nil)
		if err != nil {
			return nil, err
		}
		netNSs = append(netNSs, netNS)
		nsIndex += 1
	}

	return netNSs, nil
}

func (c *common) buildSingleNSNetConfig(
	taskID string,
	index int,
	networkInterfaces []*ecsacs.ElasticNetworkInterface,
	proxyConfig *ecsacs.ProxyConfiguration) (*tasknetworkconfig.NetworkNamespace, error) {
	var primaryIF *networkinterface.NetworkInterface
	var ifaces []*networkinterface.NetworkInterface
	lowestIdx := int64(indexHighValue)
	for _, ni := range networkInterfaces {
		iface, err := networkinterface.New(ni, "", nil)
		if err != nil {
			return nil, err
		}
		if aws.Int64Value(ni.Index) < lowestIdx {
			primaryIF = iface
			lowestIdx = aws.Int64Value(ni.Index)
		}
		ifaces = append(ifaces, iface)
	}

	primaryIF.Default = true
	netNSName := networkinterface.NetNSName(taskID, primaryIF.Name)
	netNSPath := c.GetNetNSPath(netNSName)

	return tasknetworkconfig.NewNetworkNamespace(
		netNSName,
		netNSPath,
		index,
		proxyConfig,
		ifaces...)
}
