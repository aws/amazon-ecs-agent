// +build linux,sudo

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
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	cgroup "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/stretchr/testify/assert"
)

func TestStartStopWithCgroup(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second
	cfg.TaskCPUMemLimit = config.ExplicitlyEnabled
	cfg.CgroupPath = "/cgroup"

	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "arn:aws:ecs:us-east-1:123456789012:task/testCgroup"
	testTask := createTestTask(taskArn)
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	for _, container := range testTask.Containers {
		container.TransitionDependenciesMap = make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet)
	}
	control := cgroup.New()
	taskEngine.(*DockerTaskEngine).resourceFields = &taskresource.ResourceFields{
		Control: control,
		IOUtil:  ioutilwrapper.NewIOUtil(),
	}
	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskIsRunning(stateChangeEvents, testTask)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskIsStopped(stateChangeEvents, testTask)

	// Should be stopped, let's verify it's still listed...
	task, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(taskArn)
	assert.True(t, ok, "Expected task to be present still, but wasn't")

	cgroupRoot, err := testTask.BuildCgroupRoot()
	assert.Nil(t, err)
	assert.True(t, control.Exists(cgroupRoot))

	task.SetSentStatus(apitask.TaskStopped) // cleanupTask waits for TaskStopped to be sent before cleaning
	time.Sleep(cfg.TaskCleanupWaitDuration)
	for i := 0; i < 60; i++ {
		_, ok = taskEngine.(*DockerTaskEngine).State().TaskByArn(taskArn)
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	assert.False(t, ok, "Expected container to have been swept but was not")
	assert.False(t, control.Exists(cgroupRoot))
}
