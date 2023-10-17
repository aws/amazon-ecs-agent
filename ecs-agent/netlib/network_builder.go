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

package netlib

import (
	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type NetworkBuilder interface {
	BuildTaskNetworkConfiguration(taskID string, taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error)

	Start(ctx context.Context, mode string, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error

	Stop(ctx context.Context, mode string, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error
}

type networkBuilder struct {
	platformAPI    platform.API
	metricsFactory metrics.EntryFactory
	volumeAccessor volume.VolumeAccessor
}

func NewNetworkBuilder(
	platformString string,
	metricsFactory metrics.EntryFactory,
	volumeAccessor volume.VolumeAccessor,
	stateDBDir string) (NetworkBuilder, error) {
	pAPI, err := platform.NewPlatform(
		platformString,
		ecscni.NewNetNSUtil(),
		volumeAccessor,
		oswrapper.NewOS(),
		ioutilwrapper.NewIOUtil(),
		netlinkwrapper.New(),
		stateDBDir,
		ecscni.NewCNIClient([]string{platform.CNIPluginPathDefault}),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to instantiate network builder")
	}
	return &networkBuilder{
		platformAPI:    pAPI,
		metricsFactory: metricsFactory,
	}, nil
}

// BuildTaskNetworkConfiguration builds the task's network configuration
func (nb *networkBuilder) BuildTaskNetworkConfiguration(
	taskID string, taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nb.platformAPI.BuildTaskNetworkConfiguration(taskID, taskPayload)
}

// Start builds up a particular network namespace for the task as per desired configuration.
func (nb *networkBuilder) Start(
	ctx context.Context,
	mode string, taskID string,
	netNS *tasknetworkconfig.NetworkNamespace,
) error {
	netNS.Mutex.Lock()
	defer netNS.Mutex.Unlock()

	logrus.WithFields(logrus.Fields{
		"NetworkMode":           mode,
		"NetNSName":             netNS.Name,
		"NetNSPath":             netNS.Path,
		"AppMeshEnabled":        netNS.AppMeshConfig != nil,
		"ServiceConnectEnabled": netNS.ServiceConnectConfig != nil,
	}).Info("Starting network namespace setup")

	var err error
	switch mode {
	case ecs.NetworkModeAwsvpc:
		err = nb.startAWSVPC(ctx, taskID, netNS)
	default:
		err = errors.New("invalid network mode: " + mode)
	}

	return err
}

func (nb *networkBuilder) Stop(ctx context.Context, mode string, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	// TODO: To be implemented.
	return nil
}

// startAWSVPC executes the required platform API methods in order to configure
// the task's network namespace running in AWSVPC mode.
func (nb *networkBuilder) startAWSVPC(ctx context.Context, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	if netNS.DesiredState == status.NetworkDeleted {
		return errors.New("invalid transition state encountered: " + netNS.DesiredState.String())
	}

	// Create the network namespace and setup DNS configuration within the netns.
	// This has to happen before any CNI plugin is executed.
	if netNS.KnownState == status.NetworkNone &&
		netNS.DesiredState == status.NetworkReadyPull {

		logrus.Debugf("Creating netns: %s", netNS.Path)
		// Create network namespace on the host.
		err := nb.platformAPI.CreateNetNS(netNS.Path)
		if err != nil {
			return err
		}

		logrus.Debug("Creating DNS config files")

		// Create necessary DNS config files for the netns.
		err = nb.platformAPI.CreateDNSConfig(taskID, netNS)
		if err != nil {
			return err
		}
	}

	// Execute CNI plugins to configure the interfaces in the namespace.
	// Depending on the type of interfaces in the netns, there maybe operations
	// to execute when the netns desired status is READY_PULL and READY.
	for _, iface := range netNS.NetworkInterfaces {
		logrus.WithFields(logrus.Fields{
			"Interface": iface,
			"NetNSName": netNS.Name,
		}).Debug("Configuring interface")

		err := nb.platformAPI.ConfigureInterface(ctx, netNS.Path, iface)
		if err != nil {
			return err
		}
	}

	// Configure AppMesh and service connect rules in the netns.
	if netNS.KnownState == status.NetworkReadyPull &&
		netNS.DesiredState == status.NetworkReady {
		if netNS.AppMeshConfig != nil {
			logrus.WithFields(logrus.Fields{
				"AppMeshConfig": netNS.AppMeshConfig,
			}).Debug("Configuring AppMesh")

			err := nb.platformAPI.ConfigureAppMesh(ctx, netNS.Path, netNS.AppMeshConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to configure AppMesh in netns %s", netNS.Name)
			}
		}

		if netNS.ServiceConnectConfig != nil {
			logrus.WithFields(logrus.Fields{
				"ServiceConnectConfig": netNS.ServiceConnectConfig,
			}).Debug("Configuring ServiceConnect")

			err := nb.platformAPI.ConfigureServiceConnect(
				ctx, netNS.Path, netNS.GetPrimaryInterface(), netNS.ServiceConnectConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to configure ServiceConnect in netns %s", netNS.Name)
			}
		}
	}

	return nil
}
