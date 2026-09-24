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
	"fmt"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	netlibdata "github.com/aws/amazon-ecs-agent/ecs-agent/netlib/data"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/networkinterface"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"

	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

type NetworkBuilder interface {
	BuildTaskNetworkConfiguration(taskID string, taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error)

	// BuildAttachmentNetworkConfiguration models the networking a task ENI
	// attachment establishes ahead of the task payload; see
	// platform.API.BuildAttachmentNetworkConfiguration.
	BuildAttachmentNetworkConfiguration(
		taskID string,
		eni *ecsacs.ElasticNetworkInterface,
	) (*tasknetworkconfig.AttachmentNetworkConfig, error)

	// ExtendTaskNetworkConfiguration completes a task's networking around
	// namespaces already provisioned ahead of its payload. The payload's
	// remaining interfaces are placed by the platform's layout rule (see
	// platform.API.ExtendTaskNetworkConfiguration); those that join an
	// existing namespace are configured into it up to the namespace's current
	// state, so this touches the host where Build does not. The returned
	// namespaces carry the state of the work done, existing and new alike,
	// ready to be persisted and managed; the existing models are not modified.
	ExtendTaskNetworkConfiguration(
		ctx context.Context,
		taskID string,
		taskPayload *ecsacs.Task,
		existing ...*tasknetworkconfig.NetworkNamespace,
	) (*tasknetworkconfig.TaskNetworkConfig, error)

	Start(ctx context.Context, mode types.NetworkMode, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error

	Stop(ctx context.Context, mode types.NetworkMode, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error
}

type networkBuilder struct {
	platformAPI    platform.API
	metricsFactory metrics.EntryFactory
	networkDAO     netlibdata.NetworkDataClient
}

func NewNetworkBuilder(
	platformConfig platform.Config,
	metricsFactory metrics.EntryFactory,
	volumeAccessor volume.TaskVolumeAccessor,
	networkDao data.NetworkDataClient,
	stateDBDir string) (NetworkBuilder, error) {
	pAPI, err := platform.NewPlatform(
		platformConfig,
		volumeAccessor,
		stateDBDir,
		netwrapper.NewNet(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to instantiate network builder")
	}

	return &networkBuilder{
		platformAPI:    pAPI,
		metricsFactory: metricsFactory,
		networkDAO:     networkDao,
	}, nil
}

// BuildTaskNetworkConfiguration builds the task's network configuration
func (nb *networkBuilder) BuildTaskNetworkConfiguration(
	taskID string, taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nb.platformAPI.BuildTaskNetworkConfiguration(taskID, taskPayload)
}

func (nb *networkBuilder) ExtendTaskNetworkConfiguration(
	ctx context.Context,
	taskID string,
	taskPayload *ecsacs.Task,
	existing ...*tasknetworkconfig.NetworkNamespace,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	netConfig, err := nb.platformAPI.ExtendTaskNetworkConfiguration(taskID, taskPayload, existing)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]*tasknetworkconfig.NetworkNamespace, len(existing))
	for _, netNS := range existing {
		byName[netNS.Name] = netNS
	}
	for _, netNS := range netConfig.NetworkNamespaces {
		prior, ok := byName[netNS.Name]
		if !ok {
			continue
		}
		if err := nb.carryNamespaceState(ctx, taskID, prior, netNS); err != nil {
			return nil, err
		}
	}
	return netConfig, nil
}

// carryNamespaceState brings a rebuilt namespace up to the state of the
// namespace it describes. The state the prior model recorded is copied onto
// the interfaces the rebuilt one shares with it, so the work already done is
// not repeated. Interfaces the rebuilt model adds are then configured through
// the pull phase, which every interface passes while the namespace is at
// READY_PULL and which the namespace's next transition would otherwise skip
// for them. The prior model is not modified.
func (nb *networkBuilder) carryNamespaceState(
	ctx context.Context,
	taskID string,
	prior, netNS *tasknetworkconfig.NetworkNamespace,
) error {
	priorIfaces := make(map[string]*networkinterface.NetworkInterface, len(prior.NetworkInterfaces))
	for _, iface := range prior.NetworkInterfaces {
		priorIfaces[iface.ID] = iface
	}
	added := 0
	for _, iface := range netNS.NetworkInterfaces {
		if have, ok := priorIfaces[iface.ID]; ok {
			iface.KnownStatus = have.KnownStatus
			iface.DesiredStatus = have.DesiredStatus
			continue
		}
		added++
	}
	netNS.KnownState = prior.KnownState
	netNS.DesiredState = prior.DesiredState

	if added == 0 || prior.KnownState == status.NetworkNone {
		return nil
	}

	logger.Info("Configuring interfaces added to a provisioned network namespace", logger.Fields{
		"NetNSName":           netNS.Name,
		"AddedInterfaceCount": added,
		"KnownState":          netNS.KnownState,
	})
	desired := netNS.DesiredState
	netNS.DesiredState = status.NetworkReadyPull
	err := nb.Start(ctx, netNS.NetworkMode, taskID, netNS)
	netNS.DesiredState = desired
	if err != nil {
		return errors.Wrapf(err, "failed to configure interfaces added to network namespace %s", netNS.Name)
	}
	return nil
}

func (nb *networkBuilder) BuildAttachmentNetworkConfiguration(
	taskID string,
	eni *ecsacs.ElasticNetworkInterface,
) (*tasknetworkconfig.AttachmentNetworkConfig, error) {
	return nb.platformAPI.BuildAttachmentNetworkConfiguration(taskID, eni)
}

// Start builds up a particular network namespace for the task as per desired configuration.
func (nb *networkBuilder) Start(
	ctx context.Context,
	mode types.NetworkMode, taskID string,
	netNS *tasknetworkconfig.NetworkNamespace,
) error {
	logFields := map[string]interface{}{
		"NetworkMode":           mode,
		"NetNSName":             netNS.Name,
		"NetNSPath":             netNS.Path,
		"AppMeshEnabled":        netNS.AppMeshConfig != nil,
		"ServiceConnectEnabled": netNS.ServiceConnectConfig != nil,
	}
	metricEntry := nb.metricsFactory.New(metrics.BuildNetworkNamespaceMetricName).WithFields(logFields)

	netNS.Mutex.Lock()
	defer netNS.Mutex.Unlock()

	logger.Info("Starting network namespace setup", logFields)

	var err error
	switch mode {
	case types.NetworkModeAwsvpc:
		err = nb.startAWSVPC(ctx, taskID, netNS)
	case types.NetworkModeHost:
		err = nb.platformAPI.HandleHostMode()
	case "daemon-bridge":
		err = nb.platformAPI.ConfigureDaemonNetNS(netNS)
	default:
		err = errors.New("invalid network mode: " + string(mode))
	}

	metricEntry.Done(err)

	return err
}

func (nb *networkBuilder) Stop(ctx context.Context, mode types.NetworkMode, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	logFields := map[string]interface{}{
		"NetworkMode":           mode,
		"NetNSName":             netNS.Name,
		"NetNSPath":             netNS.Path,
		"AppMeshEnabled":        netNS.AppMeshConfig != nil,
		"ServiceConnectEnabled": netNS.ServiceConnectConfig != nil,
	}
	metricEntry := nb.metricsFactory.New(metrics.DeleteNetworkNamespaceMetricName).WithFields(logFields)

	netNS.Mutex.Lock()
	defer netNS.Mutex.Unlock()

	logger.Info("Deleting network namespace setup", logFields)

	var err error
	switch mode {
	case types.NetworkModeAwsvpc:
		err = nb.stopAWSVPC(ctx, netNS)
	case types.NetworkModeHost:
		err = nb.platformAPI.HandleHostMode()
	case "daemon-bridge":
		err = nb.platformAPI.StopDaemonNetNS(ctx, netNS)
	default:
		err = errors.New("invalid network mode: " + string(mode))
	}

	metricEntry.Done(err)

	return err
}

// startAWSVPC executes the required platform API methods in order to configure
// the task's network namespace running in AWSVPC mode.
func (nb *networkBuilder) startAWSVPC(ctx context.Context, taskID string, netNS *tasknetworkconfig.NetworkNamespace) error {
	var err error
	if netNS.DesiredState == status.NetworkDeleted {
		return errors.New("invalid transition state encountered: " + netNS.DesiredState.String())
	}

	// Create the network namespace and setup DNS configuration within the netns.
	// This has to happen before any CNI plugin is executed.
	if netNS.KnownState == status.NetworkNone &&
		netNS.DesiredState == status.NetworkReadyPull {

		logger.Debug("Creating netns: " + netNS.Path)
		// Create network namespace on the host.
		err = nb.platformAPI.CreateNetNS(netNS.Path)
		if err != nil {
			return err
		}

		logger.Debug("Creating DNS config files")

		// Create necessary DNS config files for the netns.
		err = nb.platformAPI.CreateDNSConfig(taskID, netNS)
		if err != nil {
			return err
		}
	}

	// Configure each interface inside the network namespace.
	err = nb.configureNetNSInterfaces(ctx, netNS)
	if err != nil {
		return err
	}
	// Configure AppMesh and service connect rules in the netns.
	if netNS.KnownState == status.NetworkReadyPull &&
		netNS.DesiredState == status.NetworkReady {
		if netNS.AppMeshConfig != nil {
			logger.Debug("Configuring AppMesh", logger.Fields{
				"AppMeshConfig": netNS.AppMeshConfig,
			})

			err = nb.platformAPI.ConfigureAppMesh(ctx, netNS.Path, netNS.AppMeshConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to configure AppMesh in netns %s", netNS.Name)
			}
		}

		if netNS.ServiceConnectConfig != nil {
			// The hosts file is first written in the NONE -> READY_PULL
			// transition, which may precede delivery of the ServiceConnect
			// hostname mappings when the namespace is built ahead of the task
			// payload. Recreating the DNS config files is idempotent.
			err = nb.platformAPI.CreateDNSConfig(taskID, netNS)
			if err != nil {
				return errors.Wrapf(err, "failed to recreate DNS config in netns %s", netNS.Name)
			}

			logger.Debug("Configuring ServiceConnect", logger.Fields{
				"ServiceConnectConfig": netNS.ServiceConnectConfig,
			})

			err = nb.platformAPI.ConfigureServiceConnect(
				ctx, netNS.Path, netNS.GetPrimaryInterface(), netNS.ServiceConnectConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to configure ServiceConnect in netns %s", netNS.Name)
			}
		}
	}

	return err
}

// configureNetNSInterfaces executes the platform API to configure every interface inside a network namespace.
func (nb *networkBuilder) configureNetNSInterfaces(ctx context.Context, netNS *tasknetworkconfig.NetworkNamespace) error {
	var errs error
	for _, iface := range netNS.NetworkInterfaces {
		logFields := logger.Fields{
			"Interface":     iface,
			"NetNSName":     netNS.Name,
			"KnownStatus":   iface.KnownStatus,
			"DesiredStatus": iface.DesiredStatus,
		}
		if iface.KnownStatus == netNS.DesiredState {
			logger.Debug("Interface already in desired state", logFields)
			continue
		}

		// The interface desired status is driven by the network namespace's desired state.
		logger.Debug("Configuring interface", logFields)
		iface.DesiredStatus = netNS.DesiredState

		err := nb.platformAPI.ConfigureInterface(ctx, netNS.Path, iface, nb.networkDAO)
		if err != nil {
			if netNS.DesiredState == status.NetworkDeleted {
				logger.Error(fmt.Sprintf("Failed to configure interface: %v", err), logFields)
				errs = multierror.Append(err, errs)
			} else {
				return err
			}
		}
		iface.KnownStatus = netNS.DesiredState

		// Save new state of the network interface in the database.
		if err = nb.networkDAO.SaveNetworkNamespace(netNS); err != nil {
			if netNS.DesiredState == status.NetworkDeleted {
				logger.Error(fmt.Sprintf("Failed to persist interface state: %v", err), logFields)
				errs = multierror.Append(err, errs)
			} else {
				return err
			}
		}
	}

	return errs
}

func (nb *networkBuilder) stopAWSVPC(ctx context.Context, netNS *tasknetworkconfig.NetworkNamespace) error {
	var errs error
	if netNS.DesiredState != status.NetworkDeleted {
		return errors.New("invalid transition state encountered: " + netNS.DesiredState.String())
	}

	logFields := logger.Fields{
		"NetNSName": netNS.Name,
	}
	err := nb.configureNetNSInterfaces(ctx, netNS)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to cleanup interfaces in netns: %v", err), logFields)
		errs = multierror.Append(err, errs)
	}

	err = nb.platformAPI.DeleteDNSConfig(netNS.Name)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to cleanup DNS config files: %v", err))
		errs = multierror.Append(err, errs)
	}

	err = nb.platformAPI.DeleteNetNS(netNS.Path)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to delete network namespace: %v", err), logFields)
		errs = multierror.Append(err, errs)
	}

	return errs
}
