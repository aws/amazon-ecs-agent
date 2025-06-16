//go:build linux && sudo
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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	engineserviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	cgroup "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	sdkClient "github.com/docker/docker/client"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	endpoint = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)
)

const (
	testVolumeImage    = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
	testFluentBitImage = "public.ecr.aws/aws-observability/aws-for-fluent-bit:stable"
	// the fluentd image release is outside our direct control, hence we use a specific version to not get impacted by
	// a bad rollout.
	testFluentdImage             = "public.ecr.aws/docker/library/fluentd:v1.18-1"
	testDataDir                  = "/var/lib/ecs/data/"
	testDataDirOnHost            = "/var/lib/ecs/"
	testExecCommandAgentImage    = "127.0.0.1:51670/amazon/amazon-ecs-exec-command-agent-test:latest"
	testExecCommandAgentSleepBin = "/sleep"
	testExecCommandAgentKillBin  = "/kill"
)

func TestStartStopWithCgroup(t *testing.T) {
	cfg := DefaultTestConfigIntegTest()
	cfg.TaskCleanupWaitDuration = 1 * time.Second
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyEnabled
	cfg.CgroupPath = "/cgroup"

	taskEngine, done, _, _ := SetupIntegTestTaskEngine(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "arn:aws:ecs:us-east-1:123456789012:task/testCgroup"
	testTask := CreateTestTask(taskArn)
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

	VerifyContainerManifestPulledStateChange(t, taskEngine)
	VerifyTaskManifestPulledStateChange(t, taskEngine)

	VerifyContainerRunningStateChange(t, taskEngine)
	VerifyTaskIsRunning(stateChangeEvents, testTask)

	VerifyContainerStoppedStateChange(t, taskEngine)
	VerifyTaskIsStopped(stateChangeEvents, testTask)

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
	cfg := DefaultTestConfigIntegTest()
	taskEngine, done, _, _ := SetupIntegTestTaskEngine(cfg, nil, t)
	defer done()

	// creates a task with local volume
	testTask := createTestLocalVolumeMountTask()
	stateChangeEvents := taskEngine.StateChangeEvents()
	go taskEngine.AddTask(testTask)

	VerifyContainerManifestPulledStateChange(t, taskEngine)
	VerifyTaskManifestPulledStateChange(t, taskEngine)
	VerifyContainerRunningStateChange(t, taskEngine)
	VerifyTaskIsRunning(stateChangeEvents, testTask)
	VerifyContainerStoppedStateChange(t, taskEngine)
	VerifyTaskIsStopped(stateChangeEvents, testTask)

	assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
	assert.Equal(t, 0, *testTask.Containers[0].GetKnownExitCode(), "Wrong exit code")
	data, err := ioutil.ReadFile(filepath.Join("/var/lib/docker/volumes/", testTask.Volumes[0].Volume.Source(), "/_data", "hello-from-container"))
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, "empty-data-volume", strings.TrimSpace(string(data)), "Incorrect file contents")
}

func createTestLocalVolumeMountTask() *apitask.Task {
	testTask := CreateTestTask("testLocalHostVolumeMount")
	testTask.Volumes = []apitask.TaskVolume{{Name: "test-tmp", Volume: &taskresourcevolume.LocalDockerVolume{}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{{ContainerPath: "/host/tmp", SourceVolume: "test-tmp"}}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.Containers[0].Command = []string{`echo -n "empty-data-volume" > /host/tmp/hello-from-container;`}
	return testTask
}

// TestFirelens integration test verifies the following:
//
// 1. Firelens data directory creation and ownership configuration
// 2. Task and container state events
// 3. Firelens container's ability to log to a destination (local file)
// 4. Firelens resource cleanup after the task stops
// 5. Verifies all the above across both Fluentd and Fluent Bit log router options.
//
// This test validates the integration of different ECS agent components. It mocks the payload received from ECS backend,
// and does not rely on external logging destinations (like CloudWatch) for logging functionality verification.
func TestFirelens(t *testing.T) {
	// Setup agent config
	cfg := DefaultTestConfigIntegTest()
	cfg.DataDir = testDataDir
	cfg.DataDirOnHost = testDataDirOnHost
	cfg.TaskCleanupWaitDuration = 1 * time.Second
	cfg.Cluster = TestCluster

	// Setup task engine
	taskEngine, done, _, _ := SetupIntegTestTaskEngine(cfg, nil, t)
	taskEngine.(*DockerTaskEngine).resourceFields = &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			EC2InstanceID: TestInstanceID,
		},
	}
	defer done()

	// Setup task metadata server
	setupTaskMetadataServer(t)

	testCases := []struct {
		name                   string
		firelensType           string
		firelensContainerImage string
		firelensContainerUser  string
	}{
		{
			name:                   "fluent_bit_default",
			firelensType:           "fluentbit",
			firelensContainerImage: testFluentBitImage,
		},
		{
			name:                   "fluent_bit_root_user",
			firelensType:           "fluentbit",
			firelensContainerImage: testFluentBitImage,
			firelensContainerUser:  "0:1000",
		},
		{
			name:                   "fluent_bit_non_root_user",
			firelensType:           "fluentbit",
			firelensContainerImage: testFluentBitImage,
			firelensContainerUser:  "1234:5678",
		},
		{
			name:                   "fluent_d_root_user",
			firelensType:           "fluentd",
			firelensContainerImage: testFluentdImage,
			firelensContainerUser:  "0:1000",
		},
		{
			name:                   "fluent_d_non_root_user",
			firelensType:           "fluentd",
			firelensContainerImage: testFluentdImage,
			firelensContainerUser:  "1234:5678",
		},
		// No fluentd default config test case since all the available fluentd images hard-code a container user in the image.
		// We will rely on functional test for this test case, where we build our own custom fluentd log router image.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory to send app logs to
			tmpDir := createFirelensLogDir(t, "firelens", tc.firelensContainerUser)
			defer func() {
				err := os.RemoveAll(tmpDir)
				require.NoError(t, err)
			}()

			// Create the Firelens task
			firelensTask := createFirelensTask(t, tc.name, tc.firelensContainerImage, tc.firelensContainerUser,
				tc.firelensType, tmpDir)

			// Start the Firelens task
			go taskEngine.AddTask(firelensTask)

			// Validate the Firelens task/container statuses during task start
			verifyFirelensTaskEvents(t, taskEngine, firelensTask, "start")

			// Verify Firelens data directory configuration
			firelensDataDir := filepath.Join(testDataDir, "firelens", firelensTask.GetID())
			verifyFirelensDataDir(t, firelensDataDir, tc.firelensContainerUser)

			// Verify app logs from the destination file
			verifyFirelensLogs(t, firelensTask, tc.firelensType, tmpDir)

			// Validate the Firelens task/container statuses during task stop
			verifyFirelensTaskEvents(t, taskEngine, firelensTask, "stop")

			// Verify Firelens task resource cleanup
			verifyFirelensResourceCleanup(t, cfg, firelensTask, taskEngine, firelensDataDir)
		})
	}
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

	testExecCmdHostBinDir := "/managed-agents/execute-command/bin"

	taskEngine, done, _ := setupEngineForExecCommandAgent(t, testExecCmdHostBinDir)
	stateChangeEvents := taskEngine.StateChangeEvents()
	defer done()

	testTask := createTestExecCommandAgentTask(testTaskId, testContainerName, sleepFor)
	execAgentLogPath := filepath.Join("/log/exec", testTaskId)
	err = os.MkdirAll(execAgentLogPath, 0644)
	require.NoError(t, err, "error creating execAgent log file")
	_, err = os.Stat(execAgentLogPath)
	require.NoError(t, err, "execAgent log dir doesn't exist")
	err = os.MkdirAll(execcmd.ECSAgentExecConfigDir, 0644)
	require.NoError(t, err, "error creating execAgent config dir")

	go taskEngine.AddTask(testTask)

	VerifyContainerManifestPulledStateChange(t, taskEngine)
	VerifyTaskManifestPulledStateChange(t, taskEngine)
	VerifyContainerRunningStateChange(t, taskEngine)
	VerifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	// session limit is 2
	testConfigFileName, _ := execcmd.GetExecAgentConfigFileName(2)
	testLogConfigFileName, _ := execcmd.GetExecAgentLogConfigFile()
	verifyExecCmdAgentExpectedMounts(t, ctx, client, testTaskId, cid, testContainerName, testExecCmdHostBinDir+"/1.0.0.0", testConfigFileName, testLogConfigFileName)
	pidA := verifyMockExecCommandAgentIsRunning(t, client, cid)
	seelog.Infof("Verified mock ExecCommandAgent is running (pidA=%s)", pidA)
	killMockExecCommandAgent(t, client, cid, pidA)
	seelog.Infof("kill signal sent to ExecCommandAgent (pidA=%s)", pidA)
	waitForKillProcToFinish(t, client, cid)
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
		VerifyTaskIsStopped(stateChangeEvents, testTask)
		cancel()
	}()

	<-ctx.Done()
	require.NotEqual(t, context.DeadlineExceeded, ctx.Err(), "Timed out waiting for task (%s) to stop", testTaskId)
	assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
	// TODO: [ecs-exec] We should be able to wait for cleanup instead of calling deleteTask directly
	taskEngine.(*DockerTaskEngine).deleteTask(testTask)
	_, err = os.Stat(execAgentLogPath)
	assert.True(t, os.IsNotExist(err), "execAgent log cleanup failed")
	os.RemoveAll(execcmd.ECSAgentExecConfigDir)
}

// TestManagedAgentEvent validates the emitted container events for a started and a stopped managed agent.
func TestManagedAgentEvent(t *testing.T) {
	testcases := []struct {
		Name                 string
		ExpectedStatus       apicontainerstatus.ManagedAgentStatus
		ManagedAgentLifetime time.Duration
		ShouldBeRunning      bool
	}{
		{
			Name:                 "Confirmed emit RUNNING event",
			ExpectedStatus:       apicontainerstatus.ManagedAgentRunning,
			ManagedAgentLifetime: 1,
			ShouldBeRunning:      true,
		},
		{
			Name:                 "Confirmed emit STOPPED event",
			ExpectedStatus:       apicontainerstatus.ManagedAgentStopped,
			ManagedAgentLifetime: 0,
			ShouldBeRunning:      false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {

			const (
				testTaskId        = "exec-command-agent-test-task"
				testContainerName = "exec-command-agent-test-container"
			)

			client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
			require.NoError(t, err, "Creating go docker client failed")

			testExecCmdHostBinDir := "/managed-agents/execute-command/bin"

			taskEngine, done, _ := setupEngineForExecCommandAgent(t, testExecCmdHostBinDir)
			defer done()

			testTask := createTestExecCommandAgentTask(testTaskId, testContainerName, time.Minute*tc.ManagedAgentLifetime)
			execAgentLogPath := filepath.Join("/log/exec", testTaskId)
			err = os.MkdirAll(execAgentLogPath, 0644)
			require.NoError(t, err, "error creating execAgent log file")
			_, err = os.Stat(execAgentLogPath)
			require.NoError(t, err, "execAgent log dir doesn't exist")
			err = os.MkdirAll(execcmd.ECSAgentExecConfigDir, 0644)
			require.NoError(t, err, "error creating execAgent config dir")

			go taskEngine.AddTask(testTask)

			VerifyContainerManifestPulledStateChange(t, taskEngine)
			VerifyTaskManifestPulledStateChange(t, taskEngine)
			VerifyContainerRunningStateChange(t, taskEngine)
			VerifyTaskRunningStateChange(t, taskEngine)

			if tc.ShouldBeRunning {
				containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
				cid := containerMap[testTask.Containers[0].Name].DockerID
				verifyMockExecCommandAgentIsRunning(t, client, cid)
			}
			waitDone := make(chan struct{})

			go verifyExecAgentStateChange(t, taskEngine, tc.ExpectedStatus, waitDone)

			timeout := false
			select {
			case <-waitDone:
			case <-time.After(20 * time.Second):
				timeout = true
			}
			assert.False(t, timeout)

			taskEngine.(*DockerTaskEngine).deleteTask(testTask)
			_, err = os.Stat(execAgentLogPath)
			assert.True(t, os.IsNotExist(err), "execAgent log cleanup failed")
			os.RemoveAll(execcmd.ECSAgentExecConfigDir)
		})
	}
}

func createTestExecCommandAgentTask(taskId, containerName string, sleepFor time.Duration) *apitask.Task {
	testTask := CreateTestTask("arn:aws:ecs:us-west-2:1234567890:task/" + taskId)
	testTask.PIDMode = string(ecstypes.PidModeHost)
	testTask.Containers[0].Name = containerName
	testTask.Containers[0].Image = testExecCommandAgentImage
	testTask.Containers[0].Command = []string{testExecCommandAgentSleepBin, "-time=" + sleepFor.String()}
	enableExecCommandAgentForContainer(testTask.Containers[0], apicontainer.ManagedAgentState{})
	return testTask
}

// setupEngineForExecCommandAgent creates a new TaskEngine with a custom execcmd.Manager that will attempt to read the
// host binaries from the directory passed as parameter (as opposed to the default directory).
// Additionally, it overrides the engine's monitorExecAgentsInterval to one second.
func setupEngineForExecCommandAgent(t *testing.T, hostBinDir string) (TaskEngine, func(), credentials.Manager) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	skipIntegTestIfApplicable(t)

	cfg := DefaultTestConfigIntegTest()
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
	hostResources := getTestHostResources()
	hostResourceManager := NewHostResourceManager(hostResources)
	daemonManagers := getTestDaemonManagers()

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, &hostResourceManager, state, metadataManager,
		nil, execCmdMgr, engineserviceconnect.NewManager(), daemonManagers)
	taskEngine.monitorExecAgentsInterval = time.Second
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

const (
	uuidRegex                = "[[:alnum:]]{8}-[[:alnum:]]{4}-[[:alnum:]]{4}-[[:alnum:]]{4}-[[:alnum:]]{12}" // matches a UUID
	containerDepsPrefixRegex = execcmd.ContainerDepsDirPrefix + uuidRegex
)

func verifyExecCmdAgentExpectedMounts(t *testing.T,
	ctx context.Context,
	client *sdkClient.Client,
	testTaskId, containerId, containerName, testExecCmdHostVersionedBinDir, testConfigFileName, testLogConfigFileName string) {
	inspectState, err := client.ContainerInspect(ctx, containerId)
	require.NoError(t, err)

	expectedMounts := []struct {
		source    string
		destRegex string
		readOnly  bool
	}{
		{
			source:    filepath.Join(testExecCmdHostVersionedBinDir, execcmd.SSMAgentBinName),
			destRegex: filepath.Join(containerDepsPrefixRegex, execcmd.SSMAgentBinName),
			readOnly:  true,
		},
		{
			source:    filepath.Join(testExecCmdHostVersionedBinDir, execcmd.SSMAgentWorkerBinName),
			destRegex: filepath.Join(containerDepsPrefixRegex, execcmd.SSMAgentWorkerBinName),
			readOnly:  true,
		},
		{
			source:    filepath.Join(testExecCmdHostVersionedBinDir, execcmd.SessionWorkerBinName),
			destRegex: filepath.Join(containerDepsPrefixRegex, execcmd.SessionWorkerBinName),
			readOnly:  true,
		},
		{
			source:    execcmd.HostCertFile,
			destRegex: filepath.Join(containerDepsPrefixRegex, execcmd.ContainerCertFileSuffix),
			readOnly:  true,
		},
		{
			source:    filepath.Join(execcmd.HostExecConfigDir, testConfigFileName),
			destRegex: filepath.Join(containerDepsPrefixRegex, execcmd.ContainerConfigFileSuffix),
			readOnly:  true,
		},
		{
			source:    filepath.Join(execcmd.HostExecConfigDir, testLogConfigFileName),
			destRegex: filepath.Join(containerDepsPrefixRegex, execcmd.ContainerLogConfigFile),
			readOnly:  true,
		},
		{
			source:    filepath.Join(execcmd.HostLogDir, testTaskId, containerName),
			destRegex: execcmd.ContainerLogDir,
			readOnly:  false,
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
		require.Regexp(t, em.destRegex, found.Destination, "Destination for mount point (%s) is invalid expected: %s, actual: %s", em.source, em.destRegex, found.Destination)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	res := make(chan string, 1)
	execCmdAgentProcessRegex := filepath.Join(containerDepsPrefixRegex, execcmd.SSMAgentBinName)
	go func() {
		for {
			pid, _, err := findContainerProcess(client, containerId, execCmdAgentProcessRegex)
			if err != nil {
				seelog.Errorf("Error when finding container process %s in container %s: %v",
					execCmdAgentProcessRegex, containerId, err)
				continue
			}
			if pid != "" {
				res <- pid
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 4):
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
	require.Equal(t, checkIsRunning, isRunning, "SSM agent was not found in container's process list")
	return pid
}

// Waits for /kill process to finish in the container.
func waitForKillProcToFinish(t *testing.T, client *sdkClient.Client, containerId string) {
	for i := 0; i < 10; i++ {
		seelog.Infof("Checking if kill process is running in container %s", containerId)
		pid, _, err := findContainerProcess(client, containerId, testExecCommandAgentKillBin)
		require.NoError(t, err, "error when finding kill process in container")
		if pid == "" {
			seelog.Info("Kill process is not running in the container")
			return
		}
		seelog.Infof("Kill process is running with pid %s in container %s", pid, containerId)
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("Timed out waiting for kill process to finish in container %s", containerId)
}

// Finds a process whose start command matches the provided regex in the container.
// Returns the process's pid and command.
func findContainerProcess(client *sdkClient.Client, containerId, matching string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	top, err := client.ContainerTop(ctx, containerId, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to run container top: %w", err)
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
	if cmdPos == -1 {
		return "", "", fmt.Errorf("CMD title not found in the container top response")
	}
	if pidPos == -1 {
		return "", "", fmt.Errorf("PID title not found in the container top response")
	}
	seelog.Infof("Processes running in container %s: %s", containerId, top.Processes)
	for _, proc := range top.Processes {
		matched, _ := regexp.MatchString(matching, proc[cmdPos])
		if matched {
			return proc[pidPos], proc[cmdPos], nil
		}
	}
	return "", "", nil
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

func TestGMSATaskFile(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")
	t.Setenv("ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	cfg := DefaultTestConfigIntegTest()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.TaskCleanupWaitDuration = 3 * time.Second
	cfg.GMSACapable = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.AWSRegion = "us-west-2"

	taskEngine, done, _ := setupGMSALinux(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	// Setup test gmsa file
	credentialSpecDataDir := "/tmp"
	testFileName := "test-gmsa.json"
	testCredSpecFilePath := filepath.Join(credentialSpecDataDir, testFileName)
	_, err := os.Create(testCredSpecFilePath)
	require.NoError(t, err)

	// add local credentialspec file
	testCredSpecData := []byte(`{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "gmsa-acct-test",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "gmsa.test.com",
                             "DnsName":  "gmsa.test.com",
                             "NetBiosName":  "gmsa"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "gmsa-acct-test",
                                                                          "Scope":  "gmsa.test.com"
                                                                      }
                                                                  ]
                              }
}`)

	err = ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	require.NoError(t, err)

	defer os.RemoveAll(testCredSpecFilePath)

	testContainer := CreateTestContainer()
	testContainer.Name = "testGMSATaskFile"

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file:///tmp/test-gmsa.json\"]}"
	testContainer.DockerConfig.HostConfig = &hostConfig

	testTask := &apitask.Task{
		Arn:                 "testGMSAFileTaskARN",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{testContainer},
	}
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = GetLongRunningCommand()

	go taskEngine.AddTask(testTask)

	VerifyTaskIsRunning(stateChangeEvents, testTask)

	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	expectedBind := "/tmp/tgt:/var/credentials-fetcher/krbdir:ro"
	err = verifyContainerBindMount(client, cid, expectedBind)
	assert.NoError(t, err)

	// Kill the existing container now
	err = client.ContainerKill(context.TODO(), cid, "SIGKILL")
	assert.NoError(t, err, "Could not kill container")

	VerifyTaskIsStopped(stateChangeEvents, testTask)
}

func TestGMSADomainlessTaskFile(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")
	t.Setenv("ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	cfg := DefaultTestConfigIntegTest()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.TaskCleanupWaitDuration = 3 * time.Second
	cfg.GMSACapable = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.AWSRegion = "us-west-2"

	taskEngine, done, _ := setupGMSALinux(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	// Setup test gmsa file
	credentialSpecDataDir := "/tmp"
	testFileName := "test-gmsa.json"
	testCredSpecFilePath := filepath.Join(credentialSpecDataDir, testFileName)
	_, err := os.Create(testCredSpecFilePath)
	require.NoError(t, err)

	// add local credentialspec file for domainless gmsa support
	testCredSpecData := []byte(`{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "gmsa-acct-test",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "gmsa.test.com",
                             "DnsName":  "gmsa.test.com",
                             "NetBiosName":  "gmsa"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "gmsa-acct-test",
                                                                          "Scope":  "gmsa.test.com"
                                                                      }
                                                                  ],
     "HostAccountConfig": {
      "PortableCcgVersion": "1",
      "PluginGUID": "{859E1386-BDB4-49E8-85C7-3070B13920E1}",
      "PluginInput": {
        "CredentialArn": "arn:aws:secretsmanager:us-west-2:123456789:secret:gmsausersecret-xb5Qev"
      }
    }
                              }
}`)

	err = ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	require.NoError(t, err)

	defer os.RemoveAll(testCredSpecFilePath)

	testContainer := CreateTestContainer()
	testContainer.Name = "testGMSADomainlessTaskFile"

	testContainer.CredentialSpecs = []string{"credentialspecdomainless:file:///tmp/test-gmsa.json"}

	testTask := &apitask.Task{
		Arn:                 "testGMSAFileTaskARN",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{testContainer},
	}
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = GetLongRunningCommand()

	go taskEngine.AddTask(testTask)

	VerifyTaskIsRunning(stateChangeEvents, testTask)

	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	expectedBind := "/tmp/tgt:/var/credentials-fetcher/krbdir:ro"
	err = verifyContainerBindMount(client, cid, expectedBind)
	assert.NoError(t, err)

	// Kill the existing container now
	err = client.ContainerKill(context.TODO(), cid, "SIGKILL")
	assert.NoError(t, err, "Could not kill container")

	VerifyTaskIsStopped(stateChangeEvents, testTask)
}

func TestGMSATaskFileS3Err(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")
	t.Setenv("ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	cfg := DefaultTestConfigIntegTest()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.TaskCleanupWaitDuration = 3 * time.Second
	cfg.GMSACapable = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.AWSRegion = "us-west-2"

	taskEngine, done, _ := setupGMSALinux(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testContainer := CreateTestContainer()
	testContainer.Name = "testGMSATaskFile"

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:arn:aws:::s3:testbucket/test-gmsa.json\"]}"
	testContainer.DockerConfig.HostConfig = &hostConfig

	testTask := &apitask.Task{
		Arn:                 "testGMSAFileTaskARN",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{testContainer},
	}
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = GetLongRunningCommand()

	go taskEngine.AddTask(testTask)

	err := VerifyTaskIsRunning(stateChangeEvents, testTask)
	assert.Error(t, err)
	assert.Error(t, err, "Task went straight to STOPPED without running, task: testGMSAFileTaskARN")
}

func TestGMSATaskFileSSMErr(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")
	t.Setenv("ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")

	cfg := DefaultTestConfigIntegTest()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.TaskCleanupWaitDuration = 3 * time.Second
	cfg.GMSACapable = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.AWSRegion = "us-west-2"

	taskEngine, done, _ := setupGMSALinux(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testContainer := CreateTestContainer()
	testContainer.Name = "testGMSATaskFile"

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:aws:arn:ssm:us-west-2:123456789012:document/test-gmsa.json\"]}"
	testContainer.DockerConfig.HostConfig = &hostConfig

	testTask := &apitask.Task{
		Arn:                 "testGMSAFileTaskARN",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{testContainer},
	}
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = GetLongRunningCommand()

	go taskEngine.AddTask(testTask)

	err := VerifyTaskIsRunning(stateChangeEvents, testTask)
	assert.Error(t, err)
	assert.Error(t, err, "Task went straight to STOPPED without running, task: testGMSAFileTaskARN")
}

func TestGMSANotRunningErr(t *testing.T) {
	t.Setenv("ECS_GMSA_SUPPORTED", "True")
	t.Setenv("ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "True")
	t.Setenv("ZZZ_SKIP_CREDENTIALS_FETCHER_INVOCATION_CHECK_NOT_SUPPORTED_IN_PRODUCTION", "False")

	cfg := DefaultTestConfigIntegTest()
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.TaskCleanupWaitDuration = 3 * time.Second
	cfg.GMSACapable = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.AWSRegion = "us-west-2"

	taskEngine, done, _ := setupGMSALinux(cfg, nil, t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	// Setup test gmsa file
	credentialSpecDataDir := "/tmp"
	testFileName := "test-gmsa.json"
	testCredSpecFilePath := filepath.Join(credentialSpecDataDir, testFileName)
	_, err := os.Create(testCredSpecFilePath)
	require.NoError(t, err)

	// add local credentialspec file
	testCredSpecData := []byte(`{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "gmsa-acct-test",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "gmsa.test.com",
                             "DnsName":  "gmsa.test.com",
                             "NetBiosName":  "gmsa"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "gmsa-acct-test",
                                                                          "Scope":  "gmsa.test.com"
                                                                      }
                                                                  ]
                              }
}`)

	err = ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	require.NoError(t, err)

	testContainer := CreateTestContainer()
	testContainer.Name = "testGMSATaskFile"

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file:///tmp/test-gmsa.json\"]}"
	testContainer.DockerConfig.HostConfig = &hostConfig

	testTask := &apitask.Task{
		Arn:                 "testGMSAFileTaskARN",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{testContainer},
	}
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = GetLongRunningCommand()

	go taskEngine.AddTask(testTask)

	err = VerifyTaskIsRunning(stateChangeEvents, testTask)
	assert.Error(t, err)
	assert.Error(t, err, "Task went straight to STOPPED without running, task: testGMSAFileTaskARN")
}

func verifyContainerBindMount(client *sdkClient.Client, id, expectedBind string) error {
	dockerContainer, err := client.ContainerInspect(context.TODO(), id)
	if err != nil {
		return err
	}

	for _, opt := range dockerContainer.HostConfig.Binds {
		if opt == expectedBind {
			return nil
		}
	}

	return errors.New("unable to validate the bind mount")
}
