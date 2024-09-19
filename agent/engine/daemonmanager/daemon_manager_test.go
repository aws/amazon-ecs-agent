//go:build unit
// +build unit

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//      http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package daemonmanager

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	md "github.com/aws/amazon-ecs-agent/ecs-agent/manageddaemon"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

func TestCreateDaemonTask(t *testing.T) {

	cases := []struct {
		testName        string
		testDaemonName  string
		testImageRef    string
		testOtherMount  *md.MountPoint
		testHealthCheck []string
	}{
		{
			testName:        "Basic Daemon",
			testDaemonName:  TestDaemonName,
			testImageRef:    TestImageRef,
			testOtherMount:  &md.MountPoint{SourceVolumeID: TestOtherVolumeID, ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/", PropagationShared: false},
			testHealthCheck: []string{TestHealthString},
		},
		{
			testName:        "Daemon Updated Daemon Name",
			testDaemonName:  "TestDeemen",
			testImageRef:    TestImageRef,
			testOtherMount:  &md.MountPoint{SourceVolumeID: TestOtherVolumeID, ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/", PropagationShared: false},
			testHealthCheck: []string{TestHealthString},
		},
		{
			testName:        "Daemon Updated Image ref",
			testDaemonName:  TestDaemonName,
			testImageRef:    "TestOtherImage",
			testOtherMount:  &md.MountPoint{SourceVolumeID: TestOtherVolumeID, ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/", PropagationShared: false},
			testHealthCheck: []string{TestHealthString},
		},
		{
			testName:        "Daemon With Updated Mounts",
			testDaemonName:  TestDaemonName,
			testImageRef:    TestImageRef,
			testOtherMount:  &md.MountPoint{SourceVolumeID: "testUpdatedMountVolume", ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/", PropagationShared: false},
			testHealthCheck: []string{TestHealthString},
		},
		{
			testName:        "Daemon With Updated HealthCheck",
			testDaemonName:  TestDaemonName,
			testImageRef:    TestImageRef,
			testOtherMount:  &md.MountPoint{SourceVolumeID: TestOtherVolumeID, ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/"},
			testHealthCheck: []string{"testOtherHealth"},
		},
	}

	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			// mock mkdirAllAndChown
			origMkdir := mkdirAllAndChown
			defer func() { mkdirAllAndChown = origMkdir }()
			mkdirAllAndChown = func(path string, perm fs.FileMode, uid, gid int) error {
				return nil
			}
			// set up test managed daemon
			tmd := md.NewManagedDaemon(c.testDaemonName, TestImageTag)
			tmd.SetLoadedDaemonImageRef(c.testImageRef)
			// create required mounts and add all
			testAgentCommunicationMount := &md.MountPoint{SourceVolumeID: "agentCommunicationMount", ContainerPath: "/container/run/"}
			testApplicationLogMount := &md.MountPoint{SourceVolumeID: "applicationLogMount", ContainerPath: "/container/log/"}
			testMountPoints := []*md.MountPoint{}
			testMountPoints = append(testMountPoints, testAgentCommunicationMount, testApplicationLogMount, c.testOtherMount)
			tmd.SetMountPoints(testMountPoints)
			// add required healthcheck
			tmd.SetHealthCheck(c.testHealthCheck, 2*time.Minute, 2*time.Minute, 1)
			// create daemon manager for the above daemon
			testDaemonManager := NewDaemonManager(tmd)
			resultDaemonTask, _ := testDaemonManager.CreateDaemonTask()
			// validate daemon task configs
			assert.Equal(t, fmt.Sprintf("arn:::::/%s-", c.testDaemonName), strings.Split(resultDaemonTask.Arn, "-")[0]+"-", "Task Arn prefix should match Image Name ")
			assert.Equal(t, apitaskstatus.TaskRunning, resultDaemonTask.DesiredStatusUnsafe, "Task DesiredStatus should be running")
			assert.Equal(t, apitask.HostNetworkMode, resultDaemonTask.NetworkMode, "Task NetworkMode should be Host")
			assert.Equal(t, "EC2", resultDaemonTask.LaunchType, "Task LaunchType should be EC2")
			assert.Equal(t, true, resultDaemonTask.IsInternal, "Task IsInternal should be true")
			// validate task container
			assert.Equal(t, c.testImageRef, resultDaemonTask.Containers[0].Image, "Task Container Image Name should match image ref")
			// validate daemon container configs
			configRaw := resultDaemonTask.Containers[0].DockerConfig.Config
			hostConfigRaw := resultDaemonTask.Containers[0].DockerConfig.HostConfig
			var configMap map[string]interface{}
			var hostConfigMap map[string]interface{}
			json.Unmarshal([]byte(aws.StringValue(configRaw)), &configMap)
			json.Unmarshal([]byte(aws.StringValue(hostConfigRaw)), &hostConfigMap)
			// validate mount point count
			if containerMounts, ok := hostConfigMap["Mounts"].([]interface{}); ok {
				assert.Equal(t, len(containerMounts), 3, "Task should have Required container binds (2) + 1 other bind")
			} else {
				t.Errorf("Unable to find 'Mounts' in container definition map")
			}
			// validate healthcheck
			containerHealthCheck := configMap["Healthcheck"].(map[string]interface{})
			containerHealthCheckTest := containerHealthCheck["Test"].([]interface{})
			assert.Equal(t, c.testHealthCheck[0], containerHealthCheckTest[0].(string), "Container health check has changed")
		})
	}
}

func TestFailCreateDaemonTask_MissingMount(t *testing.T) {
	// mock mkdirAllAndChown
	origMkdir := mkdirAllAndChown
	defer func() { mkdirAllAndChown = origMkdir }()
	mkdirAllAndChown = func(path string, perm fs.FileMode, uid, gid int) error {
		return nil
	}
	// set up test managed daemon
	tmd := md.NewManagedDaemon(TestDaemonName, TestImageTag)
	tmd.SetLoadedDaemonImageRef(TestImageRef)
	// test failure with a missing applicationLogMount
	testAgentCommunicationMount := &md.MountPoint{SourceVolumeID: "agentCommunicationMount", ContainerPath: "/container/run/"}
	testOtherMount := &md.MountPoint{SourceVolumeID: TestOtherVolumeID, ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/"}
	testMountPoints := []*md.MountPoint{}
	testMountPoints = append(testMountPoints, testAgentCommunicationMount, testOtherMount)
	tmd.SetMountPoints(testMountPoints)
	testDaemonManager := NewDaemonManager(tmd)
	_, err := testDaemonManager.CreateDaemonTask()
	assert.EqualError(t, err, fmt.Sprintf("%s is an invalid managed daemon", TestDaemonName))

	// add required log mount but no healthcheck
	testApplicationLogMount := &md.MountPoint{SourceVolumeID: "applicationLogMount", ContainerPath: "/container/log/"}
	testMountPoints = append(testMountPoints, testApplicationLogMount)
	tmd.SetMountPoints(testMountPoints)
	testDaemonManager = NewDaemonManager(tmd)
	_, err = testDaemonManager.CreateDaemonTask()
	assert.Nil(t, err)

	// add required healthcheck
	testHealthCheck := []string{"test"}
	tmd.SetHealthCheck(testHealthCheck, 2*time.Minute, 2*time.Minute, 1)
	testDaemonManager = NewDaemonManager(tmd)
	resultDaemonTask, err := testDaemonManager.CreateDaemonTask()

	// validate daemon task configs
	assert.Equal(t, fmt.Sprintf("arn:::::/%s-", TestDaemonName), strings.Split(resultDaemonTask.Arn, "-")[0]+"-", "Task Arn prefix should match Image Name ")
	assert.Equal(t, apitaskstatus.TaskRunning, resultDaemonTask.DesiredStatusUnsafe, "Task DesiredStatus should be running")
	assert.Equal(t, apitask.HostNetworkMode, resultDaemonTask.NetworkMode, "Task NetworkMode should be Host")
	assert.Equal(t, "EC2", resultDaemonTask.LaunchType, "Task LaunchType should be EC2")
	assert.Equal(t, true, resultDaemonTask.IsInternal, "Task IsInternal should be true")

	// validate task container
	assert.Equal(t, TestImageRef, resultDaemonTask.Containers[0].Image, "Task Container Image Name should match image ref")

	// validate daemon container configs
	configRaw := resultDaemonTask.Containers[0].DockerConfig.Config
	hostConfigRaw := resultDaemonTask.Containers[0].DockerConfig.HostConfig
	var configMap map[string]interface{}
	var hostConfigMap map[string]interface{}
	json.Unmarshal([]byte(aws.StringValue(configRaw)), &configMap)
	json.Unmarshal([]byte(aws.StringValue(hostConfigRaw)), &hostConfigMap)
	// validate mount point count
	if containerMounts, ok := hostConfigMap["Mounts"].([]interface{}); ok {
		assert.Equal(t, len(containerMounts), 3, "Task should have Required container binds (2) + 1 other bind")
	} else {
		t.Errorf("Unable to find 'Mounts' in container definition map")
	}
	// validate healthcheck
	containerHealthCheck := configMap["Healthcheck"].(map[string]interface{})
	containerHealthCheckTest := containerHealthCheck["Test"].([]interface{})
	assert.Equal(t, testHealthCheck[0], containerHealthCheckTest[0].(string), "Container health check has changed")
}

func TestCreateDaemonTask_PrivilegeAndMountPropagation(t *testing.T) {
	// mock mkdirAllAndChown
	origMkdir := mkdirAllAndChown
	defer func() { mkdirAllAndChown = origMkdir }()
	mkdirAllAndChown = func(path string, perm fs.FileMode, uid, gid int) error {
		return nil
	}
	// set up test managed daemon
	tmd := md.NewManagedDaemon(TestDaemonName, TestImageTag)
	tmd.SetLoadedDaemonImageRef(TestImageRef)
	tmd.SetPrivileged(true)
	// test failure with a missing applicationLogMount
	testAgentCommunicationMount := &md.MountPoint{SourceVolumeID: "agentCommunicationMount", ContainerPath: "/container/run/"}
	testOtherMount := &md.MountPoint{SourceVolumeID: TestOtherVolumeID, ContainerPath: "/container/other/", SourceVolumeHostPath: "/var/ecs/other/", PropagationShared: true}
	testMountPoints := []*md.MountPoint{}
	testMountPoints = append(testMountPoints, testAgentCommunicationMount, testOtherMount)
	tmd.SetMountPoints(testMountPoints)
	testDaemonManager := NewDaemonManager(tmd)
	_, err := testDaemonManager.CreateDaemonTask()
	assert.EqualError(t, err, fmt.Sprintf("%s is an invalid managed daemon", TestDaemonName))

	// add required log mount but no healthcheck
	testApplicationLogMount := &md.MountPoint{SourceVolumeID: "applicationLogMount", ContainerPath: "/container/log/"}
	testMountPoints = append(testMountPoints, testApplicationLogMount)
	tmd.SetMountPoints(testMountPoints)
	testDaemonManager = NewDaemonManager(tmd)
	_, err = testDaemonManager.CreateDaemonTask()
	assert.Nil(t, err)

	// add required healthcheck
	testHealthCheck := []string{"test"}
	tmd.SetHealthCheck(testHealthCheck, 2*time.Minute, 2*time.Minute, 1)
	testDaemonManager = NewDaemonManager(tmd)
	resultDaemonTask, err := testDaemonManager.CreateDaemonTask()

	// validate daemon task configs
	assert.Equal(t, fmt.Sprintf("arn:::::/%s-", TestDaemonName), strings.Split(resultDaemonTask.Arn, "-")[0]+"-", "Task Arn prefix should match Image Name ")
	assert.Equal(t, apitaskstatus.TaskRunning, resultDaemonTask.DesiredStatusUnsafe, "Task DesiredStatus should be running")
	assert.Equal(t, apitask.HostNetworkMode, resultDaemonTask.NetworkMode, "Task NetworkMode should be Host")
	assert.Equal(t, "EC2", resultDaemonTask.LaunchType, "Task LaunchType should be EC2")
	assert.Equal(t, true, resultDaemonTask.IsInternal, "Task IsInteral should be true")

	// validate task container
	assert.Equal(t, TestImageRef, resultDaemonTask.Containers[0].Image, "Task Container Image Name should match image ref")

	// validate daemon container configs
	configRaw := resultDaemonTask.Containers[0].DockerConfig.Config
	hostConfigRaw := resultDaemonTask.Containers[0].DockerConfig.HostConfig
	var configMap map[string]interface{}
	var hostConfigMap map[string]interface{}
	json.Unmarshal([]byte(aws.StringValue(configRaw)), &configMap)
	json.Unmarshal([]byte(aws.StringValue(hostConfigRaw)), &hostConfigMap)
	// validate mount point has One mount with Shared Propagation
	if containerMounts, ok := hostConfigMap["Mounts"].([]interface{}); ok {
		res := strings.Count(fmt.Sprintf("%v", containerMounts), "Propagation:shared")
		assert.Equal(t, res, 1, "Task should have only one mount with shared Propagation")
	} else {
		t.Errorf("missing 'Mounts' in hostConfigMap")
	}
	// validate privileged
	containerPrivileged := hostConfigMap["Privileged"].(bool)
	assert.True(t, containerPrivileged, "Daemon Container should be privileged")
	// validate healthcheck
	containerHealthCheck := configMap["Healthcheck"].(map[string]interface{})
	containerHealthCheckTest := containerHealthCheck["Test"].([]interface{})
	assert.Equal(t, testHealthCheck[0], containerHealthCheckTest[0].(string), "Container health check has changed")
}

// containsString will typecast elements to strings and compare to the target
func containsString(arr []interface{}, target string) bool {
	for _, val := range arr {
		if val.(string) == target {
			return true
		}
	}
	return false
}
