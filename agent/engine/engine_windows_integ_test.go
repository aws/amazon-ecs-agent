// +build windows,integration

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

package engine

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dockerEndpoint              = "npipe:////./pipe/docker_engine"
	testVolumeImage             = "amazon/amazon-ecs-volumes-test:make"
	testRegistryImage           = "amazon/amazon-ecs-netkitten:make"
	testHelloworldImage         = "cggruszka/microsoft-windows-helloworld:latest"
	dockerVolumeDirectoryFormat = "c:\\ProgramData\\docker\\volumes\\%s\\_data"
)

var endpoint = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), dockerEndpoint)

// TODO implement this
func isDockerRunning() bool { return true }

func createTestContainer() *apicontainer.Container {
	return &apicontainer.Container{
		Name:                "windows",
		Image:               "microsoft/windowsservercore:latest",
		Essential:           true,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		CPU:                 512,
		Memory:              256,
	}
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
	testTask.Containers[0].Image = "microsoft/nanoserver:latest"
	testTask.Containers[0].Name = "test-health-check"
	testTask.Containers[0].HealthCheckType = "docker"
	testTask.Containers[0].Command = []string{"powershell", "-command", "Start-Sleep -s 300"}
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{
		Config: aws.String(`{
			"HealthCheck":{
				"Test":["CMD-SHELL", "echo hello"],
				"Interval":100000000,
				"Timeout":100000000,
				"StartPeriod":100000000,
				"Retries":3}
		}`),
	}
	return testTask
}

func createVolumeTask(scope, arn, volume string, provisioned bool) (*apitask.Task, string, error) {
	testTask := createTestTask(arn)
	testTask.Volumes = []apitask.TaskVolume{
		{
			Type: "docker",
			Name: volume,
			Volume: &taskresourcevolume.DockerVolumeConfig{
				Scope:         scope,
				Autoprovision: provisioned,
				Driver:        "local",
			},
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

func getContainerIP(client *docker.Client, id string) (string, error) {
	dockerContainer, err := client.InspectContainer(id)
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
	removeImage(t, testHelloworldImage)

	testTask := createTestTask("testStartUnpulled")
	testTask.Containers[0].Image = testHelloworldImage

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
	imageDigest := "cggruszka/microsoft-windows-helloworld@sha256:89282ba3e122e461381eae854d142c6c4895fcc1087d4849dbe4786fb21018f8"
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(t, imageDigest)

	testTask := createTestTask("testStartUnpulledDigest")
	testTask.Containers[0].Image = imageDigest

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

// TestPortForward runs a container serving data on the randomly chosen port
// 24751 and verifies that when you do forward the port you can access it and if
// you don't forward the port you can't
func TestPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()
	client, _ := docker.NewClient(endpoint)

	testArn := "testPortForwardFail"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Image = testRegistryImage
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent}

	// Port not forwarded; verify we can't access it
	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID

	cip, err := getContainerIP(client, cid)
	require.NoError(t, err, "failed to acquire container ip from docker")

	time.Sleep(waitForDockerDuration) // wait for Docker
	_, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", cip, containerPortOne), dialTimeout)
	assert.Error(t, err, "Did not expect to be able to dial port %d but didn't get error", containerPortOne)

	// Kill the existing container now to make the test run more quickly.
	err = client.KillContainer(docker.KillContainerOptions{ID: cid})
	assert.NoError(t, err, "Could not kill container")

	verifyTaskIsStopped(stateChangeEvents, testTask)

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	testTask.Containers[0].Image = testRegistryImage
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent}
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortOne}}

	taskEngine.AddTask(testTask)

	err = verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)

	containerMap, _ = taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid = containerMap[testTask.Containers[0].Name].DockerID
	cip, err = getContainerIP(client, cid)
	require.NoError(t, err, "failed to acquire container ip from docker")

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", cip, containerPortOne), 10, dialTimeout)
	require.NoError(t, err, "error dialing simple container ")

	var response []byte
	for i := 0; i < 10; i++ {
		response, err = ioutil.ReadAll(conn)
		assert.NoError(t, err, "error reading response")
		if len(response) > 0 {
			break
		}
		// Retry for a non-blank response. The container in docker 1.7+ sometimes
		// isn't up quickly enough and we get a blank response. It's still unclear
		// to me if this is a docker bug or netkitten bug
		t.Log("Retrying getting response from container; got nothing")
		time.Sleep(100 * time.Millisecond)
	}
	assert.Equal(t, string(response), serverContent, "got response: "+string(response)+" instead of ", serverContent)

	// Stop the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

// TestMultiplePortForwards tests that two ldockinks containers in the same task can
// both expose ports successfully
func TestMultiplePortForwards(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()
	client, _ := docker.NewClient(endpoint)

	// Forward it and make sure that works
	testArn := "testMultiplePortForwards"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Image = testRegistryImage
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent + "1"}
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortOne}}
	testTask.Containers[0].Essential = false
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Image = testRegistryImage
	testTask.Containers[1].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent + "2"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortTwo}}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid1 := containerMap[testTask.Containers[0].Name].DockerID
	cid2 := containerMap[testTask.Containers[1].Name].DockerID

	cip1, err := getContainerIP(client, cid1)
	require.NoError(t, err, "failed to acquire the container ip from docker")
	cip2, err := getContainerIP(client, cid2)
	require.NoError(t, err, "failed to acquire the container ip from docker")

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", cip1, containerPortOne), 10, dialTimeout)
	require.NoError(t, err, "error dialing simple container 1 ")
	t.Log("Dialed first container")
	response, _ := ioutil.ReadAll(conn)
	assert.Equal(t, string(response), serverContent+"1", "got response: "+string(response)+" instead of "+serverContent+"1")

	t.Log("Read first container")
	conn, err = dialWithRetries("tcp", fmt.Sprintf("%s:%d", cip2, containerPortOne), 10, dialTimeout)
	require.NoError(t, err, "error dialing simple container 2")
	t.Log("Dialed second container")
	response, _ = ioutil.ReadAll(conn)
	assert.Equal(t, string(response), serverContent+"2", "got response: "+string(response)+" instead of "+serverContent+"2")
	t.Log("Read second container")

	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

// TestDynamicPortForward runs a container serving data on a port chosen by the
// docker deamon and verifies that the port is reported in the state-change
func TestDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "testDynamicPortForward"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Image = testRegistryImage
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning, "Expected task to be RUNNING")

	assert.Len(t, portBindings, 1, "portBindings was not set; should have been len 1")

	var bindingFor24751 uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == containerPortOne {
			bindingFor24751 = binding.HostPort
		}
	}
	assert.NotEqual(t, bindingFor24751, 0, "could not find the port mapping for %d", containerPortOne)

	client, _ := docker.NewClient(endpoint)
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	cip, err := getContainerIP(client, cid)
	assert.NoError(t, err)

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", cip+fmt.Sprintf(":%d", containerPortOne), 10, dialTimeout)
	require.NoError(t, err, "error dialing simple container")

	response, _ := ioutil.ReadAll(conn)
	assert.Equal(t, string(response), serverContent, "got response: "+string(response)+" instead of %s", serverContent)

	// Kill the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped, "Expected container to be STOPPED")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped, "Expected task to be STOPPED")
}

func TestMultipleDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Image = testRegistryImage
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent, `-loop`}
	// No HostPort or 0 hostport; docker should pick two ports for us
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne}, {ContainerPort: containerPortOne, HostPort: 0}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning, "Expected task to be RUNNING")

	assert.Len(t, portBindings, 2, "could not bind to two ports from one container port", portBindings)
	var bindingFor24751_1 uint16
	var bindingFor24751_2 uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == containerPortOne {
			if bindingFor24751_1 == 0 {
				bindingFor24751_1 = binding.HostPort
			} else {
				bindingFor24751_2 = binding.HostPort
			}
		}
	}
	assert.NotZero(t, bindingFor24751_1, "could not find the port mapping for ", containerPortOne)
	assert.NotZero(t, bindingFor24751_2, "could not find the port mapping for ", containerPortOne)

	client, _ := docker.NewClient(endpoint)
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	cip, err := getContainerIP(client, cid)
	assert.NoError(t, err)

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", cip, containerPortOne), 10, dialTimeout)
	require.NoError(t, err, "error dialing simple container")

	response, _ := ioutil.ReadAll(conn)
	assert.Equal(t, string(response), serverContent, "got response: "+string(response)+" instead of %s", serverContent)

	// Kill the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

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

	testTask, tmpDirectory, err := createVolumeTask("task", "TestTaskLevelVolume", "TestTaskLevelVolume", false)
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
