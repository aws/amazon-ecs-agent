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
	"strconv"
	"strings"
	"testing"

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
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dockerEndpoint              = "npipe:////./pipe/docker_engine"
	testVolumeImage             = "amazon/amazon-ecs-volumes-test:make"
	testRegistryImage           = "amazon/amazon-ecs-netkitten:make"
	testBaseImage               = "amazon-ecs-ftest-windows-base:make"
	dockerVolumeDirectoryFormat = "c:\\ProgramData\\docker\\volumes\\%s\\_data"
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

func createVolumeTask(scope, arn, volume string, autoprovision bool) (*apitask.Task, string, error) {
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

	testTask, tmpDirectory, err := createVolumeTask("task", "TestTaskLevelVolume", "TestTaskLevelVolume", true)
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

	cfg.GMSACapable = true
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
		},
		DockerClient:    dockerClient,
		S3ClientCreator: s3factory.NewS3ClientCreator(),
	}

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, state, metadataManager, resourceFields)
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
