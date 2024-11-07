// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

//go:build windows
// +build windows

package platform

import (
	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/appmesh"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/serviceconnect"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/pkg/errors"
)

type common struct {
	nsUtil    ecscni.NetNSUtil
	cniClient ecscni.CNI
	os        oswrapper.OS
	net       netwrapper.Net
}

type containerd struct {
	common
}

type containerdDebug struct {
	containerd
}

// NewPlatform returns a platform instance with windows specific implementations of the API interface.
func NewPlatform(
	config Config,
	_ volume.TaskVolumeAccessor,
	_ string,
	net netwrapper.Net) (API, error) {
	c := containerd{
		common: common{
			nsUtil:    ecscni.NewNetNSUtil(),
			cniClient: ecscni.NewCNIClient([]string{GetCNIPluginPath()}),
			os:        oswrapper.NewOS(),
			net:       net,
		},
	}
	switch config.Name {
	case WarmpoolPlatform:
		return &c, nil
	case WarmpoolDebugPlatform:
		return &containerdDebug{
			containerd: c,
		}, nil
	default:
		return nil, errors.New("invalid platform string: " + config.Name)
	}
	return nil, nil
}

// BuildTaskNetworkConfiguration builds a task network configuration object from the task payload.
func (c *containerd) BuildTaskNetworkConfiguration(
	taskID string,
	taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	mode := aws.StringValue(taskPayload.NetworkMode)
	switch mode {
	case ecs.NetworkModeAwsvpc:
		return c.buildAWSVPCNetworkConfig(taskID, taskPayload)
	default:
		return nil, errors.New("invalid network mode")
	}
	return nil, nil
}

// buildAWSVPCNetworkConfig builds task network config object for AWSVPC.
func (c *containerd) buildAWSVPCNetworkConfig(
	taskID string,
	taskPayload *ecsacs.Task,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	if len(taskPayload.ElasticNetworkInterfaces) == 0 {
		return nil, errors.New("interfaces list cannot be empty")
	}

	// Find primary network interface in order to build the task netns name.
	var primaryIF *ecsacs.ElasticNetworkInterface
	for _, eni := range taskPayload.ElasticNetworkInterfaces {
		if aws.Int64Value(eni.Index) == 0 {
			primaryIF = eni
		}
	}
	ifName := networkinterface.GetInterfaceName(primaryIF)
	netNSName := networkinterface.NetNSName(taskID, ifName)
	netNSPath := c.GetNetNSPath(netNSName)

	// Make a map of mac addresses to network interfaces to make lookup easier.
	macToNames, err := c.interfacesMACToName()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get read network devices")
	}

	logger.Debug("Creating task network configuration", logger.Fields{
		"TaskID":     taskID,
		"NetNSName":  netNSName,
		"NetNSPath":  netNSPath,
		"MacToNames": macToNames,
	})

	// Create interface object.
	iface, err := networkinterface.New(
		taskPayload.ElasticNetworkInterfaces[0],
		"",
		taskPayload.ElasticNetworkInterfaces,
		macToNames,
	)
	iface.Default = true
	if err != nil {
		return nil, errors.Wrap(err, "failed to create network interface model")
	}

	netNS := &tasknetworkconfig.NetworkNamespace{
		Name:  netNSName,
		Path:  netNSPath,
		Index: 0,
		NetworkInterfaces: []*networkinterface.NetworkInterface{
			iface,
		},
		KnownState:   status.NetworkNone,
		DesiredState: status.NetworkReadyPull,
	}

	return &tasknetworkconfig.TaskNetworkConfig{
		NetworkNamespaces: []*tasknetworkconfig.NetworkNamespace{
			netNS,
		},
		NetworkMode: ecs.NetworkModeAwsvpc,
	}, nil
}

// CreateNetNS creates network namespace for the task.
func (c *containerd) CreateNetNS(netNSID string) error {
	// Check if the network namespace exists.
	nsExists, err := c.nsUtil.NSExists(netNSID)
	if err != nil {
		return errors.Wrapf(err, "failed to check netns %s", netNSID)
	}

	if nsExists {
		return nil
	}

	// If network namespace doesn't exist, create a new one.
	err = c.nsUtil.NewNetNS(netNSID)
	if err != nil {
		return errors.Wrapf(err, "failed to create netns %s", netNSID)
	}

	return nil
}

// CreateNetNS deletes network namespace of the task.
func (c *containerd) DeleteNetNS(netNSID string) error {
	// Check if the network namespace exists.
	nsExists, err := c.nsUtil.NSExists(netNSID)
	if err != nil {
		return errors.Wrapf(err, "failed to check netns %s", netNSID)
	}

	if !nsExists {
		return nil
	}

	err = c.nsUtil.DelNetNS(netNSID)
	if err != nil {
		return errors.Wrapf(err, "failed to delete netns %s", netNSID)
	}

	return nil
}

func (c *containerd) CreateDNSConfig(_ string, _ *tasknetworkconfig.NetworkNamespace) error {
	return nil
}

func (c *containerd) DeleteDNSConfig(_ string) error {
	return nil
}

func (c *containerd) GetNetNSPath(_ string) string {
	return c.nsUtil.NewNetNSID()
}

// ConfigureInterface configures task network interface.
func (c *containerd) ConfigureInterface(
	ctx context.Context,
	netNSID string,
	iface *networkinterface.NetworkInterface,
	netDAO netlibdata.NetworkDataClient,
) error {
	switch iface.InterfaceAssociationProtocol {
	case networkinterface.DefaultInterfaceAssociationProtocol:
		return c.configureRegularENI(ctx, netNSID, iface)
	default:
		return errors.Errorf("unknown ENI type %s", iface.InterfaceAssociationProtocol)
	}
	return nil
}

func (c *containerd) ConfigureAppMesh(ctx context.Context, netNSPath string, cfg *appmesh.AppMesh) error {
	return errors.New("not implemented")
}

func (c *containerd) ConfigureServiceConnect(
	ctx context.Context,
	netNSPath string,
	primaryIf *networkinterface.NetworkInterface,
	scConfig *serviceconnect.ServiceConnectConfig,
) error {
	return errors.New("not implemented")
}

// configureRegularENI configures a network interface for an ENI.
func (c *containerd) configureRegularENI(ctx context.Context, netNSID string, iface *networkinterface.NetworkInterface) error {
	var cniNetConf []ecscni.PluginConfig
	var add bool
	var err error

	// Set the log file path for CNI plugin.
	c.os.Setenv(VPCCNIPluginLogFileEnv, getCNIPluginLogfilePath())

	switch iface.DesiredStatus {
	case status.NetworkReadyPull:
		// Create the configuration for moving task ENI into the task namespace.
		cniNetConf = append(cniNetConf, newVPCENIConfigForENI(iface, netNSID, taskNetworkNamePrefix))
		// Create the configuration for creating task IAM role interface in the task namespace.
		cniNetConf = append(cniNetConf, newVPCENIConfigForBridge(netNSID, fargateBridgeNetworkName))

		add = true
	case status.NetworkDeleted:
		// Regular ENIs are used in single-use warmpool instances, so cleanup isn't necessary.
		cniNetConf = nil
		add = false
	}

	_, err = c.executeCNIPluginWithRetry(ctx, add, cniNetConf...)
	return err
}

// executeCNIPluginWithRetry retries the CNI plugin execution in case of failure.
func (c *containerd) executeCNIPluginWithRetry(
	ctx context.Context,
	add bool,
	cniNetConf ...ecscni.PluginConfig,
) ([]*types.Result, error) {
	var results []*types.Result
	var err error
	backoff := retry.NewExponentialBackoff(setupNSBackoffMin, setupNSBackoffMax,
		setupNSBackoffJitter, setupNSBackoffMultiple)

	err = retry.RetryNWithBackoff(
		backoff,
		setupNSMaxRetryCount,
		func() error {
			results, err = c.executeCNIPlugin(ctx, add, cniNetConf...)
			return err
		},
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to setup regular eni")
	}

	return results, nil
}
