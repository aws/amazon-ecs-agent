// +build linux,sudo

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

package engine

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	cgroup "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLogSenderImage = "amazonlinux:2"
	testFluentbitImage = "amazon/aws-for-fluent-bit:latest"
	testVolumeImage    = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
	testCluster        = "testCluster"
	validTaskArnPrefix = "arn:aws:ecs:region:account-id:task/"
	testDataDir        = "/var/lib/ecs/data/"
	testDataDirOnHost  = "/var/lib/ecs/"
	testInstanceID     = "testInstanceID"
	testTaskDefFamily  = "testFamily"
	testTaskDefVersion = "1"
	testECSRegion      = "us-east-1"
	testLogGroupName   = "test-fluentbit"
	testLogGroupPrefix = "firelens-fluentbit-"
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
		container.TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	}
	control := cgroup.New()

	commonResources := &taskresource.ResourceFieldsCommon{
		IOUtil: ioutilwrapper.NewIOUtil(),
	}

	taskEngine.(*DockerTaskEngine).resourceFields = &taskresource.ResourceFields{
		Control:              control,
		ResourceFieldsCommon: commonResources,
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

	task.SetSentStatus(apitaskstatus.TaskStopped) // cleanupTask waits for TaskStopped to be sent before cleaning
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

func TestLocalHostVolumeMount(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	// creates a task with local volume
	testTask := createTestLocalVolumeMountTask()
	stateChangeEvents := taskEngine.StateChangeEvents()
	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskIsRunning(stateChangeEvents, testTask)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskIsStopped(stateChangeEvents, testTask)

	assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
	assert.Equal(t, 0, *testTask.Containers[0].GetKnownExitCode(), "Wrong exit code")
	data, err := ioutil.ReadFile(filepath.Join("/var/lib/docker/volumes/", testTask.Volumes[0].Volume.Source(), "/_data", "hello-from-container"))
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, "empty-data-volume", strings.TrimSpace(string(data)), "Incorrect file contents")
}

func createTestLocalVolumeMountTask() *apitask.Task {
	testTask := createTestTask("testLocalHostVolumeMount")
	testTask.Volumes = []apitask.TaskVolume{{Name: "test-tmp", Volume: &taskresourcevolume.LocalDockerVolume{}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{{ContainerPath: "/host/tmp", SourceVolume: "test-tmp"}}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.Containers[0].Command = []string{`echo -n "empty-data-volume" > /host/tmp/hello-from-container;`}
	return testTask
}

func TestFirelensFluentbit(t *testing.T) {
	// Skipping the test for arm as they do not have official support for Arm images
	if runtime.GOARCH == "arm64" {
		t.Skip("Skipping test, unsupported image for arm64")
	}
	cfg := defaultTestConfigIntegTest()
	cfg.DataDir = testDataDir
	cfg.DataDirOnHost = testDataDirOnHost
	cfg.TaskCleanupWaitDuration = 1 * time.Second
	cfg.Cluster = testCluster
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	testTask := createFirelensTask(t)
	taskEngine.(*DockerTaskEngine).resourceFields = &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			EC2InstanceID: testInstanceID,
		},
	}
	go taskEngine.AddTask(testTask)
	testEvents := InitEventCollection(taskEngine)

	//Verify logsender container is running
	err := VerifyContainerStatus(apicontainerstatus.ContainerRunning, testTask.Arn+":logsender", testEvents, t)
	assert.NoError(t, err, "Verify logsender container is running")

	//Verify firelens container is running
	err = VerifyContainerStatus(apicontainerstatus.ContainerRunning, testTask.Arn+":firelens", testEvents, t)
	assert.NoError(t, err, "Verify firelens container is running")

	//Verify task is in running state
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTask.Arn, testEvents, t)
	assert.NoError(t, err, "Not verified task running")

	//Verify logsender container is stopped
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":logsender", testEvents, t)
	assert.NoError(t, err)

	//Verify firelens container is stopped
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":firelens", testEvents, t)
	assert.NoError(t, err)

	//Verify the task itself has stopped
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTask.Arn, testEvents, t)
	assert.NoError(t, err)

	taskID, err := testTask.GetID()

	//declare a cloudwatch client
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(testECSRegion))
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(testLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("firelens-fluentbit-logsender-firelens-%s", taskID)),
	}

	// wait for the cloud watch logs
	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err)
	// there should only be one event as we are echoing only one thing that part of the include-filter
	assert.Equal(t, 1, len(resp.Events))

	message := aws.StringValue(resp.Events[0].Message)
	jsonBlob := make(map[string]string)
	err = json.Unmarshal([]byte(message), &jsonBlob)
	require.NoError(t, err)
	assert.Equal(t, "stdout", jsonBlob["source"])
	assert.Equal(t, "include", jsonBlob["log"])
	assert.Contains(t, jsonBlob, "container_id")
	assert.Contains(t, jsonBlob["container_name"], "logsender")
	assert.Equal(t, testCluster, jsonBlob["ecs_cluster"])
	assert.Equal(t, testTask.Arn, jsonBlob["ecs_task_arn"])

	testTask.SetSentStatus(apitaskstatus.TaskStopped)
	time.Sleep(3 * cfg.TaskCleanupWaitDuration)

	for i := 0; i < 60; i++ {
		_, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(testTask.Arn)
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	// Make sure all the resource is cleaned up
	_, err = ioutil.ReadDir(filepath.Join(testDataDir, "firelens", testTask.Arn))
	assert.Error(t, err)
}

func createFirelensTask(t *testing.T) *apitask.Task {
	testTask := createTestTask(validTaskArnPrefix + uuid.New())
	rawHostConfigInputForLogSender := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
			Type: logDriverTypeFirelens,
			Config: map[string]string{
				"Name":              "cloudwatch",
				"exclude-pattern":   "exclude",
				"include-pattern":   "include",
				"log_group_name":    testLogGroupName,
				"log_stream_prefix": testLogGroupPrefix,
				"region":            testECSRegion,
				"auto_create_group": "true",
			},
		},
	}
	rawHostConfigForLogSender, err := json.Marshal(&rawHostConfigInputForLogSender)
	require.NoError(t, err)
	testTask.Containers = []*apicontainer.Container{
		{
			Name:      "logsender",
			Image:     testLogSenderImage,
			Essential: true,
			// TODO: the firelens router occasionally failed to send logs when it's shut down very quickly after started.
			// Let the task run for a while with a sleep helps avoid that failure, but still needs to figure out the
			// root cause.
			Command: []string{"sh", "-c", "echo exclude; echo include; sleep 10;"},
			DockerConfig: apicontainer.DockerConfig{
				HostConfig: func() *string {
					s := string(rawHostConfigForLogSender)
					return &s
				}(),
			},
			DependsOnUnsafe: []apicontainer.DependsOn{
				{
					ContainerName: "firelens",
					Condition:     "START",
				},
			},
		},
		{
			Name:      "firelens",
			Image:     testFluentbitImage,
			Essential: true,
			FirelensConfig: &apicontainer.FirelensConfig{
				Type: firelens.FirelensConfigTypeFluentbit,
				Options: map[string]string{
					"enable-ecs-log-metadata": "true",
				},
			},
			Environment: map[string]string{
				"AWS_EXECUTION_ENV": "AWS_ECS_EC2",
				"FLB_LOG_LEVEL":     "debug",
			},
			TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
		},
	}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	return testTask
}

func waitCloudwatchLogs(client *cloudwatchlogs.CloudWatchLogs, params *cloudwatchlogs.GetLogEventsInput) (*cloudwatchlogs.GetLogEventsOutput, error) {
	// The test could fail for timing issue, so retry for 30 seconds to make this test more stable
	for i := 0; i < 30; i++ {
		resp, err := client.GetLogEvents(params)
		if err != nil {
			awsError, ok := err.(awserr.Error)
			if !ok || awsError.Code() != "ResourceNotFoundException" {
				return nil, err
			}
		} else if len(resp.Events) > 0 {
			return resp, nil
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("timeout waiting for the logs to be sent to cloud watch logs")
}
