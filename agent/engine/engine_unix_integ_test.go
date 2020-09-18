// +build !windows,integration

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
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRegistryHost          = "127.0.0.1:51670"
	testBusyboxImage          = testRegistryHost + "/busybox:latest"
	testAuthRegistryHost      = "127.0.0.1:51671"
	testAuthRegistryImage     = "127.0.0.1:51671/amazon/amazon-ecs-netkitten:latest"
	testVolumeImage           = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
	testExecCommandAgentImage = "127.0.0.1:51670/amazon/amazon-ecs-exec-command-agent-test:latest"
	testPIDNamespaceImage     = "127.0.0.1:51670/amazon/amazon-ecs-pid-namespace-test:latest"
	testIPCNamespaceImage     = "127.0.0.1:51670/amazon/amazon-ecs-ipc-namespace-test:latest"
	testUbuntuImage           = "127.0.0.1:51670/ubuntu:latest"
	testFluentdImage          = "127.0.0.1:51670/amazon/fluentd:latest"
	testAuthUser              = "user"
	testAuthPass              = "swordfish"

	// Search for the running process
	testPIDNamespaceCommand         = "if [ `ps ax | grep pidNamespaceTest | grep -vc grep` -gt 0 ]; then exit 1; else exit 2; fi;"
	testPIDNamespaceProcessFound    = 1
	testPIDNamespaceProcessNotFound = 2

	// Search for a semaphore with a Key of 5 (created in ipcNamespaceTest.c
	// running in testIPCNamespaceImage)
	testIPCNamespaceCommand          = "if [ `ipcs -s | awk '$1+0 == 5' | wc -l` -gt 0 ]; then exit 1; else exit 2; fi;"
	testIPCNamespaceResourceFound    = 1
	testIPCNamespaceResourceNotFound = 2

	testGPUImage         = "nvidia/cuda:9.0-base"
	testGPUContainerName = "testGPUContainer"
	gpuConfigFilePath    = "/var/lib/ecs/ecs.config"
)

var (
	endpoint            = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)
	TestGPUInstanceType = []string{"p2", "p3", "g3", "g4dn"}
)

func createTestHealthCheckTask(arn string) *apitask.Task {
	testTask := &apitask.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{createTestContainer()},
	}
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Name = "test-health-check"
	testTask.Containers[0].HealthCheckType = "docker"
	testTask.Containers[0].Command = []string{"sh", "-c", "sleep 300"}
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{
		Config: aws.String(alwaysHealthyHealthCheckConfig),
	}
	return testTask
}

// All Namespace Sharing Tests will rely on 3 containers
// container0 will be the container that starts an executable or creates a resource
// container1 and container2 will attempt to see this process/resource
// and quit with exit 0 for success and 1 for failure
func createNamespaceSharingTask(arn, pidMode, ipcMode, testImage string, theCommand []string) *apitask.Task {
	testTask := &apitask.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		PIDMode:             pidMode,
		IPCMode:             ipcMode,
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				Name:                      "container0",
				Image:                     testImage,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
			&apicontainer.Container{
				Name:                      "container1",
				Image:                     testBusyboxImage,
				Command:                   theCommand,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
			&apicontainer.Container{
				Name:                      "container2",
				Image:                     testBusyboxImage,
				Command:                   theCommand,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}

	// Setting a container dependency so the executable can be started or resource can be created
	// before read is attempted by other containers
	testTask.Containers[1].BuildContainerDependency(testTask.Containers[0].Name, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated)
	testTask.Containers[2].BuildContainerDependency(testTask.Containers[0].Name, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerCreated)
	return testTask
}

func createVolumeTask(scope, arn, volume string, autoprovision bool) (*apitask.Task, string, error) {
	tmpDirectory, err := ioutil.TempDir("", "ecs_test")
	if err != nil {
		return nil, "", err
	}
	err = ioutil.WriteFile(filepath.Join(tmpDirectory, "volume-data"), []byte("volume"), 0666)
	if err != nil {
		return nil, "", err
	}

	testTask := createTestTask(arn)

	volumeConfig := &taskresourcevolume.DockerVolumeConfig{
		Scope:  scope,
		Driver: "local",
		DriverOpts: map[string]string{
			"device": tmpDirectory,
			"o":      "bind",
			"type":   "tmpfs",
		},
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

	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].TransitionDependenciesMap = make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet)
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{
		{
			SourceVolume:  volume,
			ContainerPath: "/ecs",
		},
	}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	testTask.Containers[0].Command = []string{"sh", "-c", "if [[ $(cat /ecs/volume-data) != \"volume\" ]]; then cat /ecs/volume-data; exit 1; fi; exit 0"}
	return testTask, tmpDirectory, nil
}

func createTestGPUTask() *apitask.Task {
	return &apitask.Task{
		Arn:                 "testGPUArn",
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{createTestGPUContainerWithImage(testGPUImage)},
		Associations: []apitask.Association{
			{
				Containers: []string{
					testGPUContainerName,
				},
				Content: apitask.EncodedString{
					Encoding: "base64",
					Value:    "val",
				},
				Name: "0",
				Type: apitask.GPUAssociationType,
			},
		},
		NvidiaRuntime: config.DefaultNvidiaRuntime,
	}
}

func createTestGPUContainerWithImage(image string) *apicontainer.Container {
	return &apicontainer.Container{
		Name:                testGPUContainerName,
		Image:               image,
		Command:             []string{},
		Essential:           true,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		CPU:                 100,
		Memory:              80,
	}
}

func getGPUEnvVar(filename string) map[string]string {
	envVariables := make(map[string]string)

	file, err := os.Open(filename)
	if err != nil {
		return envVariables
	}

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return envVariables
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		envVariables[parts[0]] = parts[1]
	}
	return envVariables
}

// This Test starts an executable named pidNamespaceTest on one docker container.
// Other containers will query the terminal for this process.
func TestHostPIDNamespaceSharingInSingleTask(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	theCommand := []string{"sh", "-c", testPIDNamespaceCommand}
	testTask := createNamespaceSharingTask("TaskSharePIDWithTask", "host", "", testPIDNamespaceImage, theCommand)
	go taskEngine.AddTask(testTask)

	testEvents := InitEventCollection(taskEngine)

	err := VerifyTaskStatus(apitaskstatus.TaskRunning, testTask.Arn, testEvents, t)
	require.NoError(t, err, "Not verified task running")
	require.Equal(t, "host", testTask.PIDMode)
	//Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container1", testEvents, t)
	require.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container2", testEvents, t)
	require.NoError(t, err)

	//Manually stop container0
	cont0, _ := testTask.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTask, cont0)

	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTask.Arn, testEvents, t)
	require.NoError(t, err)

	cont1, _ := testTask.ContainerByName("container1")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont1.GetKnownExitCode()), "container1 could not see NamespaceTest process")
	cont2, _ := testTask.ContainerByName("container2")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont2.GetKnownExitCode()), "container2 could not see NamespaceTest process")
}

// This Test starts an executable in a container in Task1 sharing the PID namespace with Host.
// Another Task is started and should be able to see this running executable
func TestHostPIDNamespaceSharingInMultipleTasks(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	theCommand := []string{"sh", "-c", testPIDNamespaceCommand}
	testTaskWithProcess := createNamespaceSharingTask("TaskWithProcess", "host", "", testPIDNamespaceImage, theCommand)
	testTaskWithoutProcess := &apitask.Task{
		Arn:                 "TaskWithoutProcess",
		Family:              "family",
		Version:             "1",
		PIDMode:             "host",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				Name:                      "container0",
				Image:                     testBusyboxImage,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				Command:                   theCommand,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}

	testEvents := InitEventCollection(taskEngine)

	// Run Task with Process attached
	go taskEngine.AddTask(testTaskWithProcess)
	err := VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithProcess.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "host", testTaskWithProcess.PIDMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container1", testEvents, t)
	require.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container2", testEvents, t)
	require.NoError(t, err)
	cont1, _ := testTaskWithProcess.ContainerByName("container1")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont1.GetKnownExitCode()), "container1 could not see NamespaceTest process")
	cont2, _ := testTaskWithProcess.ContainerByName("container2")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont2.GetKnownExitCode()), "container2 could not see NamespaceTest process")

	// Run Task without Process attached
	go taskEngine.AddTask(testTaskWithoutProcess)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutProcess.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "host", testTaskWithoutProcess.PIDMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutProcess.Arn+":container0", testEvents, t)
	require.NoError(t, err)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutProcess.Arn, testEvents, t)
	require.NoError(t, err)
	cont0, _ := testTaskWithoutProcess.ContainerByName("container0")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont0.GetKnownExitCode()), "container0 could not see NamespaceTest process, but should be able to.")

	// Test is complete, can stop container with process
	cont, _ := testTaskWithProcess.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithProcess, cont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithProcess.Arn, testEvents, t)
	require.NoError(t, err)
}

// This Test starts an executable in a container in Task1 with the PID namespace within the Task.
// Another Task is started and should not be able to see this running executable
func TestTaskPIDNamespaceSharingInMultipleTasks(t *testing.T) {
	config.DefaultPauseContainerImageName = "amazon/amazon-ecs-pause"
	config.DefaultPauseContainerTag = "0.1.0"
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	theCommand := []string{"sh", "-c", testPIDNamespaceCommand}
	testTaskWithProcess := createNamespaceSharingTask("TaskWithProcess", "task", "", testPIDNamespaceImage, theCommand)
	testTaskWithoutProcess := &apitask.Task{
		Arn:                 "TaskWithoutProcess",
		Family:              "family",
		Version:             "1",
		PIDMode:             "task",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				Name:                      "container0",
				Image:                     testBusyboxImage,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				Command:                   theCommand,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}

	testEvents := InitEventCollection(taskEngine)

	// Run Task with Process attached
	go taskEngine.AddTask(testTaskWithProcess)
	err := VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithProcess.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "task", testTaskWithProcess.PIDMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container1", testEvents, t)
	require.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container2", testEvents, t)
	require.NoError(t, err)
	cont1, _ := testTaskWithProcess.ContainerByName("container1")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont1.GetKnownExitCode()), "container1 could not see NamespaceTest process")
	cont2, _ := testTaskWithProcess.ContainerByName("container2")
	require.Equal(t, testPIDNamespaceProcessFound, *(cont2.GetKnownExitCode()), "container2 could not see NamespaceTest process")

	// Run Task without Process attached
	go taskEngine.AddTask(testTaskWithoutProcess)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutProcess.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "task", testTaskWithoutProcess.PIDMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutProcess.Arn+":container0", testEvents, t)
	require.NoError(t, err)

	// Manually stop the Pause container and verify if TaskWithoutProcess has stopped
	pauseCont, _ := testTaskWithoutProcess.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithoutProcess, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutProcess.Arn, testEvents, t)
	require.NoError(t, err)

	cont0, _ := testTaskWithoutProcess.ContainerByName("container0")
	require.Equal(t, testPIDNamespaceProcessNotFound, *(cont0.GetKnownExitCode()), "container0 could see NamespaceTest process, but shouldn't be able to.")

	// Test is complete, can stop container with process
	cont, _ := testTaskWithProcess.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithProcess, cont)
	// Manually stop the Pause container and verify if TaskWithProcess has stopped
	pauseCont, _ = testTaskWithProcess.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithProcess, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithProcess.Arn, testEvents, t)
	require.NoError(t, err)
}

// This Test creates an IPC semaphore on one docker container.
// Other containers will query the terminal for this semaphore.
func TestHostIPCNamespaceSharingInSingleTask(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	theCommand := []string{"sh", "-c", testIPCNamespaceCommand}
	testTask := createNamespaceSharingTask("TaskShareIPCWithHost", "", "host", testIPCNamespaceImage, theCommand)

	testEvents := InitEventCollection(taskEngine)

	go taskEngine.AddTask(testTask)
	err := VerifyTaskStatus(apitaskstatus.TaskRunning, testTask.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "host", testTask.IPCMode)

	//Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container1", testEvents, t)
	require.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container2", testEvents, t)
	require.NoError(t, err)
	//Manually stop container0
	cont0, _ := testTask.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTask, cont0)

	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTask.Arn, testEvents, t)
	require.NoError(t, err)
	cont1, _ := testTask.ContainerByName("container1")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont1.GetKnownExitCode()), "container1 could not see IPC Semaphore")
	cont2, _ := testTask.ContainerByName("container2")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont2.GetKnownExitCode()), "container2 could not see IPC Semaphore")
}

// This Test creates an IPC Semaphore in a container in TaskWithResource sharing the IPC namespace with Host.
// Another Task is started and should be able to see the created IPC Semaphore
func TestHostIPCNamespaceSharingInMultipleTasks(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	theCommand := []string{"sh", "-c", testIPCNamespaceCommand}
	testTaskWithResource := createNamespaceSharingTask("TaskWithResource", "", "host", testIPCNamespaceImage, theCommand)
	testTaskWithoutResource := &apitask.Task{
		Arn:                 "TaskWithoutResource",
		Family:              "family",
		Version:             "1",
		IPCMode:             "host",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				Name:                      "container0",
				Image:                     testBusyboxImage,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				Command:                   theCommand,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}

	testEvents := InitEventCollection(taskEngine)

	// Run Task with IPC Semaphore attached
	go taskEngine.AddTask(testTaskWithResource)
	err := VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithResource.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "host", testTaskWithResource.IPCMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container1", testEvents, t)
	require.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container2", testEvents, t)
	require.NoError(t, err)
	cont1, _ := testTaskWithResource.ContainerByName("container1")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont1.GetKnownExitCode()), "container1 could not see IPC Semaphore")
	cont2, _ := testTaskWithResource.ContainerByName("container2")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont2.GetKnownExitCode()), "container2 could not see IPC Semaphore")

	// Run Task with IPC Semaphore attached
	go taskEngine.AddTask(testTaskWithoutResource)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutResource.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "host", testTaskWithoutResource.IPCMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutResource.Arn+":container0", testEvents, t)
	require.NoError(t, err)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutResource.Arn, testEvents, t)
	require.NoError(t, err)
	cont0, _ := testTaskWithoutResource.ContainerByName("container0")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont0.GetKnownExitCode()), "container0 could not see IPC Semaphore, but should be able to.")

	// Test is complete, can stop container with process
	cont, _ := testTaskWithResource.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithResource, cont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithResource.Arn, testEvents, t)
	require.NoError(t, err)
}

// This Test creates an IPC Semaphore in a container in TaskWithResource with the IPC namespace within the Task.
// Another Task is started and should not be able to see this created semaphore
func TestTaskIPCNamespaceSharingInMultipleTasks(t *testing.T) {
	config.DefaultPauseContainerImageName = "amazon/amazon-ecs-pause"
	config.DefaultPauseContainerTag = "0.1.0"
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	theCommand := []string{"sh", "-c", testIPCNamespaceCommand}
	testTaskWithResource := createNamespaceSharingTask("TaskWithResource", "", "task", testIPCNamespaceImage, theCommand)
	testTaskWithoutResource := &apitask.Task{
		Arn:                 "TaskWithoutResource",
		Family:              "family",
		Version:             "1",
		IPCMode:             "task",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers: []*apicontainer.Container{
			&apicontainer.Container{
				Name:                      "container0",
				Image:                     testBusyboxImage,
				DesiredStatusUnsafe:       apicontainerstatus.ContainerRunning,
				CPU:                       100,
				Memory:                    80,
				Command:                   theCommand,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			},
		},
	}

	testEvents := InitEventCollection(taskEngine)

	// Run Task with Semaphore
	go taskEngine.AddTask(testTaskWithResource)
	err := VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithResource.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "task", testTaskWithResource.IPCMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container1", testEvents, t)
	require.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container2", testEvents, t)
	require.NoError(t, err)
	cont1, _ := testTaskWithResource.ContainerByName("container1")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont1.GetKnownExitCode()), "container1 could not see IPC Semaphore")
	cont2, _ := testTaskWithResource.ContainerByName("container2")
	require.Equal(t, testIPCNamespaceResourceFound, *(cont2.GetKnownExitCode()), "container2 could not see IPC Semaphore")

	// Run Task without Semaphore
	go taskEngine.AddTask(testTaskWithoutResource)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutResource.Arn, testEvents, t)
	require.NoError(t, err)
	require.Equal(t, "task", testTaskWithoutResource.IPCMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutResource.Arn+":container0", testEvents, t)
	require.NoError(t, err)

	// Manually stop the Pause container and verify if TaskWithoutResource has stopped
	pauseCont, _ := testTaskWithoutResource.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithoutResource, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutResource.Arn, testEvents, t)
	require.NoError(t, err)

	cont0, _ := testTaskWithoutResource.ContainerByName("container0")
	require.Equal(t, testIPCNamespaceResourceNotFound, *(cont0.GetKnownExitCode()), "container0 could see IPC Semaphore, but shouldn't be able to.")

	// Test is complete, can stop container with semaphore
	cont, _ := testTaskWithResource.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithResource, cont)
	// Manually stop the Pause container and verify if TaskWithResource has stopped
	pauseCont, _ = testTaskWithResource.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithResource, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithResource.Arn, testEvents, t)
	require.NoError(t, err)
}

// TestStartStopUnpulledImage ensures that an unpulled image is successfully
// pulled, run, and stopped via docker.
func TestStartStopUnpulledImage(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	// Ensure this image isn't pulled by deleting it
	removeImage(t, testRegistryImage)

	testTask := createTestTask("testStartUnpulled")

	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

// TestStartStopUnpulledImageDigest ensures that an unpulled image with
// specified digest is successfully pulled, run, and stopped via docker.
func TestStartStopUnpulledImageDigest(t *testing.T) {
	imageDigest := "tianon/true@sha256:30ed58eecb0a44d8df936ce2efce107c9ac20410c915866da4c6a33a3795d057"
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(t, imageDigest)

	testTask := createTestTask("testStartUnpulledDigest")
	testTask.Containers[0].Image = imageDigest

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

// TestPortForward runs a container serving data on the randomly chosen port
// 24751 and verifies that when you do forward the port you can access it and if
// you don't forward the port you can't
func TestPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "testPortForwardFail"
	testTask := createTestTask(testArn)
	port1 := getUnassignedPort()
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", port1), "-serve", serverContent}

	// Port not forwarded; verify we can't access it
	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)

	time.Sleep(waitForDockerDuration) // wait for Docker
	_, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", localhost, port1), dialTimeout)

	require.Error(t, err, "Did not expect to be able to dial %s:%d but didn't get error", localhost, port1)

	// Kill the existing container now to make the test run more quickly.
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	err = client.ContainerKill(context.TODO(), cid, "SIGKILL")
	require.NoError(t, err, "Could not kill container", err)

	verifyTaskIsStopped(stateChangeEvents, testTask)

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	port2 := getUnassignedPort()
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", port2), "-serve", serverContent}
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: port2, HostPort: port2}}

	taskEngine.AddTask(testTask)

	err = verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, port2), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	var response []byte
	for i := 0; i < 10; i++ {
		response, err = ioutil.ReadAll(conn)
		if err != nil {
			t.Error("Error reading response", err)
		}
		if len(response) > 0 {
			break
		}
		// Retry for a non-blank response. The container in docker 1.7+ sometimes
		// isn't up quickly enough and we get a blank response. It's still unclear
		// to me if this is a docker bug or netkitten bug
		t.Log("Retrying getting response from container; got nothing")
		time.Sleep(100 * time.Millisecond)
	}
	if string(response) != serverContent {
		t.Error("Got response: " + string(response) + " instead of " + serverContent)
	}

	// Stop the existing container now
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

// TestMultiplePortForwards tests that two links containers in the same task can
// both expose ports successfully
func TestMultiplePortForwards(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	// Forward it and make sure that works
	testArn := "testMultiplePortForwards"
	testTask := createTestTask(testArn)
	port1 := getUnassignedPort()
	port2 := getUnassignedPort()
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", port1), "-serve", serverContent + "1"}
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: port1, HostPort: port1}}
	testTask.Containers[0].Essential = false
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Command = []string{fmt.Sprintf("-l=%d", port1), "-serve", serverContent + "2"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: port1, HostPort: port2}}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, port1), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container 1 " + err.Error())
	}
	t.Log("Dialed first container")
	response, _ := ioutil.ReadAll(conn)
	if string(response) != serverContent+"1" {
		t.Error("Got response: " + string(response) + " instead of" + serverContent + "1")
	}
	t.Log("Read first container")
	conn, err = dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, port2), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container 2 " + err.Error())
	}
	t.Log("Dialed second container")
	response, _ = ioutil.ReadAll(conn)
	if string(response) != serverContent+"2" {
		t.Error("Got response: " + string(response) + " instead of" + serverContent + "2")
	}
	t.Log("Read second container")

	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)
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
	port := getUnassignedPort()
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", port), "-serve", serverContent}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: port}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	require.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings

	verifyTaskRunningStateChange(t, taskEngine)

	if len(portBindings) != 1 {
		t.Error("PortBindings was not set; should have been len 1", portBindings)
	}
	var bindingForcontainerPortOne uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == port {
			bindingForcontainerPortOne = binding.HostPort
		}
	}
	if bindingForcontainerPortOne == 0 {
		t.Errorf("Could not find the port mapping for %d!", port)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", localhost+":"+strconv.Itoa(int(bindingForcontainerPortOne)), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != serverContent {
		t.Error("Got response: " + string(response) + " instead of " + serverContent)
	}

	// Kill the existing container now
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

func TestMultipleDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	port := getUnassignedPort()
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", port), "-serve", serverContent, `-loop`}
	// No HostPort or 0 hostport; docker should pick two ports for us
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: port}, {ContainerPort: port, HostPort: 0}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	require.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings

	verifyTaskRunningStateChange(t, taskEngine)

	if len(portBindings) != 2 {
		t.Error("Could not bind to two ports from one container port", portBindings)
	}
	var bindingForcontainerPortOne_1 uint16
	var bindingForcontainerPortOne_2 uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == port {
			if bindingForcontainerPortOne_1 == 0 {
				bindingForcontainerPortOne_1 = binding.HostPort
			} else {
				bindingForcontainerPortOne_2 = binding.HostPort
			}
		}
	}
	if bindingForcontainerPortOne_1 == 0 {
		t.Errorf("Could not find the port mapping for %d!", port)
	}
	if bindingForcontainerPortOne_2 == 0 {
		t.Errorf("Could not find the port mapping for %d!", port)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", localhost+":"+strconv.Itoa(int(bindingForcontainerPortOne_1)), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != serverContent {
		t.Error("Got response: " + string(response) + " instead of " + serverContent)
	}

	conn, err = dialWithRetries("tcp", localhost+":"+strconv.Itoa(int(bindingForcontainerPortOne_2)), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ = ioutil.ReadAll(conn)
	if string(response) != serverContent {
		t.Error("Got response: " + string(response) + " instead of " + serverContent)
	}

	// Kill the existing container now
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

// TestLinking ensures that container linking does allow networking to go
// through to a linked container.  this test specifically starts a server that
// prints "hello linker" and then links a container that proxies that data to
// a publicly exposed port, where the tests reads it
func TestLinking(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	testArn := "TestLinking"
	testTask := createTestTask(testArn)
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[0].Command = []string{"-l=80", "-serve", "hello linker"}
	testTask.Containers[0].Name = "linkee"
	port := getUnassignedPort()
	testTask.Containers[1].Command = []string{fmt.Sprintf("-l=%d", port), "linkee_alias:80"}
	testTask.Containers[1].Links = []string{"linkee:linkee_alias"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: port, HostPort: port}}

	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(waitForDockerDuration)

	var response []byte
	for i := 0; i < 10; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", localhost, port), dialTimeout)
		if err != nil {
			t.Log("Error dialing simple container" + err.Error())
		}
		response, err = ioutil.ReadAll(conn)
		if err != nil {
			t.Error("Error reading response", err)
		}
		if len(response) > 0 {
			break
		}
		// Retry for a non-blank response. The container in docker 1.7+ sometimes
		// isn't up quickly enough and we get a blank response. It's still unclear
		// to me if this is a docker bug or netkitten bug
		t.Log("Retrying getting response from container; got nothing")
		time.Sleep(500 * time.Millisecond)
	}
	if string(response) != "hello linker" {
		t.Error("Got response: " + string(response) + " instead of 'hello linker'")
	}

	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyTaskIsStopped(stateChangeEvents, testTask)
}

func TestDockerCfgAuth(t *testing.T) {
	logdir := setupIntegTestLogs(t)
	defer os.RemoveAll(logdir)

	authString := base64.StdEncoding.EncodeToString([]byte(testAuthUser + ":" + testAuthPass))
	cfg := defaultTestConfigIntegTest()
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` +
		testAuthRegistryHost + `/v1/":{"auth":"` + authString + `"}}`))
	cfg.EngineAuthType = "dockercfg"

	removeImage(t, testAuthRegistryImage)
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()
	defer func() {
		cfg.EngineAuthData = config.NewSensitiveRawMessage(nil)
		cfg.EngineAuthType = ""
	}()

	testArn := "testDockerCfgAuth"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Image = testAuthRegistryImage

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	// Create instead of copying the testTask, to avoid race condition.
	// AddTask idempotently handles update, filtering by Task ARN.
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	// Flushes all currently buffered logs
	seelog.Flush()

	// verify there's no sign of auth details in the config; action item taken as
	// a result of accidentally logging them once
	badStrings := []string{"user:swordfish", "swordfish", authString}
	err := filepath.Walk(logdir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		t.Logf("Reading file:%s", path)
		if err != nil {
			return err
		}
		for _, badstring := range badStrings {
			if strings.Contains(string(data), badstring) {
				t.Fatalf("log data contained bad string: %v, %v", string(data), badstring)
			}
			if strings.Contains(string(data), fmt.Sprintf("%v", []byte(badstring))) {
				t.Fatalf("log data contained byte-slice representation of bad string: %v, %v", string(data), badstring)
			}
			gobytes := fmt.Sprintf("%#v", []byte(badstring))
			// format is []byte{0x12, 0x34}
			// if it were json.RawMessage or another alias, it would print as json.RawMessage ... in the log
			// Because of this, strip down to just the comma-separated hex and look for that
			if strings.Contains(string(data), gobytes[len(`[]byte{`):len(gobytes)-1]) {
				t.Fatalf("log data contained byte-hex representation of bad string: %v, %v", string(data), badstring)
			}
		}
		return nil
	})
	require.NoError(t, err)
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
	testTask.Containers[1].Command = []string{"touch /data/readonly-fs || exit 42"}
	// make all the three containers non-essential to make sure all of the
	// container can be transitioned to running even one of them finished first
	testTask.Containers[1].Essential = false
	testTask.Containers[2].VolumesFrom = []apicontainer.VolumeFrom{{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[2].Command = []string{"touch /data/notreadonly-fs-1 || exit 42"}
	testTask.Containers[2].Essential = false
	testTask.Containers[3].VolumesFrom = []apicontainer.VolumeFrom{{SourceContainer: testTask.Containers[0].Name, ReadOnly: false}}
	testTask.Containers[3].Command = []string{"touch /data/notreadonly-fs-2 || exit 42"}
	testTask.Containers[3].Essential = false

	go taskEngine.AddTask(testTask)

	verifyTaskIsRunning(stateChangeEvents, testTask)
	taskEngine.(*DockerTaskEngine).stopContainer(testTask, testTask.Containers[0])

	verifyTaskIsStopped(stateChangeEvents, testTask)

	if testTask.Containers[1].GetKnownExitCode() == nil || *testTask.Containers[1].GetKnownExitCode() != 42 {
		t.Error("Didn't exit due to failure to touch ro fs as expected: ", testTask.Containers[1].GetKnownExitCode())
	}
	if testTask.Containers[2].GetKnownExitCode() == nil || *testTask.Containers[2].GetKnownExitCode() != 0 {
		t.Error("Couldn't touch with default of rw")
	}
	if testTask.Containers[3].GetKnownExitCode() == nil || *testTask.Containers[3].GetKnownExitCode() != 0 {
		t.Error("Couldn't touch with explicit rw")
	}
}

func createTestHostVolumeMountTask(tmpPath string) *apitask.Task {
	testTask := createTestTask("testHostVolumeMount")
	testTask.Volumes = []apitask.TaskVolume{{Name: "test-tmp", Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: tmpPath}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []apicontainer.MountPoint{{ContainerPath: "/host/tmp", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{`echo -n "hi" > /host/tmp/hello-from-container; if [[ "$(cat /host/tmp/test-file)" != "test-data" ]]; then exit 4; fi; exit 42`}
	return testTask
}

// This integ test is meant to validate the docker assumptions related to
// https://github.com/aws/amazon-ecs-agent/issues/261
// Namely, this test verifies that Docker does emit a 'die' event after an OOM
// event if the init dies.
// Note: Your kernel must support swap limits in order for this test to run.
// See https://github.com/docker/docker/pull/4251 about enabling swap limit
// support, or set MY_KERNEL_DOES_NOT_SUPPORT_SWAP_LIMIT to non-empty to skip
// this test.
func TestInitOOMEvent(t *testing.T) {
	if os.Getenv("MY_KERNEL_DOES_NOT_SUPPORT_SWAP_LIMIT") != "" {
		t.Skip("Skipped because MY_KERNEL_DOES_NOT_SUPPORT_SWAP_LIMIT")
	}
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testTask := createTestTask("oomtest")
	testTask.Containers[0].Memory = 20
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Command = []string{"sh", "-c", `x="a"; while true; do x=$x$x$x; done`}
	// should cause sh to get oomkilled as pid 1

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	event := <-stateChangeEvents
	require.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped, "Expected container to be STOPPED")

	// hold on to the container stopped event, will need to check exit code
	contEvent := event.(api.ContainerStateChange)

	verifyTaskStoppedStateChange(t, taskEngine)

	if contEvent.ExitCode == nil {
		t.Error("Expected exitcode to be set")
	} else if *contEvent.ExitCode != 137 {
		t.Errorf("Expected exitcode to be 137, not %v", *contEvent.ExitCode)
	}

	dockerVersion, err := taskEngine.Version()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dockerVersion, " 1.9.") {
		// Skip the final check for some versions of docker
		t.Logf("Docker version is 1.9.x (%s); not checking OOM reason", dockerVersion)
		return
	}
	if !strings.HasPrefix(contEvent.Reason, dockerapi.OutOfMemoryError{}.ErrorName()) {
		t.Errorf("Expected reason to have OOM error, was: %v", contEvent.Reason)
	}
}

// This integ test exercises the Docker "kill" facility, which exists to send
// signals to PID 1 inside a container.  Starting with Docker 1.7, a `kill`
// event was emitted by the Docker daemon on any `kill` invocation.
// Signals used in this test:
// SIGTERM - sent by Docker "stop" prior to SIGKILL (9)
// SIGUSR1 - used for the test as an arbitrary signal
func TestSignalEvent(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "signaltest"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Command = []string{
		"sh",
		"-c",
		fmt.Sprintf(`trap "exit 42" %d; trap "echo signal!" %d; while true; do sleep 1; done`, int(syscall.SIGTERM), int(syscall.SIGUSR1)),
	}

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	// Signal the container now
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	err := client.ContainerKill(context.TODO(), cid, "SIGUSR1")
	require.NoError(t, err, "Could not signal container", err)

	// Verify the container has not stopped
	time.Sleep(2 * time.Second)
check_events:
	for {
		select {
		case event := <-stateChangeEvents:
			if event.GetEventType() == statechange.ContainerEvent {
				contEvent := event.(api.ContainerStateChange)
				if contEvent.TaskArn != testTask.Arn {
					continue
				}
				t.Fatalf("Expected no events; got " + contEvent.Status.String())
			}
		default:
			break check_events
		}
	}

	// Stop the container now
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	if testTask.Containers[0].GetKnownExitCode() == nil || *testTask.Containers[0].GetKnownExitCode() != 42 {
		t.Error("Wrong exit code; file probably wasn't present")
	}
}

// TestDockerStopTimeout tests the container was killed after ECS_CONTAINER_STOP_TIMEOUT
func TestDockerStopTimeout(t *testing.T) {
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", testDockerStopTimeout.String())
	defer os.Unsetenv("ECS_CONTAINER_STOP_TIMEOUT")
	cfg := defaultTestConfigIntegTest()

	taskEngine, _, _ := setup(cfg, nil, t)

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)

	if dockerTaskEngine.cfg.DockerStopTimeout != testDockerStopTimeout {
		t.Errorf("Expect the docker stop timeout read from environment variable when ECS_CONTAINER_STOP_TIMEOUT is set, %v", dockerTaskEngine.cfg.DockerStopTimeout)
	}
	testTask := createTestTask("TestDockerStopTimeout")
	testTask.Containers[0].Command = []string{"sh", "-c", "trap 'echo hello' SIGTERM; while true; do echo `date +%T`; sleep 1s; done;"}
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Name = "test-docker-timeout"

	go dockerTaskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	startTime := ttime.Now()
	dockerTaskEngine.stopContainer(testTask, testTask.Containers[0])

	verifyContainerStoppedStateChange(t, taskEngine)

	if ttime.Since(startTime) < testDockerStopTimeout {
		t.Errorf("Container stopped before the timeout: %v", ttime.Since(startTime))
	}
	if ttime.Since(startTime) > testDockerStopTimeout+1*time.Second {
		t.Errorf("Container should have stopped eariler, but stopped after %v", ttime.Since(startTime))
	}
}

func TestStartStopWithSecurityOptionNoNewPrivileges(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	testArn := "testSecurityOptionNoNewPrivileges"
	testTask := createTestTask(testArn)
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{HostConfig: aws.String(`{"SecurityOpt":["no-new-privileges"]}`)}

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	// Kill the existing container now
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestTaskLevelVolume(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	stateChangeEvents := taskEngine.StateChangeEvents()

	testTask, tmpDirectory, err := createVolumeTask("task", "TestTaskLevelVolume", "TestTaskLevelVolume", true)
	defer os.Remove(tmpDirectory)
	require.NoError(t, err, "creating test task failed")

	go taskEngine.AddTask(testTask)

	verifyTaskIsRunning(stateChangeEvents, testTask)
	verifyTaskIsStopped(stateChangeEvents, testTask)
	require.Equal(t, *testTask.Containers[0].GetKnownExitCode(), 0)
	require.NotEqual(t, testTask.ResourcesMapUnsafe["dockerVolume"][0].(*taskresourcevolume.VolumeResource).VolumeConfig.Source(), "TestTaskLevelVolume", "task volume name is the same as specified in task definition")

	client := taskEngine.(*DockerTaskEngine).client
	client.RemoveVolume(context.TODO(), "TestTaskLevelVolume", 5*time.Second)
}

func TestSwapConfigurationTask(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testArn := "TestSwapMemory"
	testTask := createTestTask(testArn)
	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{HostConfig: aws.String(`{"MemorySwap":314572800, "MemorySwappiness":90}`)}

	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	state, _ := client.ContainerInspect(ctx, cid)
	require.EqualValues(t, 314572800, state.HostConfig.MemorySwap)
	require.EqualValues(t, 90, *state.HostConfig.MemorySwappiness)

	// Kill the existing container now
	taskUpdate := createTestTask(testArn)
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestGPUAssociationTask(t *testing.T) {
	gpuSupportEnabled := utils.ParseBool(getGPUEnvVar(gpuConfigFilePath)["ECS_ENABLE_GPU_SUPPORT"], false)

	iid, _ := ec2.NewEC2MetadataClient(nil).InstanceIdentityDocument()

	var isGPUInstanceType bool

	for _, gpuInstanceType := range TestGPUInstanceType {
		if strings.HasPrefix(iid.InstanceType, gpuInstanceType) {
			isGPUInstanceType = true
			break
		}
	}

	if !(isGPUInstanceType && gpuSupportEnabled) {
		t.Skip("Skipped because either ECS_ENABLE_GPU_SUPPORT is not set to true or the instance type is not a supported GPU instance type")
	}

	cfg := defaultTestConfigIntegTest()
	cfg.GPUSupportEnabled = true
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	stateChangeEvents := taskEngine.StateChangeEvents()
	testTask := createTestGPUTask()
	container := testTask.Containers[0]

	go taskEngine.AddTask(testTask)

	verifyTaskIsRunning(stateChangeEvents, testTask)
	require.Equal(t, []string{"0"}, container.GPUIDs)
	require.Equal(t, "0", container.Environment[apitask.NvidiaVisibleDevicesEnvVar])

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	state, _ := client.ContainerInspect(ctx, cid)
	require.Equal(t, testTask.NvidiaRuntime, state.HostConfig.Runtime)
	require.Contains(t, state.Config.Env, "NVIDIA_VISIBLE_DEVICES=0")

	taskUpdate := createTestGPUTask()
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)
	verifyTaskIsStopped(stateChangeEvents, testTask)
}

func TestPerContainerStopTimeout(t *testing.T) {
	// set the global stop timemout, but this should be ignored since the per container value is set
	globalStopContainerTimeout := 1000 * time.Second
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", globalStopContainerTimeout.String())
	defer os.Unsetenv("ECS_CONTAINER_STOP_TIMEOUT")
	cfg := defaultTestConfigIntegTest()

	taskEngine, _, _ := setup(cfg, nil, t)

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)

	if dockerTaskEngine.cfg.DockerStopTimeout != globalStopContainerTimeout {
		t.Errorf("Expect ECS_CONTAINER_STOP_TIMEOUT to be set to , %v", dockerTaskEngine.cfg.DockerStopTimeout)
	}

	testTask := createTestTask("TestDockerStopTimeout")
	testTask.Containers[0].Command = []string{"sh", "-c", "trap 'echo hello' SIGTERM; while true; do echo `date +%T`; sleep 1s; done;"}
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Name = "test-docker-timeout"
	testTask.Containers[0].StopTimeout = uint(testDockerStopTimeout.Seconds())

	go dockerTaskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	startTime := ttime.Now()
	dockerTaskEngine.stopContainer(testTask, testTask.Containers[0])

	verifyContainerStoppedStateChange(t, taskEngine)

	if ttime.Since(startTime) < testDockerStopTimeout {
		t.Errorf("Container stopped before the timeout: %v", ttime.Since(startTime))
	}
	if ttime.Since(startTime) > testDockerStopTimeout+1*time.Second {
		t.Errorf("Container should have stopped eariler, but stopped after %v", ttime.Since(startTime))
	}
}

func TestMemoryOverCommit(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	memoryReservation := 50

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	testArn := "TestMemoryOverCommit"
	testTask := createTestTask(testArn)

	testTask.Containers[0].DockerConfig = apicontainer.DockerConfig{HostConfig: aws.String(`{
	"MemoryReservation": 52428800 }`)}

	go taskEngine.AddTask(testTask)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	state, _ := client.ContainerInspect(ctx, cid)

	require.EqualValues(t, memoryReservation*1024*1024, state.HostConfig.MemoryReservation)

	// Kill the existing container now
	testUpdate := createTestTask(testArn)
	testUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(testUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

// TestNetworkModeHost tests the container network can be configured
// as bridge mode in task definition
func TestNetworkModeHost(t *testing.T) {
	testNetworkMode(t, "bridge")
}

// TestNetworkModeBridge tests the container network can be configured
// as host mode in task definition
func TestNetworkModeBridge(t *testing.T) {
	testNetworkMode(t, "host")
}

func TestFluentdTag(t *testing.T) {
	// Skipping the test for arm as they do not have official support for Arm images
	if runtime.GOARCH == "arm64" {
		t.Skip("Skipping test, unsupported image for arm64")
	}

	logdir := os.TempDir()
	logdir = path.Join(logdir, "ftslog")
	defer os.RemoveAll(logdir)

	os.Setenv("ECS_AVAILABLE_LOGGING_DRIVERS", `["fluentd"]`)
	defer os.Unsetenv("ECS_AVAILABLE_LOGGING_DRIVERS")

	taskEngine, _, _ := setupWithDefaultConfig(t)

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint),
		sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	// start Fluentd driver task
	testTaskFleuntdDriver := createTestTask("testFleuntdDriver")
	testTaskFleuntdDriver.Volumes = []apitask.TaskVolume{{Name: "logs", Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: "/tmp"}}}
	testTaskFleuntdDriver.Containers[0].Image = testFluentdImage
	testTaskFleuntdDriver.Containers[0].MountPoints = []apicontainer.MountPoint{{ContainerPath: "/fluentd/log",
		SourceVolume: "logs"}}
	testTaskFleuntdDriver.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: 24224, HostPort: 24224}}
	go taskEngine.AddTask(testTaskFleuntdDriver)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	// Sleep before starting the test task so that fluentd driver is setup
	time.Sleep(30 * time.Second)

	// start fluentd log task
	testTaskFluentdLogTag := createTestTask("testFleuntdTag")
	testTaskFluentdLogTag.Containers[0].Command = []string{"sh", "-c", `echo hello, this is fluentd integration test`}
	testTaskFluentdLogTag.Containers[0].Image = testUbuntuImage
	testTaskFluentdLogTag.Containers[0].DockerConfig = apicontainer.DockerConfig{
		HostConfig: aws.String(`{"LogConfig": {
		"Type": "fluentd",
		"Config": {
			"fluentd-address":"0.0.0.0:24224",
			"tag":"ecs.{{.Name}}.{{.FullID}}"
		}
	}}`)}

	go taskEngine.AddTask(testTaskFluentdLogTag)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTaskFluentdLogTag.Arn)
	cid := containerMap[testTaskFluentdLogTag.Containers[0].Name].DockerID
	state, _ := client.ContainerInspect(ctx, cid)

	// Kill the fluentd driver task
	testUpdate := createTestTask("testFleuntdDriver")
	testUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(testUpdate)
	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)

	logTag := fmt.Sprintf("ecs.%v.%v", strings.Replace(state.Name,
		"/", "", 1), cid)

	// Verify the log file existed and also the content contains the expected format
	err = utils.SearchStrInDir(logdir, "ecsfts", "hello, this is fluentd integration test")
	require.NoError(t, err, "failed to find the content in the fluent log file")

	err = utils.SearchStrInDir(logdir, "ecsfts", logTag)
	require.NoError(t, err, "failed to find the log tag specified in the task definition")
}

func createTestExecCommandAgentTask(taskId, containerName string, sleepFor time.Duration) *apitask.Task {
	testTask := createTestTask("arn:aws:ecs:us-west-2:1234567890:task/" + taskId)
	testTask.ExecCommandAgentEnabled = true
	testTask.Containers[0].Name = containerName
	testTask.Containers[0].Image = testExecCommandAgentImage
	testTask.Containers[0].Command = []string{"/sleep", "-time=" + sleepFor.String()}
	return testTask
}

// TODO: [ecs-exec] Enable this test when exec command agent is in place on the host
func TestExecCommandAgent(t *testing.T) {
	t.Skip("Skipping until exec command agent is in place on the host")
	const (
		testTaskId        = "exec-command-agent-test-task"
		testContainerName = "exec-command-agent-test-container"
		sleepFor          = time.Second * 5
	)
	taskEngine, done, _ := setupWithDefaultConfig(t)
	stateChangeEvents := taskEngine.StateChangeEvents()
	defer done()

	testTask := createTestExecCommandAgentTask(testTaskId, testContainerName, sleepFor)

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	client, err := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Creating go docker client failed")

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	inspectState, _ := client.ContainerInspect(ctx, cid)

	expectedMounts := []struct {
		source   string
		dest     string
		readOnly bool
	}{
		{
			source:   filepath.Join(apitask.ExecCommandAgentHostBinDir, apitask.ExecCommandAgentBinName),
			dest:     filepath.Join(apitask.ExecCommandAgentContainerBinDir, apitask.ExecCommandAgentBinName),
			readOnly: true,
		},
		{
			source:   filepath.Join(apitask.ExecCommandAgentHostBinDir, apitask.ExecCommandAgentSessionWorkerBinName),
			dest:     filepath.Join(apitask.ExecCommandAgentContainerBinDir, apitask.ExecCommandAgentSessionWorkerBinName),
			readOnly: true,
		},
		{
			source:   apitask.ExecCommandAgentHostCertFile,
			dest:     apitask.ExecCommandAgentContainerCertFile,
			readOnly: true,
		},
		{
			source:   filepath.Join(apitask.ExecCommandAgentHostLogDir, testTaskId, testContainerName),
			dest:     apitask.ExecCommandAgentContainerLogDir,
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

	require.Equal(t, len(expectedMounts), len(inspectState.Mounts), "Wrong number of bind mounts detected in container (%s)", testContainerName)

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

}

func TestDockerExecAPI(t *testing.T) {
	testTimeout := 1 * time.Minute
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	taskArn := "testDockerExec"
	testTask := createTestTask(taskArn)

	A := createTestContainerWithImageAndName(baseImageForOS, "A")

	A.EntryPoint = &entryPointForOS
	A.Command = []string{"sleep 30"}
	A.Essential = true

	testTask.Containers = []*apicontainer.Container{
		A,
	}
	execConfig := types.ExecConfig{
		Detach: true,
		Cmd:    []string{"ls"},
	}
	go taskEngine.AddTask(testTask)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	finished := make(chan interface{})
	go func() {
		// Both containers should start
		verifyContainerRunningStateChange(t, taskEngine)
		verifyTaskIsRunning(stateChangeEvents, testTask)

		containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
		dockerID := containerMap[testTask.Containers[0].Name].DockerID

		//Create Exec object on the host
		execContainerOut, err := taskEngine.(*DockerTaskEngine).client.CreateContainerExec(ctx, dockerID, execConfig, dockerclient.ContainerExecCreateTimeout)
		require.NoError(t, err)
		require.NotNil(t, execContainerOut)

		//Start the above Exec process on the host
		err1 := taskEngine.(*DockerTaskEngine).client.StartContainerExec(ctx, execContainerOut.ID, dockerclient.ContainerExecStartTimeout)
		require.NoError(t, err1)

		//Inspect the above Exec process on the host to check if the exit code is 0 which indicates successful run of the command
		execContInspectOut, err := taskEngine.(*DockerTaskEngine).client.InspectContainerExec(ctx, execContainerOut.ID, dockerclient.ContainerExecInspectTimeout)
		require.NoError(t, err)
		require.Equal(t, dockerID, execContInspectOut.ContainerID)
		require.Equal(t, 0, execContInspectOut.ExitCode)

		// Task should stop
		verifyContainerStoppedStateChange(t, taskEngine)
		verifyTaskIsStopped(stateChangeEvents, testTask)
		close(finished)
	}()

	waitFinished(t, finished, testTimeout)
}
