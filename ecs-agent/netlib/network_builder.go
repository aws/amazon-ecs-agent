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
	// BuildTaskNetworkConfiguration models the task's networking from its
	// payload. Namespaces of the task that already exist, such as one built
	// from an ENI attachment ahead of the payload, are passed as existing and
	// are not modified; see platform.API.BuildTaskNetworkConfiguration.
	BuildTaskNetworkConfiguration(
		taskID string,
		taskPayload *ecsacs.Task,
		existing ...*tasknetworkconfig.NetworkNamespace,
	) (*tasknetworkconfig.TaskNetworkConfig, error)

	// BuildAttachmentNetworkConfiguration models the networking a task ENI
	// attachment establishes ahead of the task payload; see
	// platform.API.BuildAttachmentNetworkConfiguration.
	BuildAttachmentNetworkConfiguration(
		taskID string,
		eni *ecsacs.ElasticNetworkInterface,
	) (*tasknetworkconfig.AttachmentNetworkConfig, error)

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
	taskID string,
	taskPayload *ecsacs.Task,
	existing ...*tasknetworkconfig.NetworkNamespace,
) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nb.platformAPI.BuildTaskNetworkConfiguration(taskID, taskPayload, existing...)
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

	// Configure each interface inside the network namespace. In the
	// READY_PULL -> CONFIGURED transition this brings interfaces the task
	// payload added after the namespace was built up to READY_PULL; the
	// interfaces already there are left alone.
	err = nb.configureNetNSInterfaces(ctx, netNS)
	if err != nil {
		return err
	}
	// Configure AppMesh and service connect rules in the netns, once every
	// interface the task declares is in place. A consumer that drives the
	// namespace from READY_PULL straight to READY gets the same result: the
	// interface pass above has just brought any added interface in.
	if (netNS.KnownState == status.NetworkConfigured || netNS.KnownState == status.NetworkReadyPull) &&
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

// interfaceTargetStatus maps a namespace's desired state to the status its
// interfaces are driven to. CONFIGURED is a namespace status: it says every
// interface the task declares has reached READY_PULL, so interfaces are driven
// to READY_PULL for it.
func interfaceTargetStatus(netNSDesired status.NetworkStatus) status.NetworkStatus {
	if netNSDesired == status.NetworkConfigured {
		return status.NetworkReadyPull
	}
	return netNSDesired
}

// configureNetNSInterfaces executes the platform API to configure every interface inside a network namespace.
func (nb *networkBuilder) configureNetNSInterfaces(ctx context.Context, netNS *tasknetworkconfig.NetworkNamespace) error {
	var errs error
	target := interfaceTargetStatus(netNS.DesiredState)
	for _, iface := range netNS.NetworkInterfaces {
		logFields := logger.Fields{
			"Interface":     iface,
			"NetNSName":     netNS.Name,
			"KnownStatus":   iface.KnownStatus,
			"DesiredStatus": iface.DesiredStatus,
		}
		if iface.KnownStatus == target {
			logger.Debug("Interface already in desired state", logFields)
			continue
		}

		// The interface desired status is driven by the network namespace's desired state.
		logger.Debug("Configuring interface", logFields)
		iface.DesiredStatus = target

		err := nb.platformAPI.ConfigureInterface(ctx, netNS.Path, iface, nb.networkDAO)
		if err != nil {
			if netNS.DesiredState == status.NetworkDeleted {
				logger.Error(fmt.Sprintf("Failed to configure interface: %v", err), logFields)
				errs = multierror.Append(err, errs)
			} else {
				return err
			}
		}
		iface.KnownStatus = target

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
