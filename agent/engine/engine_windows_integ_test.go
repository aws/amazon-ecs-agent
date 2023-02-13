//go:build windows && integration
// +build windows,integration

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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	engineserviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dockerEndpoint               = "npipe:////./pipe/docker_engine"
	testVolumeImage              = "amazon/amazon-ecs-volumes-test:make"
	testRegistryImage            = "amazon/amazon-ecs-netkitten:make"
	testExecCommandAgentImage    = "amazon/amazon-ecs-exec-command-agent-windows-test:make"
	testBaseImage                = "amazon-ecs-ftest-windows-base:make"
	dockerVolumeDirectoryFormat  = "c:\\ProgramData\\docker\\volumes\\%s\\_data"
	testExecCommandAgentSleepBin = "c:\\sleep.exe"
)

var endpoint = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), dockerEndpoint)

// TODO implement this
func isDockerRunning() bool { return true }

func createTestContainer() *apicontainer.Container {
	return &apicontainer.Container{
		Name:                "windows",
		Image:               testBaseImage,
		Essential:           true,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		CPU:                 512,
		Memory:              256,
	}
}

// getLongRunningCommand returns the command that keeps the container running for the container
// that uses the default integ test image (amazon-ecs-ftest-windows-base:make for windows)
func getLongRunningCommand() []string {
	return []string{"ping", "-t", "localhost"}
}

func createTestHostVolumeMountTask(tmpPath string) *apitask.Task {
	testTask := createTestTask("testHostVolumeMount")
	testTask.Volumes = []apitask.TaskVolume{{Name: "test-tmp", Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: tmpPath}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{{ContainerPath: "C:/host/tmp", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{
		`echo "hi" | Out-File -FilePath C:\host\tmp\hello-from-container -Encoding ascii ; $exists = Test-Path C:\host\tmp\test-file ; if (!$exists) { exit 2 } ;$contents = [IO.File]::ReadAllText("C:\host\tmp\test-file") ; if (!$contents -match "test-data") { $contents ; exit 4 } ; exit 42`,
	}
	return testTask
}

func createTestLocalVolumeMountTask() *apitask.Task {
	testTask := createTestTask("testLocalHostVolumeMount")
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].Command = []string{`Write-Output "empty-data-volume" | Out-File -FilePath C:\host\tmp\hello-from-container -Encoding ascii`}
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{{ContainerPath: "C:\\host\\tmp", SourceVolume: "test-tmp"}}
	testTask.Volumes = []apitask.TaskVolume{{Name: "test-tmp", Volume: &taskresourcevolume.LocalDockerVolume{}}}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	return testTask
}

func createTestHealthCheckTask(arn string) *apitask.Task {
	testTask := &apitask.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{createTestContainer()},
	}
	testTask.Containers[0].Image = testBaseImage
	testTask.Containers[0].Name = "test-health-check"
	testTask.Containers[0].HealthCheckType = "docker"
	testTask.Containers[0].Command = []string{"powershell", "-command", "Start-Sleep -s 300"}
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{
		Config: aws.String(alwaysHealthyHealthCheckConfig),
	}
	return testTask
}

func createVolumeTaskWindows(scope, arn, volume string, autoprovision bool) (*apitask.Task, string, error) {
	testTask := createTestTask(arn)

	volumeConfig := &taskresourcevolume.DockerVolumeConfig{
		Scope:  scope,
		Driver: "local",
	}
	if scope == "shared" {
		volumeConfig.Autoprovision = autoprovision
	}

	testTask.Volumes = []apitask.TaskVolume{
		{
			Type:   "docker",
			Name:   volume,
			Volume: volumeConfig,
		},
	}

	// Construct the volume path, windows doesn't support create a volume from local directory
	err := os.MkdirAll(fmt.Sprintf(dockerVolumeDirectoryFormat, volume), 0666)
	if err != nil {
		return nil, "", err
	}
	volumePath := filepath.Join(fmt.Sprintf(dockerVolumeDirectoryFormat, volume), "volumecontent")
	err = ioutil.WriteFile(volumePath, []byte("volume"), 0666)
	if err != nil {
		return nil, "", err
	}

	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{
		{
			SourceVolume:  volume,
			ContainerPath: "c:\\ecs",
		},
	}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = []string{"$output = (cat c:\\ecs\\volumecontent); if ( $output -eq \"volume\" ) { Exit 0 } else { Exit 1 }"}
	return testTask, volumePath, nil
}

func TestSharedAutoprovisionVolume(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	stateChangeEvents := taskEngine.StateChangeEvents()
	// Set the task clean up duration to speed up the test
	taskEngine.(*DockerTaskEngine).cfg.TaskCleanupWaitDuration = 1 * time.Second

	testTask, tmpDirectory, err := createVolumeTaskWindows("shared", "TestSharedAutoprovisionVolume", "TestSharedAutoprovisionVolume", true)
	defer os.Remove(tmpDirectory)
	require.NoError(t, err, "creating test task failed")

	go taskEngine.AddTask(testTask)

	verifyTaskIsRunning(stateChangeEvents, testTask)
	verifyTaskIsStopped(stateChangeEvents, testTask)
	assert.Equal(t, *testTask.Containers[0].GetKnownExitCode(), 0)
	assert.Equal(t, testTask.ResourcesMapUnsafe["dockerVolume"][0].(*taskresourcevolume.VolumeResource).VolumeConfig.DockerVolumeName, "TestSharedAutoprovisionVolume", "task volume name is not the same as specified in task definition")
	// Wait for task to be cleaned up
	testTask.SetSentStatus(apitaskstatus.TaskStopped)
	waitForTaskCleanup(t, taskEngine, testTask.Arn, 5)
	client := taskEngine.(*DockerTaskEngine).client
	response := client.InspectVolume(context.TODO(), "TestSharedAutoprovisionVolume", 1*time.Second)
	assert.NoError(t, response.Error, "expect shared volume not removed")

	cleanVolumes(testTask, taskEngine)
}

func TestSharedDoNotAutoprovisionVolume(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	stateChangeEvents := taskEngine.StateChangeEvents()
	client := taskEngine.(*DockerTaskEngine).client
	// Set the task clean up duration to speed up the test
	taskEngine.(*DockerTaskEngine).cfg.TaskCleanupWaitDuration = 1 * time.Second

	testTask, tmpDirectory, err := createVolumeTaskWindows("shared", "TestSharedDoNotAutoprovisionVolume", "TestSharedDoNotAutoprovisionVolume", false)
	defer os.Remove(tmpDirectory)
	require.NoError(t, err, "creating test task failed")

	// creating volume to simulate previously provisioned volume
	volumeConfig := testTask.Volumes[0].Volume.(*taskresourcevolume.DockerVolumeConfig)
	volumeMetadata := client.CreateVolume(context.TODO(), "TestSharedDoNotAutoprovisionVolume",
		volumeConfig.Driver, volumeConfig.DriverOpts, volumeConfig.Labels, 1*time.Minute)
	require.NoError(t, volumeMetadata.Error)

	go taskEngine.AddTask(testTask)

	verifyTaskIsRunning(stateChangeEvents, testTask)
	verifyTaskIsStopped(stateChangeEvents, testTask)
	assert.Equal(t, *testTask.Containers[0].GetKnownExitCode(), 0)
	assert.Len(t, testTask.ResourcesMapUnsafe["dockerVolume"], 0, "volume that has been provisioned does not require the agent to create it again")
	// Wait for task to be cleaned up
	testTask.SetSentStatus(apitaskstatus.TaskStopped)
	waitForTaskCleanup(t, taskEngine, testTask.Arn, 5)
	response := client.InspectVolume(context.TODO(), "TestSharedDoNotAutoprovisionVolume", 1*time.Second)
	assert.NoError(t, response.Error, "expect shared volume not removed")

	cleanVolumes(testTask, taskEngine)
}

// TODO Modify the container ip to localhost after the AMI has the required feature
// https://github.com/docker/for-win/issues/204#issuecomment-352899657

func getContainerIP(client *sdkClient.Client, id string) (string, error) {
	dockerContainer, err := client.ContainerInspect(context.TODO(), id)
	if err != nil {
		return "", err
	}

	networks := dockerContainer.NetworkSettings.Networks
	if len(networks) != 1 {
		return "", fmt.Errorf("getContainerIP: inspect return multiple networks of container")
	}
	for _, v := range networks {
		return v.IPAddress, nil
	}
	return "", nil
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

	data, err := ioutil.ReadFile(filepath.Join("c:\\ProgramData\\docker\\volumes", testTask.Volumes[0].Volume.Source(), "_data", "hello-from-container"))
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, "empty-data-volume", strings.TrimSpace(string(data)), "Incorrect file contents")
}

// TestStartStopUnpulledImage ensures that an unpulled image is successfully
// pulled, run, and stopped via docker.
func TestStartStopUnpulledImage(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	baseImg := os.Getenv("BASE_IMAGE_NAME")
	removeImage(t, baseImg)

	testTask := createTestTask("testStartUnpulled")
	testTask.Containers[0].Image = baseImg

	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning, "Expected task to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped, "Expected container to be STOPPED")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped, "Expected task to be STOPPED")
}

// TestStartStopUnpulledImageDigest ensures that an unpulled image with
// specified digest is successfully pulled, run, and stopped via docker.
func TestStartStopUnpulledImageDigest(t *testing.T) {
	baseImgWithDigest := os.Getenv("BASE_IMAGE_NAME_WITH_DIGEST")
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(t, baseImgWithDigest)

	testTask := createTestTask("testStartUnpulledDigest")
	testTask.Containers[0].Image = baseImgWithDigest

	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning, "Expected task to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped, "Expected container to be STOPPED")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped, "Expected task to be STOPPED")
}

func TestVolumesFrom(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testTask := createTestTask("testVolumeContainer")
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "test2"
	testTask.Containers[1].Image = testVolumeImage
	testTask.Containers[1].VolumesFrom = []apicontainer.VolumeFrom{{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[1].Command = []string{"-c", "$output= (cat /data/test-file); if ($output -eq \"test\") { Exit 42 } else { Exit 1 }"}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)
	verifyTaskIsStopped(stateChangeEvents, testTask)
	assert.Equal(t, *testTask.Containers[1].GetKnownExitCode(), 42)
}

func TestVolumesFromRO(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testTask := createTestTask("testVolumeROContainer")
	testTask.Containers[0].Image = testVolumeImage
	for i := 0; i < 3; i++ {
		cont := createTestContainer()
		cont.Name = "test" + strconv.Itoa(i)
		cont.Image = testVolumeImage
		cont.Essential = i > 0
		testTask.Containers = append(testTask.Containers, cont)
	}
	testTask.Containers[1].VolumesFrom = []apicontainer.VolumeFrom{{SourceContainer: testTask.Containers[0].Name, ReadOnly: true}}
	testTask.Containers[1].Command = []string{"New-Item c:/volume/readonly-fs; if ($?) { Exit 0 } else { Exit 42 }"}
	testTask.Containers[1].Essential = false
	testTask.Containers[2].VolumesFrom = []apicontainer.VolumeFrom{{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[2].Command = []string{"New-Item c:/volume/readonly-fs-2; if ($?) { Exit 0 } else { Exit 42 }"}
	testTask.Containers[2].Essential = false
	testTask.Containers[3].VolumesFrom = []apicontainer.VolumeFrom{{SourceContainer: testTask.Containers[0].Name, ReadOnly: false}}
	testTask.Containers[3].Command = []string{"New-Item c:/volume/readonly-fs-3; if ($?) { Exit 0 } else { Exit 42 }"}
	testTask.Containers[3].Essential = false

	go taskEngine.AddTask(testTask)
	verifyTaskIsRunning(stateChangeEvents, testTask)
	// Make sure all the three test container stopped first
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	// Stop the task by stopping the essential container
	taskEngine.(*DockerTaskEngine).stopContainer(testTask, testTask.Containers[0])
	verifyTaskIsStopped(stateChangeEvents, testTask)

	assert.NotEqual(t, *testTask.Containers[1].GetKnownExitCode(), 0, "didn't exit due to failure to touch ro fs as expected: ", *testTask.Containers[1].GetKnownExitCode())
	assert.Equal(t, *testTask.Containers[2].GetKnownExitCode(), 0, "couldn't touch with default of rw")
	assert.Equal(t, *testTask.Containers[3].GetKnownExitCode(), 0, "couldn't touch with explicit rw")
}

func TestTaskLevelVolume(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	stateChangeEvents := taskEngine.StateChangeEvents()

	testTask, tmpDirectory, err := createVolumeTaskWindows("task", "TestTaskLevelVolume", "TestTaskLevelVolume", true)
	defer os.Remove(tmpDirectory)
	require.NoError(t, err, "creating test task failed")

	// modify the command of the container so that the container will write to the volume
	testTask.Containers[0].Command = []string{"Write-Output \"volume\" | Out-File -FilePath C:\\ecs\\volumecontent -Encoding ascii"}

	go taskEngine.AddTask(testTask)

	verifyTaskIsRunning(stateChangeEvents, testTask)
	verifyTaskIsStopped(stateChangeEvents, testTask)
	assert.NotEqual(t, testTask.ResourcesMapUnsafe["dockerVolume"][0].(*taskresourcevolume.VolumeResource).VolumeConfig.Source(), "TestTaskLevelVolume", "task volume name is the same as specified in task definition")

	// Find the volume mount path
	var sourceVolume string
	for _, vol := range testTask.Containers[0].MountPoints {
		if vol.ContainerPath == "c:\\ecs" {
			sourceVolume = vol.SourceVolume
		}
	}
	assert.NotEmpty(t, sourceVolume)

	volumeFile := filepath.Join(fmt.Sprintf(dockerVolumeDirectoryFormat, sourceVolume), "volumecontent")
	data, err := ioutil.ReadFile(volumeFile)
	assert.NoError(t, err)
	assert.Equal(t, string(data), "volume")
}

func TestGMSATaskFile(t *testing.T) {
	taskEngine, done, _ := setupGMSA(defaultTestConfigIntegTestGMSA(), nil, t)
	defer done()
	stateChangeEvents := taskEngine.StateChangeEvents()

	// Setup test gmsa file
	envProgramData := "ProgramData"
	dockerCredentialSpecDataDir := "docker/credentialspecs"
	programDataDir := os.Getenv(envProgramData)
	if programDataDir == "" {
		programDataDir = "C:/ProgramData"
	}
	testFileName := "test-gmsa.json"
	testCredSpecFilePath := filepath.Join(programDataDir, dockerCredentialSpecDataDir, testFileName)
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

	err := ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	require.NoError(t, err)

	testContainer := createTestContainer()
	testContainer.Name = "testGMSATaskFile"

	hostConfig := "{\"SecurityOpt\": [\"credentialspec:file://test-gmsa.json\"]}"
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
	testTask.Containers[0].Command = getLongRunningCommand()

	go taskEngine.AddTask(testTask)

	err = verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)

	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	testCredentialSpecOption := "credentialspec=file://test-gmsa.json"
	err = verifyContainerCredentialSpec(client, cid, testCredentialSpecOption)
	assert.NoError(t, err)

	// Kill the existing container now
	err = client.ContainerKill(context.TODO(), cid, "SIGKILL")
	assert.NoError(t, err, "Could not kill container")

	verifyTaskIsStopped(stateChangeEvents, testTask)

	// Cleanup the test file
	err = os.Remove(testCredSpecFilePath)
	assert.NoError(t, err)
}

func defaultTestConfigIntegTestGMSA() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.ImagePullBehavior = config.ImagePullPreferCachedBehavior

	cfg.GMSACapable = config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled}
	cfg.AWSRegion = "us-west-2"

	return cfg
}

func setupGMSA(cfg *config.Config, state dockerstate.TaskEngineState, t *testing.T) (TaskEngine, func(), credentials.Manager) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	if os.Getenv("ECS_SKIP_ENGINE_INTEG_TEST") != "" {
		t.Skip("ECS_SKIP_ENGINE_INTEG_TEST")
	}
	if !isDockerRunning() {
		t.Skip("Docker not running")
	}

	sdkClientFactory := sdkclientfactory.NewFactory(ctx, dockerEndpoint)
	dockerClient, err := dockerapi.NewDockerGoClient(sdkClientFactory, cfg, context.Background())
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	if state == nil {
		state = dockerstate.NewTaskEngineState()
	}
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetDataClient(data.NewNoopClient())
	metadataManager := containermetadata.NewManager(dockerClient, cfg)

	resourceFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator: ssmfactory.NewSSMClientCreator(),
			S3ClientCreator:  s3factory.NewS3ClientCreator(),
		},
		DockerClient: dockerClient,
	}

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, state, metadataManager,
		resourceFields, execcmd.NewManager(), engineserviceconnect.NewManager())
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

func verifyContainerCredentialSpec(client *sdkClient.Client, id, credentialspecOpt string) error {
	dockerContainer, err := client.ContainerInspect(context.TODO(), id)
	if err != nil {
		return err
	}

	for _, opt := range dockerContainer.HostConfig.SecurityOpt {
		if strings.HasPrefix(opt, "credentialspec=") && opt == credentialspecOpt {
			return nil
		}
	}

	return errors.New("unable to obtain credentialspec")
}

// This setup for execcmd is same as Linux, just implemented different
// TestExecCommandAgent validates ExecCommandAgent start and monitor processes. The algorithm to test is as follows:
// 1. Pre-setup: the build file in ../../misc/exec-command-agent-windows-test will create a special docker sleeper image
// based on a base windows 2019 image. This image simulates a ecs windows image and contains a pre-baked C:\sleep.exe binary.
// /sleep is the main process used to launch the test container; Then use the windows "taskkill /f /im amazon-ssm-agent.exe" to
// kill the running agent process in the container
// The build file will also create a fake amazon-ssm-agent which is a go program that only sleeps for a certain time specified.
//
// 2. Setup: Create a new docker task engine with a modified path pointing to our fake amazon-ssm-agent binary
// 3. Create and start our test task using our test image
// 4. Wait for the task to start and verify that the expected ExecCommandAgent bind mounts are present in the containers
// 5. Verify that our fake amazon-ssm-agent was started inside the container using docker top, and retrieve its PID
// 6. Kill the fake amazon-ssm-agent using the command in step 1
// 7. Verify that the engine restarted our fake amazon-ssm-agent by doing docker top one more time (a new PID should popup)
func TestExecCommandAgent(t *testing.T) {
	// the execcmd feature is not supported for Win2016
	if windows2016, _ := config.IsWindows2016(); windows2016 {
		return
	}
	const (
		testTaskId        = "exec-command-agent-test-task"
		testContainerName = "exec-command-agent-test-container"
		sleepFor          = time.Minute * 2
	)

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testExecCmdHostBinDir := "C:\\Program Files\\Amazon\\ECS\\managed-agents\\execute-command\\bin"

	taskEngine, done, _ := setupEngineForExecCommandAgent(t, testExecCmdHostBinDir)
	stateChangeEvents := taskEngine.StateChangeEvents()
	defer done()

	testTask := createTestExecCommandAgentTask(testTaskId, testContainerName, sleepFor)
	execAgentLogPath := filepath.Join("C:\\ProgramData\\Amazon\\ECS\\exec", testTaskId)
	err = os.MkdirAll(execAgentLogPath, 0644)
	require.NoError(t, err, "error creating execAgent log file")
	_, err = os.Stat(execAgentLogPath)
	require.NoError(t, err, "execAgent log dir doesn't exist")
	err = os.MkdirAll(execcmd.ECSAgentExecConfigDir, 0644)
	require.NoError(t, err, "error creating execAgent config dir")

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	// session limit is 2
	testconfigDirName, _ := execcmd.GetExecAgentConfigDir(2)

	// todo: change to file contents passed in
	verifyExecCmdAgentExpectedMounts(t, ctx, client, testTaskId, cid, testContainerName, testExecCmdHostBinDir+"\\1.0.0.0", testconfigDirName)
	pidA := verifyMockExecCommandAgentIsRunning(t, client, cid)
	seelog.Infof("Verified mock ExecCommandAgent is running (pidA=%s)", pidA)
	killMockExecCommandAgent(t, client, cid)
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
	// TODO: [ecs-exec] We should be able to wait for cleanup instead of calling deleteTask directly
	taskEngine.(*DockerTaskEngine).deleteTask(testTask)
	_, err = os.Stat(execAgentLogPath)
	assert.True(t, os.IsNotExist(err), "execAgent log cleanup failed")
	os.RemoveAll(execcmd.ECSAgentExecConfigDir)
}

// TestManagedAgentEvent validates the emitted container events for a started and a stopped managed agent.
func TestManagedAgentEvent(t *testing.T) {
	// the execcmd feature is not supported for Win2016
	if windows2016, _ := config.IsWindows2016(); windows2016 {
		return
	}
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

			testExecCmdHostBinDir := "C:\\Program Files\\Amazon\\ECS\\managed-agents\\execute-command\\bin"

			taskEngine, done, _ := setupEngineForExecCommandAgent(t, testExecCmdHostBinDir)
			defer done()

			testTask := createTestExecCommandAgentTask(testTaskId, testContainerName, time.Minute*tc.ManagedAgentLifetime)
			execAgentLogPath := filepath.Join("C:\\ProgramData\\Amazon\\ECS\\exec", testTaskId)
			err = os.MkdirAll(execAgentLogPath, 0644)
			require.NoError(t, err, "error creating execAgent log file")
			_, err = os.Stat(execAgentLogPath)
			require.NoError(t, err, "execAgent log dir doesn't exist")
			err = os.MkdirAll(execcmd.ECSAgentExecConfigDir, 0644)
			require.NoError(t, err, "error creating execAgent config dir")

			go taskEngine.AddTask(testTask)

			verifyContainerRunningStateChange(t, taskEngine)
			verifyTaskRunningStateChange(t, taskEngine)

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

			if tc.ShouldBeRunning {
				stateChangeEvents := taskEngine.StateChangeEvents()
				taskUpdate := createTestExecCommandAgentTask(testTaskId, testContainerName, time.Minute*tc.ManagedAgentLifetime)
				taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
				go taskEngine.AddTask(taskUpdate)

				ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
				go func() {
					verifyTaskIsStopped(stateChangeEvents, testTask)
					cancel()
				}()

				<-ctx.Done()
				require.NotEqual(t, context.DeadlineExceeded, ctx.Err(), "Timed out waiting for task (%s) to stop", testTaskId)
				assert.NotNil(t, testTask.Containers[0].GetKnownExitCode(), "No exit code found")
			}

			taskEngine.(*DockerTaskEngine).deleteTask(testTask)
			_, err = os.Stat(execAgentLogPath)
			assert.True(t, os.IsNotExist(err), "execAgent log cleanup failed")
			os.RemoveAll(execcmd.ECSAgentExecConfigDir)
		})
	}
}

func createTestExecCommandAgentTask(taskId, containerName string, sleepFor time.Duration) *apitask.Task {
	testTask := createTestTask("arn:aws:ecs:us-west-2:1234567890:task/" + taskId)
	testTask.PIDMode = ecs.PidModeHost
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
		nil, execCmdMgr, engineserviceconnect.NewManager())
	taskEngine.monitorExecAgentsInterval = time.Second
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

var (
	containerDepsDir = execcmd.ContainerDepsFolder
)

func verifyExecCmdAgentExpectedMounts(t *testing.T,
	ctx context.Context,
	client *sdkClient.Client,
	testTaskId, containerId, containerName, testExecCmdHostVersionedBinDir, testconfigDirName string) {
	inspectState, _ := client.ContainerInspect(ctx, containerId)

	// The dockerclient only gives back lowercase paths in Windows
	expectedMounts := []struct {
		source    string
		destRegex string
		readOnly  bool
	}{
		{
			source:    strings.ToLower(testExecCmdHostVersionedBinDir),
			destRegex: strings.ToLower(containerDepsDir),
			readOnly:  true,
		},
		{
			source:    strings.ToLower(filepath.Join(execcmd.HostExecConfigDir, testconfigDirName)),
			destRegex: strings.ToLower(filepath.Join(containerDepsDir, "configuration")),
			readOnly:  true,
		},
		{
			source:    strings.ToLower(filepath.Join(execcmd.HostLogDir, testTaskId, containerName)),
			destRegex: strings.ToLower(execcmd.ContainerLogDir),
			readOnly:  false,
		},
		{
			source:    strings.ToLower(execcmd.SSMPluginDir),
			destRegex: strings.ToLower(execcmd.SSMPluginDir),
			readOnly:  true,
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
		require.Equal(t, em.destRegex, found.Destination, "Destination for mount point (%s) is invalid expected: %s, actual: %s", em.source, em.destRegex, found.Destination)
		if em.readOnly {
			require.False(t, found.RW, found.Mode, "Destination for mount point (%s) should be read only", em.source)
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
	execCmdAgentProcessRegex := execcmd.SSMAgentBinName
	go func() {
		for {
			top, err := client.ContainerTop(ctx, containerId, nil)
			if err != nil {
				continue
			}
			cmdPos := -1
			pidPos := -1
			for i, t := range top.Titles {
				if strings.ToUpper(t) == "NAME" {
					cmdPos = i
				}
				if strings.ToUpper(t) == "PID" {
					pidPos = i
				}

			}
			require.NotEqual(t, -1, cmdPos, "NAME title not found in the container top response")
			require.NotEqual(t, -1, pidPos, "PID title not found in the container top response")
			for _, proc := range top.Processes {
				matched, _ := regexp.MatchString(execCmdAgentProcessRegex, proc[cmdPos])
				// Process we are checking to be stopped might still be running.
				// expectedPid matches the pid of the process in that case, so wait if that's
				// the case.
				if matched && (checkIsRunning || expectedPid != proc[pidPos]) {
					res <- proc[pidPos]
					return
				}
			}
			seelog.Infof("Processes running in container: %s", top.Processes)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 1):
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

func killMockExecCommandAgent(t *testing.T, client *sdkClient.Client, containerId string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	// this is the same user used to start the execcmd agent (ssm agent)
	containerNTUser := "NT AUTHORITY\\SYSTEM"
	create, err := client.ContainerExecCreate(ctx, containerId, types.ExecConfig{
		User:   containerNTUser,
		Detach: true,
		Cmd:    []string{"cmd", "/C", "taskkill /F /IM amazon-ssm-agent.exe"},
	})
	require.NoError(t, err)

	err = client.ContainerExecStart(ctx, create.ID, types.ExecStartCheck{
		Detach: true,
	})
	require.NoError(t, err)
}
