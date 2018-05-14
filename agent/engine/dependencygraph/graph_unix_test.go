// +build linux,unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package dependencygraph

import (
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/stretchr/testify/assert"
)

func TestVerifyCgroupDependenciesResolved(t *testing.T) {
	testcases := []struct {
		Name             string
		TargetKnown      apicontainer.ContainerStatus
		TargetDep        apicontainer.ContainerStatus
		DependencyKnown  taskresource.ResourceStatus
		RequiredStatus   taskresource.ResourceStatus
		ExpectedResolved bool
	}{
		{
			Name:             "resource none,container pull depends on resource created",
			TargetKnown:      apicontainer.ContainerStatusNone,
			TargetDep:        apicontainer.ContainerPulled,
			DependencyKnown:  taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			RequiredStatus:   taskresource.ResourceStatus(cgroup.CgroupCreated),
			ExpectedResolved: false,
		},
		{
			Name:             "resource created,container pull depends on resource created",
			TargetKnown:      apicontainer.ContainerStatusNone,
			TargetDep:        apicontainer.ContainerPulled,
			DependencyKnown:  taskresource.ResourceStatus(cgroup.CgroupCreated),
			RequiredStatus:   taskresource.ResourceStatus(cgroup.CgroupCreated),
			ExpectedResolved: true,
		},
		{
			Name:             "resource none,container create depends on resource created",
			TargetKnown:      apicontainer.ContainerStatusNone,
			TargetDep:        apicontainer.ContainerCreated,
			DependencyKnown:  taskresource.ResourceStatus(cgroup.CgroupStatusNone),
			RequiredStatus:   taskresource.ResourceStatus(cgroup.CgroupCreated),
			ExpectedResolved: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			cgroupResource := &cgroup.CgroupResource{}
			cgroupResource.SetKnownStatus(tc.DependencyKnown)
			target := &apicontainer.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet),
			}
			target.BuildResourceDependency("cgroup", tc.RequiredStatus, tc.TargetDep)
			resources := make(map[string]taskresource.TaskResource)
			resources[cgroupResource.GetName()] = cgroupResource
			resolved := verifyResourceDependenciesResolved(target, resources)
			assert.Equal(t, tc.ExpectedResolved, resolved)
		})
	}
}
