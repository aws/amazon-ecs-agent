// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package cgroup

import (
	"fmt"
	"github.com/aws/amazon-ecs-agent/agent/config"

	"github.com/cihub/seelog"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// Init is used to setup the cgroup root for ecs
func (c *control) Init(config *config.Config) error {
	seelog.Infof("Creating root ecs cgroup: %s.slice", config.CgroupPrefix)

	// Build cgroup spec
	cgroupSpec := &Spec{
		Root:  fmt.Sprintf("%s.slice", config.CgroupPrefix),
		Specs: &specs.LinuxResources{},
	}
	_, err := c.Create(cgroupSpec)
	return err
}
