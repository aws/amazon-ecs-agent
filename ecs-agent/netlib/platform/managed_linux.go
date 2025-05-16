package platform

import (
	"context"
	goErr "errors"
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/net"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/pkg/errors"
)

const (
	MacResource                = "mac"
	IPv4SubNetCidrBlock        = "network/interfaces/macs/%s/subnet-ipv4-cidr-block"
	IPv6SubNetCidrBlock        = "network/interfaces/macs/%s/subnet-ipv6-cidr-blocks"
	PrivateIPv4Address         = "local-ipv4"
	PrivateIPv6Address         = "ipv6"
	InstanceIDResource         = "instance-id"
	DefaultArg                 = "default"
	NetworkInterfaceDeviceName = "eth1" // default network interface name in the task network namespace.
)

type managedLinux struct {
	common
	client ec2.EC2MetadataClient
}

// BuildTaskNetworkConfiguration translates network data in task payload sent by ACS
// into the task network configuration data structure internal to the agent.
func (m *managedLinux) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	mode := types.NetworkMode(aws.ToString(taskPayload.NetworkMode))
	var netNSs []*tasknetworkconfig.NetworkNamespace
	var err error
	switch mode {
	case types.NetworkModeAwsvpc:
		netNSs, err = m.common.buildAWSVPCNetworkNamespaces(taskID, taskPayload, false, nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to translate network configuration")
		}
	case types.NetworkModeHost:
		netNSs, err = m.buildDefaultNetworkNamespace(taskID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create network namespace with host eni")
		}
	default:
		return nil, errors.New("invalid network mode: " + string(mode))
	}
	return &tasknetworkconfig.TaskNetworkConfig{
		NetworkNamespaces: netNSs,
		NetworkMode:       mode,
	}, nil
}

func (m *managedLinux) CreateDNSConfig(taskID string,
	netNS *tasknetworkconfig.NetworkNamespace) error {
	return m.common.createDNSConfig(taskID, true, netNS)
}

func (m *managedLinux) ConfigureInterface(
	ctx context.Context,
	netNSPath string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	// Set the network interface name on the task network namespace to eth1.
	iface.DeviceName = NetworkInterfaceDeviceName
	return m.common.configureInterface(ctx, netNSPath, iface, netDAO)
}

func (m *managedLinux) ConfigureAppMesh(ctx context.Context,
	netNSPath string,
	cfg *appmesh.AppMesh) error {
	return m.common.configureAppMesh(ctx, netNSPath, cfg)
}

func (m *managedLinux) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return m.common.configureServiceConnect(ctx, netNSPath, primaryIf, scConfig)
}

// buildDefaultNetworkNamespace return default network namespace of host ENI for host mode.
func (m *managedLinux) buildDefaultNetworkNamespace(taskID string) ([]*tasknetworkconfig.NetworkNamespace, error) {
	macAddress, err1 := m.client.GetMetadata(MacResource)
	ec2ID, err2 := m.client.GetMetadata(InstanceIDResource)
	macToNames, err3 := m.common.interfacesMACToName()
	if err := goErr.Join(err1, err2, err3); err != nil {
		logger.Error("Error fetching fields for default ENI", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, err
	}

	hostENI := &ecsacs.ElasticNetworkInterface{
		AttachmentArn:                aws.String("arn"),
		Ec2Id:                        aws.String(ec2ID),
		MacAddress:                   aws.String(macAddress),
		DomainNameServers:            []*string{},
		DomainName:                   []*string{},
		PrivateDnsName:               aws.String(DefaultArg),
		InterfaceAssociationProtocol: aws.String(DefaultArg),
		Index:                        aws.Int64(64),
	}

	ipComp, err := net.DetermineIPCompatibility(m.netlink, macAddress)
	if err != nil {
		logger.Error("Failed to determine IP compatibility of host ENI", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, err
	}

	if !ipComp.IsIPv4Compatible() && !ipComp.IsIPv6Compatible() {
		return nil, errors.New("Failed to build the default network namespace because the host ENI is neither " +
			"IPv4 enabled nor IPv6 enabled")
	}

	if ipComp.IsIPv6Compatible() {
		privateIpv6, err1 := m.client.GetMetadata(PrivateIPv6Address)
		ipv6SubNet, err2 := m.client.GetMetadata(fmt.Sprintf(IPv6SubNetCidrBlock, macAddress))
		if err := goErr.Join(err1, err2); err != nil {
			logger.Error("Error fetching IPv6 fields for default ENI", logger.Fields{
				loggerfield.Error: err,
			})
			return nil, err
		}

		hostENI.Ipv6Addresses = []*ecsacs.IPv6AddressAssignment{
			{
				Primary: aws.Bool(true),
				Address: aws.String(privateIpv6),
			},
		}
		hostENI.SubnetGatewayIpv6Address = aws.String(ipv6SubNet)
	}

	if ipComp.IsIPv4Compatible() {
		privateIpv4, err1 := m.client.GetMetadata(PrivateIPv4Address)
		ipv4SubNet, err2 := m.client.GetMetadata(fmt.Sprintf(IPv4SubNetCidrBlock, macAddress))
		if err := goErr.Join(err1, err2); err != nil {
			logger.Error("Error fetching IPv4 fields for default ENI", logger.Fields{
				loggerfield.Error: err,
			})
			return nil, err
		}

		hostENI.Ipv4Addresses = []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String(privateIpv4),
			},
		}
		hostENI.SubnetGatewayIpv4Address = aws.String(ipv4SubNet)
	}

	netNSName := networkinterface.NetNSName(taskID, DefaultArg)
	netInt, err := networkinterface.New(hostENI, DefaultArg, nil, macToNames)
	if err != nil {
		logger.Error("Failed to create the network interface", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, err
	}

	netInt.Default = true
	netInt.DesiredStatus = status.NetworkReady
	netInt.KnownStatus = status.NetworkReady
	defaultNameSpace, err := tasknetworkconfig.NewNetworkNamespace(netNSName, "", 0, nil, netInt)
	if err != nil {
		logger.Error("Error building default network namespace for host mode", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, err
	}
	defaultNameSpace.KnownState = status.NetworkReady
	defaultNameSpace.DesiredState = status.NetworkReady
	return []*tasknetworkconfig.NetworkNamespace{defaultNameSpace}, nil
}

// HandleHostMode is a no op because Host Mode does not require network interface configuration. No need to invoke CNI plugins.
func (m *managedLinux) HandleHostMode() error {
	return nil
}
