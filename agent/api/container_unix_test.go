// +build !windows,!integration
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

package api

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/stretchr/testify/assert"
)

func TestBuildResourceDependency(t *testing.T) {
	container := Container{TransitionDependenciesMap: make(map[ContainerStatus]TransitionDependencySet)}
	depResourceName := "cgroup"
	container.BuildResourceDependency(depResourceName, taskresource.ResourceStatus(cgroup.CgroupCreated), ContainerRunning)
	assert.NotNil(t, container.TransitionDependenciesMap)
	resourceDep := container.TransitionDependenciesMap[ContainerRunning].ResourceDependencies
	assert.Len(t, container.TransitionDependenciesMap, 1)
	assert.Len(t, resourceDep, 1)
	assert.Equal(t, depResourceName, resourceDep[0].Name)
	assert.Equal(t, taskresource.ResourceStatus(cgroup.CgroupCreated), resourceDep[0].GetRequiredStatus())
}
