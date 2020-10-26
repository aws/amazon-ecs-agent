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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	cgroup "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/aws/amazon-ecs-agent/agent/utils/retry"
	"github.com/cihub/seelog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	sdkClient "github.com/docker/docker/client"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	endpoint = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)
)

const (
	testLogSenderImage           = "amazonlinux:2"
	testFluentbitImage           = "amazon/aws-for-fluent-bit:latest"
	testVolumeImage              = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
	testCluster                  = "testCluster"
	validTaskArnPrefix           = "arn:aws:ecs:region:account-id:task/"
	testDataDir                  = "/var/lib/ecs/data/"
	testDataDirOnHost            = "/var/lib/ecs/"
	testInstanceID               = "testInstanceID"
	testTaskDefFamily            = "testFamily"
	testTaskDefVersion           = "1"
	testECSRegion                = "us-east-1"
	testLogGroupName             = "test-fluentbit"
	testLogGroupPrefix           = "firelens-fluentbit-"
	testExecCommandAgentImage    = "127.0.0.1:51670/amazon/amazon-ecs-exec-command-agent-test:latest"
	testExecCommandAgentSleepBin = "/sleep"
	testExecCommandAgentKillBin  = "/kill"
)

func TestStartStopWithCgroup(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyEnabled
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

// TestExecCommandAgent validates ExecCommandAgent start and monitor processes. The algorithm to test is as follows:
// 1. Pre-setup: the make file in ../../misc/exec-command-agent-test will create a special docker sleeper image
// based on a scratch image. This image simulates a customer image and contains pre-baked /sleep and /kill binaries.
// /sleep is the main process used to launch the test container; /kill is an application that kills a process running in
// the container given a PID.
// The make file will also create a fake amazon-ssm-agent which is a go program that only sleeps for a certain time specified.
//
// 2. Setup: Create a new docker task engine with a modified path pointing to our fake amazon-ssm-agent binary
// 3. Create and start our test task using our test image
// 4. Wait for the task to start and verify that the expected ExecCommandAgent bind mounts are present in the containers
// 5. Verify that our fake amazon-ssm-agent was started inside the container using docker top, and retrieve its PID
// 6. Kill the fake amazon-ssm-agent using the PID retrieved in previous step
// 7. Verify that the engine restarted our fake amazon-ssm-agent by doing docker top one more time (a new PID should popup)
func TestExecCommandAgent(t *testing.T) {
	const (
		testTaskId        = "exec-command-agent-test-task"
		testContainerName = "exec-command-agent-test-container"
		sleepFor          = time.Minute * 2
	)

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testExecCmdHostBinDir, err := filepath.Abs("../../misc/exec-command-agent-test")
	require.NoError(t, err)

	taskEngine, done, _ := setupEngineForExecCommandAgent(t, testExecCmdHostBinDir)
	stateChangeEvents := taskEngine.StateChangeEvents()
	defer done()

	testTask := createTestExecCommandAgentTask(testTaskId, testContainerName, sleepFor)
	execAgentLogPath := filepath.Join("/log/exec", testTaskId)
	err = os.MkdirAll(execAgentLogPath, 0644)
	require.NoError(t, err, "error creating execAgent log file")
	_, err = os.Stat(execAgentLogPath)
	require.NoError(t, err, "execAgent log dir doesn't exist")
	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	verifyExecCmdAgentExpectedMounts(t, ctx, client, testTaskId, cid, testContainerName, testExecCmdHostBinDir)
	pidA := verifyMockExecCommandAgentIsRunning(t, client, cid)
	verifyExecAgentRunningStateChange(t, taskEngine)
	seelog.Infof("Verified mock ExecCommandAgent is running (pidA=%s)", pidA)
	killMockExecCommandAgent(t, client, cid, pidA)
	seelog.Infof("kill signal sent to ExecCommandAgent (pidA=%s)", pidA)
	verifyMockExecCommandAgentIsStopped(t, client, cid, pidA)
	seelog.Infof("Verified mock ExecCommandAgent was killed (pidA=%s)", pidA)
	pidB := verifyMockExecCommandAgentIsRunning(t, client, cid)
	seelog.Infof("Verified mock ExecCommandAgent was restarted (pidB=%s)", pidB)
	require.NotEqual(t, pidA, pidB, "ExecCommandAgent PID did not change after restart")

	taskUpdate := createTestExecCommandAgentTask(testTaskId, testContainerName, sleepFor)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*20)
	go func() {
		verifyTaskIsStopped(stateChangeEvents, testTask)
		cancel()
	}()

	<-ctx.Done()
	require.NotEqual(t, context.DeadlineExceeded, ctx.Err(), "Timed out waiting for task (%s) to stop", testTaskId)
	assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
	taskEngine.(*DockerTaskEngine).deleteTask(testTask)
	_, err = os.Stat(execAgentLogPath)
	assert.True(t, os.IsNotExist(err), "execAgent log cleanup failed")
}

func createTestExecCommandAgentTask(taskId, containerName string, sleepFor time.Duration) *apitask.Task {
	testTask := createTestTask("arn:aws:ecs:us-west-2:1234567890:task/" + taskId)
	testTask.ExecCommandAgentEnabledUnsafe = true
	testTask.PIDMode = ecs.PidModeHost
	testTask.Containers[0].Name = containerName
	testTask.Containers[0].Image = testExecCommandAgentImage
	testTask.Containers[0].Command = []string{testExecCommandAgentSleepBin, "-time=" + sleepFor.String()}
	return testTask
}

// setupEngineForExecCommandAgent creates a new TaskEngine with a custom execcmd.Manager that will attempt to read the
// host binaries from the directory passed as parameter (as opposed to the default directory).
// Additionally, it overrides the engine's monitorExecAgentsInterval to one second.
func setupEngineForExecCommandAgent(t *testing.T, hostBinDir string) (TaskEngine, func(), credentials.Manager) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	skipIntegTestIfApplicable(t)

	cfg := defaultTestConfigIntegTest()
	sdkClientFactory := sdkclientfactory.NewFactory(ctx, dockerEndpoint)
	dockerClient, err := dockerapi.NewDockerGoClient(sdkClientFactory, cfg, context.Background())
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	state := dockerstate.NewTaskEngineState()
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetDataClient(data.NewNoopClient())
	metadataManager := containermetadata.NewManager(dockerClient, cfg)
	execCmdMgr := execcmd.NewManagerWithBinDir(hostBinDir)

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, state, metadataManager,
		nil, execCmdMgr)
	taskEngine.monitorExecAgentsInterval = time.Second
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

func verifyExecCmdAgentExpectedMounts(t *testing.T, ctx context.Context, client *sdkClient.Client, testTaskId, containerId, containerName, testExecCmdHostBinDir string) {
	inspectState, _ := client.ContainerInspect(ctx, containerId)
	expectedMounts := []struct {
		source   string
		dest     string
		readOnly bool
	}{
		{
			source:   filepath.Join(testExecCmdHostBinDir, execcmd.BinName),
			dest:     filepath.Join(execcmd.ContainerBinDir, execcmd.BinName),
			readOnly: true,
		},
		{
			source:   filepath.Join(testExecCmdHostBinDir, execcmd.SessionWorkerBinName),
			dest:     filepath.Join(execcmd.ContainerBinDir, execcmd.SessionWorkerBinName),
			readOnly: true,
		},
		{
			source:   execcmd.HostCertFile,
			dest:     execcmd.ContainerCertFile,
			readOnly: true,
		},
		{
			source:   filepath.Join(testExecCmdHostBinDir, execcmd.ConfigFileName),
			dest:     execcmd.ContainerConfigFile,
			readOnly: true,
		},
		{
			source:   filepath.Join(execcmd.HostLogDir, testTaskId, containerName),
			dest:     execcmd.ContainerLogDir,
			readOnly: false,
		},
	}

	for _, em := range expectedMounts {
		var found *types.MountPoint
		for _, m := range inspectState.Mounts {
			if m.Source == em.source {
				found = &m
				break
			}
		}
		require.NotNil(t, found, "Expected mount point not found (%s)", em.source)
		require.Equal(t, em.dest, found.Destination, "Destination for mount point (%s) is invalid expected: %s, actual: %s", em.source, em.dest, found.Destination)
		if em.readOnly {
			require.Equal(t, "ro", found.Mode, "Destination for mount point (%s) should be read only", em.source)
		} else {
			require.True(t, found.RW, "Destination for mount point (%s) should be writable", em.source)
		}
		require.Equal(t, "bind", string(found.Type), "Destination for mount point (%s) is not of type bind", em.source)
	}

	require.Equal(t, len(expectedMounts), len(inspectState.Mounts), "Wrong number of bind mounts detected in container (%s)", containerName)
}

func verifyMockExecCommandAgentIsRunning(t *testing.T, client *sdkClient.Client, containerId string) string {
	return verifyMockExecCommandAgentStatus(t, client, containerId, "", true)
}

func verifyMockExecCommandAgentIsStopped(t *testing.T, client *sdkClient.Client, containerId, pid string) {
	verifyMockExecCommandAgentStatus(t, client, containerId, pid, false)
}

func verifyMockExecCommandAgentStatus(t *testing.T, client *sdkClient.Client, containerId, expectedPid string, checkIsRunning bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	res := make(chan string, 1)
	go func() {
		for {
			top, err := client.ContainerTop(ctx, containerId, nil)
			if err != nil {
				continue
			}
			cmdPos := -1
			pidPos := -1
			for i, t := range top.Titles {
				if strings.ToUpper(t) == "CMD" {
					cmdPos = i
				}
				if strings.ToUpper(t) == "PID" {
					pidPos = i
				}

			}
			require.NotEqual(t, -1, cmdPos, "CMD title not found in the container top response")
			require.NotEqual(t, -1, pidPos, "PID title not found in the container top response")
			for _, proc := range top.Processes {
				if proc[cmdPos] == filepath.Join(execcmd.ContainerBinDir, execcmd.BinName) {
					res <- proc[pidPos]
					return
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(retry.AddJitter(time.Second, time.Second*5)):
			}
		}
	}()

	var (
		isRunning bool
		pid       string
	)
	select {
	case <-ctx.Done():
	case r := <-res:
		if r != "" {
			pid = r
			isRunning = true
			if expectedPid != "" && pid != expectedPid {
				isRunning = false
			}
		}

	}
	require.Equal(t, checkIsRunning, isRunning, "ExecCmdAgent was not in the desired running-status")
	return pid
}

func killMockExecCommandAgent(t *testing.T, client *sdkClient.Client, containerId, pid string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	create, err := client.ContainerExecCreate(ctx, containerId, types.ExecConfig{
		Detach: true,
		Cmd:    []string{testExecCommandAgentKillBin, "-pid=" + pid},
	})
	require.NoError(t, err)

	err = client.ContainerExecStart(ctx, create.ID, types.ExecStartCheck{
		Detach: true,
	})
	require.NoError(t, err)
}

func verifyTaskRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning,
		"Expected task to be RUNNING")
}
