package netlib

import (
	"context"

	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/tasknetworkconfig"
	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/platform"

	"github.com/pkg/errors"
)

type NetworkBuilder interface {
	BuildTaskNetworkConfiguration(taskID string, taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error)

	Start(ctx context.Context, taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error

	Stop(ctx context.Context, taskNetConfig *tasknetworkconfig.TaskNetworkConfig) error
}

type networkBuilder struct {
	platformAPI platform.API
}

func NewNetworkBuilder(platformString string) (NetworkBuilder, error) {
	pAPI, err := platform.NewPlatform(platformString, ecscni.NewNetNSUtil())
	if err != nil {
		return nil, errors.Wrap(err, "failed to instantiate network builder")
	}
	return &networkBuilder{
		platformAPI: pAPI,
	}, nil
}

func (nb *networkBuilder) BuildTaskNetworkConfiguration(taskID string, taskPayload *ecsacs.Task) (*tasknetworkconfig.TaskNetworkConfig, error) {
	return nb.platformAPI.BuildTaskNetworkConfiguration(taskID, taskPayload)
}

func (nb *networkBuilder) Start(ctx context.Context, netConfig *tasknetworkconfig.TaskNetworkConfig) error {
	return nil
}

func (nb *networkBuilder) Stop(ctx context.Context, netConfig *tasknetworkconfig.TaskNetworkConfig) error {
	return nil
}
