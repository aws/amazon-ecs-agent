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

package platform

import (
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-agent/netlib/model/ecscni"

	"github.com/containernetworking/cni/pkg/types"
)

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
