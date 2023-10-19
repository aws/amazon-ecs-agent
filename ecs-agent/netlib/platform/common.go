package platform

import (
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/netlinkwrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/oswrapper"
	"github.com/aws/amazon-ecs-agent/ecs-agent/volume"

	"github.com/containernetworking/cni/pkg/types"
)

// common will be embedded within every implementation of the platform API.
// It contains all fields and methods that can be commonly used by all
// platforms.
type common struct {
	nsUtil             ecscni.NetNSUtil
	taskVolumeAccessor volume.VolumeAccessor
	os                 oswrapper.OS
	ioutil             ioutilwrapper.IOUtil
	netlink            netlinkwrapper.NetLink
	stateDBDir         string
	cniClient          ecscni.CNI
}

// executeCNIPlugin executes CNI plugins with the given network configs and a timeout context.
func (c *common) executeCNIPlugin(
	ctx context.Context,
	add bool,
	cniNetConf ...ecscni.PluginConfig,
) ([]*types.Result, error) {
	var timeout time.Duration
	var results []*types.Result
	var err error

	if add {
		timeout = nsSetupTimeoutDuration
	} else {
		timeout = nsCleanupTimeoutDuration
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, cfg := range cniNetConf {
		if add {
			var addResult types.Result
			addResult, err = c.cniClient.Add(ctx, cfg)
			if addResult != nil {
				results = append(results, &addResult)
			}
		} else {
			err = c.cniClient.Del(ctx, cfg)
		}

		if err != nil {
			break
		}
	}

	return results, err
}
