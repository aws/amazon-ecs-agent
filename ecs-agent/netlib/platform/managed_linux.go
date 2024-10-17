package platform

import (
	"context"
	goErr "errors"
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
)

const (
	MacResource         = "mac"
	SubNetCidrBlock     = "network/interfaces/macs/%s/subnet-ipv4-cidr-block"
	PrivateIPv4Resource = "local-ipv4"
	InstanceIDResource  = "instance-id"
	DefaultArg          = "default"
)

type managedLinux struct {
	common
	client ec2.HttpClient
}

// BuildTaskNetworkConfiguration translates network data in task payload sent by ACS
// into the task network configuration data structure internal to the agent.
func (m *managedLinux) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	mode := aws.StringValue(taskPayload.NetworkMode)
	var netNSs []*tasknetworkconfig.NetworkNamespace
	var err error
	switch mode {
	case ecs.NetworkModeAwsvpc:
		netNSs, err = m.common.buildAWSVPCNetworkNamespaces(taskID, taskPayload, false, nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to translate network configuration")
		}
	case ecs.NetworkModeHost:
		netNSs, err = m.buildDefaultNetworkNamespace(taskID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create network namespace with host eni")
		}
	default:
		return nil, errors.New("invalid network mode: " + mode)
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
	privateIpv4, err1 := m.client.GetMetadata(PrivateIPv4Resource)
	macAddress, err2 := m.client.GetMetadata(MacResource)
	ec2ID, err3 := m.client.GetMetadata(InstanceIDResource)
	subNet, err4 := m.client.GetMetadata(fmt.Sprintf(SubNetCidrBlock, macAddress))
	macToNames, err5 := m.common.interfacesMACToName()
	if err := goErr.Join(err1, err2, err3, err4, err5); err != nil {
		logger.Error("Error fetching fields for default ENI", logger.Fields{
			loggerfield.Error: err,
		})
		return nil, err
	}

	hostENI := &ecsacs.ElasticNetworkInterface{
		AttachmentArn: aws.String("arn"),
		Ec2Id:         aws.String(ec2ID),
		Ipv4Addresses: []*ecsacs.IPv4AddressAssignment{
			{
				Primary:        aws.Bool(true),
				PrivateAddress: aws.String(privateIpv4),
			},
		},
		SubnetGatewayIpv4Address:     aws.String(subNet),
		MacAddress:                   aws.String(macAddress),
		DomainNameServers:            []*string{},
		DomainName:                   []*string{},
		PrivateDnsName:               aws.String(DefaultArg),
		InterfaceAssociationProtocol: aws.String(DefaultArg),
		Index:                        aws.Int64(64),
	}

	netNSName := networkinterface.NetNSName(taskID, DefaultArg)
	netInt, _ := networkinterface.New(hostENI, DefaultArg, nil, macToNames)
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
