// +build !windows
// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package engine

import (
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

// isParallelPullCompatible checks the docker version and return true if
// docker version >= 1.11.1
// TODO get rid of this altogether once support for pre 1.9 Docker versions
// gets dropped.
// See https://github.com/moby/moby/pull/16893 for details
func (engine *DockerTaskEngine) isParallelPullCompatible() bool {
	version, err := engine.Version()
	if err != nil {
		seelog.Warnf("Task engine: failed to get docker version: %v", err)
		return false
	}

	match, err := utils.Version(version).Matches(">=1.11.1")
	if err != nil {
		seelog.Warnf("Task engine: Could not compare docker version: %v", err)
		return false
	}

	if match {
		seelog.Debugf("Task engine: Found Docker version [%s]. Enabling concurrent pull", version)
		return true
	}

	return false
}
