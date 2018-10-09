// +build !windows,integration

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
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRegistryHost      = "127.0.0.1:51670"
	testBusyboxImage      = testRegistryHost + "/busybox:latest"
	testAuthRegistryHost  = "127.0.0.1:51671"
	testAuthRegistryImage = "127.0.0.1:51671/amazon/amazon-ecs-netkitten:latest"
	testVolumeImage       = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
	testPIDNamespaceImage = "127.0.0.1:51670/amazon/amazon-ecs-pid-namespace-test:latest"
	testIPCNamespaceImage = "127.0.0.1:51670/amazon/amazon-ecs-ipc-namespace-test:latest"
	testAuthUser          = "user"
	testAuthPass          = "swordfish"

	// Search for the running process
	testPIDNamespaceCommand         = "if [ `ps ax | grep pidNamespaceTest | grep -vc grep` -gt 0 ]; then exit 1; else exit 2; fi;"
	testPIDNamespaceProcessFound    = 1
	testPIDNamespaceProcessNotFound = 2

	// Search for a semaphore with a Key of 5 (created in ipcNamespaceTest.c
	// running in testIPCNamespaceImage)
	testIPCNamespaceCommand          = "if [ `ipcs -s | awk '$1+0 == 5' | wc -l` -gt 0 ]; then exit 1; else exit 2; fi;"
	testIPCNamespaceResourceFound    = 1
	testIPCNamespaceResourceNotFound = 2
)

var (
	endpoint = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)
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

// A map that stores statusChangeEvents for both Tasks and Containers
// Organized first by EventType (Task or Container),
// then by StatusType (i.e. RUNNING, STOPPED, etc)
// then by Task/Container identifying string (TaskARN or ContainerName)
//                   EventType
//                  /         \
//          TaskEvent         ContainerEvent
//        /          \           /        \
//    RUNNING      STOPPED   RUNNING      STOPPED
//    /    \        /    \      |             |
//  ARN1  ARN2    ARN3  ARN4  ARN:Cont1    ARN:Cont2
type EventSet map[statechange.EventType]statusToName

// Type definition for mapping a Status to a TaskARN/ContainerName
type statusToName map[string]nameSet

// Type definition for a generic set implemented as a map
type nameSet map[string]bool

// Holds the Events Map described above with a RW mutex
type TestEvents struct {
	RecordedEvents    EventSet
	StateChangeEvents <-chan statechange.Event
}

// Initializes the TestEvents using the TaskEngine. Abstracts the overhead required to set up
// collecting TaskEngine stateChangeEvents.
// We must use the Golang assert library and NOT the require library to ensure the Go routine is
// stopped at the end of our tests
func InitEventCollection(taskEngine TaskEngine) *TestEvents {
	stateChangeEvents := taskEngine.StateChangeEvents()
	recordedEvents := make(EventSet)
	testEvents := &TestEvents{
		RecordedEvents:    recordedEvents,
		StateChangeEvents: stateChangeEvents,
	}
	return testEvents
}

// This method queries the TestEvents struct to check a Task Status.
// This method will block if there are no more stateChangeEvents from the DockerTaskEngine but is expected
func VerifyTaskStatus(status apitaskstatus.TaskStatus, taskARN string, testEvents *TestEvents, t *testing.T) error {
	for {
		if _, found := testEvents.RecordedEvents[statechange.TaskEvent][status.String()][taskARN]; found {
			return nil
		}
		event := <-testEvents.StateChangeEvents
		RecordEvent(testEvents, event)
	}
}

// This method queries the TestEvents struct to check a Task Status.
// This method will block if there are no more stateChangeEvents from the DockerTaskEngine but is expected
func VerifyContainerStatus(status apicontainerstatus.ContainerStatus, ARNcontName string, testEvents *TestEvents, t *testing.T) error {
	for {
		if _, found := testEvents.RecordedEvents[statechange.ContainerEvent][status.String()][ARNcontName]; found {
			return nil
		}
		event := <-testEvents.StateChangeEvents
		RecordEvent(testEvents, event)
	}
}

// Will record the event that was just collected into the TestEvents struct's RecordedEvents map
func RecordEvent(testEvents *TestEvents, event statechange.Event) {
	switch event.GetEventType() {
	case statechange.TaskEvent:
		taskEvent := event.(api.TaskStateChange)
		if _, exists := testEvents.RecordedEvents[statechange.TaskEvent]; !exists {
			testEvents.RecordedEvents[statechange.TaskEvent] = make(statusToName)
		}
		if _, exists := testEvents.RecordedEvents[statechange.TaskEvent][taskEvent.Status.String()]; !exists {
			testEvents.RecordedEvents[statechange.TaskEvent][taskEvent.Status.String()] = make(map[string]bool)
		}
		testEvents.RecordedEvents[statechange.TaskEvent][taskEvent.Status.String()][taskEvent.TaskARN] = true
	case statechange.ContainerEvent:
		containerEvent := event.(api.ContainerStateChange)
		if _, exists := testEvents.RecordedEvents[statechange.ContainerEvent]; !exists {
			testEvents.RecordedEvents[statechange.ContainerEvent] = make(statusToName)
		}
		if _, exists := testEvents.RecordedEvents[statechange.ContainerEvent][containerEvent.Status.String()]; !exists {
			testEvents.RecordedEvents[statechange.ContainerEvent][containerEvent.Status.String()] = make(map[string]bool)
		}
		testEvents.RecordedEvents[statechange.ContainerEvent][containerEvent.Status.String()][containerEvent.TaskArn+":"+containerEvent.ContainerName] = true
	}
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
	assert.NoError(t, err, "Not verified task running")
	assert.Equal(t, "host", testTask.PIDMode)
	//Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container1", testEvents, t)
	assert.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container2", testEvents, t)
	assert.NoError(t, err)

	//Manually stop container0
	cont0, _ := testTask.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTask, cont0)

	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTask.Arn, testEvents, t)
	assert.NoError(t, err)

	cont1, _ := testTask.ContainerByName("container1")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont1.GetKnownExitCode()), "container1 could not see NamespaceTest process")
	cont2, _ := testTask.ContainerByName("container2")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont2.GetKnownExitCode()), "container2 could not see NamespaceTest process")
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
	assert.NoError(t, err)
	assert.Equal(t, "host", testTaskWithProcess.PIDMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container1", testEvents, t)
	assert.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container2", testEvents, t)
	assert.NoError(t, err)
	cont1, _ := testTaskWithProcess.ContainerByName("container1")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont1.GetKnownExitCode()), "container1 could not see NamespaceTest process")
	cont2, _ := testTaskWithProcess.ContainerByName("container2")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont2.GetKnownExitCode()), "container2 could not see NamespaceTest process")

	// Run Task without Process attached
	go taskEngine.AddTask(testTaskWithoutProcess)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutProcess.Arn, testEvents, t)
	assert.NoError(t, err)
	assert.Equal(t, "host", testTaskWithoutProcess.PIDMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutProcess.Arn+":container0", testEvents, t)
	assert.NoError(t, err)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutProcess.Arn, testEvents, t)
	assert.NoError(t, err)
	cont0, _ := testTaskWithoutProcess.ContainerByName("container0")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont0.GetKnownExitCode()), "container0 could not see NamespaceTest process, but should be able to.")

	// Test is complete, can stop container with process
	cont, _ := testTaskWithProcess.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithProcess, cont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithProcess.Arn, testEvents, t)
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, "task", testTaskWithProcess.PIDMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container1", testEvents, t)
	assert.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithProcess.Arn+":container2", testEvents, t)
	assert.NoError(t, err)
	cont1, _ := testTaskWithProcess.ContainerByName("container1")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont1.GetKnownExitCode()), "container1 could not see NamespaceTest process")
	cont2, _ := testTaskWithProcess.ContainerByName("container2")
	assert.Equal(t, testPIDNamespaceProcessFound, *(cont2.GetKnownExitCode()), "container2 could not see NamespaceTest process")

	// Run Task without Process attached
	go taskEngine.AddTask(testTaskWithoutProcess)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutProcess.Arn, testEvents, t)
	assert.NoError(t, err)
	assert.Equal(t, "task", testTaskWithoutProcess.PIDMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutProcess.Arn+":container0", testEvents, t)
	assert.NoError(t, err)

	// Manually stop the Pause container and verify if TaskWithoutProcess has stopped
	pauseCont, _ := testTaskWithoutProcess.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithoutProcess, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutProcess.Arn, testEvents, t)
	assert.NoError(t, err)

	cont0, _ := testTaskWithoutProcess.ContainerByName("container0")
	assert.Equal(t, testPIDNamespaceProcessNotFound, *(cont0.GetKnownExitCode()), "container0 could see NamespaceTest process, but shouldn't be able to.")

	// Test is complete, can stop container with process
	cont, _ := testTaskWithProcess.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithProcess, cont)
	// Manually stop the Pause container and verify if TaskWithProcess has stopped
	pauseCont, _ = testTaskWithProcess.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithProcess, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithProcess.Arn, testEvents, t)
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, "host", testTask.IPCMode)

	//Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container1", testEvents, t)
	assert.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTask.Arn+":container2", testEvents, t)
	assert.NoError(t, err)
	//Manually stop container0
	cont0, _ := testTask.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTask, cont0)

	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTask.Arn, testEvents, t)
	assert.NoError(t, err)
	cont1, _ := testTask.ContainerByName("container1")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont1.GetKnownExitCode()), "container1 could not see IPC Semaphore")
	cont2, _ := testTask.ContainerByName("container2")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont2.GetKnownExitCode()), "container2 could not see IPC Semaphore")
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
	assert.NoError(t, err)
	assert.Equal(t, "host", testTaskWithResource.IPCMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container1", testEvents, t)
	assert.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container2", testEvents, t)
	assert.NoError(t, err)
	cont1, _ := testTaskWithResource.ContainerByName("container1")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont1.GetKnownExitCode()), "container1 could not see IPC Semaphore")
	cont2, _ := testTaskWithResource.ContainerByName("container2")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont2.GetKnownExitCode()), "container2 could not see IPC Semaphore")

	// Run Task with IPC Semaphore attached
	go taskEngine.AddTask(testTaskWithoutResource)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutResource.Arn, testEvents, t)
	assert.NoError(t, err)
	assert.Equal(t, "host", testTaskWithoutResource.IPCMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutResource.Arn+":container0", testEvents, t)
	assert.NoError(t, err)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutResource.Arn, testEvents, t)
	assert.NoError(t, err)
	cont0, _ := testTaskWithoutResource.ContainerByName("container0")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont0.GetKnownExitCode()), "container0 could not see IPC Semaphore, but should be able to.")

	// Test is complete, can stop container with process
	cont, _ := testTaskWithResource.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithResource, cont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithResource.Arn, testEvents, t)
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, "task", testTaskWithResource.IPCMode)
	// Wait for container1 and container2 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container1", testEvents, t)
	assert.NoError(t, err)
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithResource.Arn+":container2", testEvents, t)
	assert.NoError(t, err)
	cont1, _ := testTaskWithResource.ContainerByName("container1")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont1.GetKnownExitCode()), "container1 could not see IPC Semaphore")
	cont2, _ := testTaskWithResource.ContainerByName("container2")
	assert.Equal(t, testIPCNamespaceResourceFound, *(cont2.GetKnownExitCode()), "container2 could not see IPC Semaphore")

	// Run Task without Semaphore
	go taskEngine.AddTask(testTaskWithoutResource)
	err = VerifyTaskStatus(apitaskstatus.TaskRunning, testTaskWithoutResource.Arn, testEvents, t)
	assert.NoError(t, err)
	assert.Equal(t, "task", testTaskWithoutResource.IPCMode)
	// Wait for container0 to go down
	err = VerifyContainerStatus(apicontainerstatus.ContainerStopped, testTaskWithoutResource.Arn+":container0", testEvents, t)
	assert.NoError(t, err)

	// Manually stop the Pause container and verify if TaskWithoutResource has stopped
	pauseCont, _ := testTaskWithoutResource.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithoutResource, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithoutResource.Arn, testEvents, t)
	assert.NoError(t, err)

	cont0, _ := testTaskWithoutResource.ContainerByName("container0")
	assert.Equal(t, testIPCNamespaceResourceNotFound, *(cont0.GetKnownExitCode()), "container0 could see IPC Semaphore, but shouldn't be able to.")

	// Test is complete, can stop container with semaphore
	cont, _ := testTaskWithResource.ContainerByName("container0")
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithResource, cont)
	// Manually stop the Pause container and verify if TaskWithResource has stopped
	pauseCont, _ = testTaskWithResource.ContainerByName(apitask.NamespacePauseContainerName)
	taskEngine.(*DockerTaskEngine).stopContainer(testTaskWithResource, pauseCont)
	err = VerifyTaskStatus(apitaskstatus.TaskStopped, testTaskWithResource.Arn, testEvents, t)
	assert.NoError(t, err)
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
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent}

	// Port not forwarded; verify we can't access it
	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	require.NoError(t, err)

	time.Sleep(waitForDockerDuration) // wait for Docker
	_, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", localhost, containerPortOne), dialTimeout)
	assert.Error(t, err, "Did not expect to be able to dial %s:%d but didn't get error", localhost, containerPortOne)

	// Kill the existing container now to make the test run more quickly.
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	client, _ := docker.NewClient(endpoint)
	err = client.KillContainer(docker.KillContainerOptions{ID: cid})
	if err != nil {
		t.Error("Could not kill container", err)
	}

	verifyTaskIsStopped(stateChangeEvents, testTask)

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent}
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortOne}}

	taskEngine.AddTask(testTask)

	err = verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, containerPortOne), 10, dialTimeout)
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
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
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
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent + "1"}
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortOne}}
	testTask.Containers[0].Essential = false
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent + "2"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortTwo}}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, containerPortOne), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container 1 " + err.Error())
	}
	t.Log("Dialed first container")
	response, _ := ioutil.ReadAll(conn)
	if string(response) != serverContent+"1" {
		t.Error("Got response: " + string(response) + " instead of" + serverContent + "1")
	}
	t.Log("Read first container")
	conn, err = dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, containerPortTwo), 10, dialTimeout)
	if err != nil {
		t.Fatal("Error dialing simple container 2 " + err.Error())
	}
	t.Log("Dialed second container")
	response, _ = ioutil.ReadAll(conn)
	if string(response) != serverContent+"2" {
		t.Error("Got response: " + string(response) + " instead of" + serverContent + "2")
	}
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
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings

	verifyTaskRunningStateChange(t, taskEngine)

	if len(portBindings) != 1 {
		t.Error("PortBindings was not set; should have been len 1", portBindings)
	}
	var bindingForcontainerPortOne uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == containerPortOne {
			bindingForcontainerPortOne = binding.HostPort
		}
	}
	if bindingForcontainerPortOne == 0 {
		t.Errorf("Could not find the port mapping for %d!", containerPortOne)
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
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestMultipleDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "-serve", serverContent, `-loop`}
	// No HostPort or 0 hostport; docker should pick two ports for us
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne}, {ContainerPort: containerPortOne, HostPort: 0}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings

	verifyTaskRunningStateChange(t, taskEngine)

	if len(portBindings) != 2 {
		t.Error("Could not bind to two ports from one container port", portBindings)
	}
	var bindingForcontainerPortOne_1 uint16
	var bindingForcontainerPortOne_2 uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == containerPortOne {
			if bindingForcontainerPortOne_1 == 0 {
				bindingForcontainerPortOne_1 = binding.HostPort
			} else {
				bindingForcontainerPortOne_2 = binding.HostPort
			}
		}
	}
	if bindingForcontainerPortOne_1 == 0 {
		t.Errorf("Could not find the port mapping for %d!", containerPortOne)
	}
	if bindingForcontainerPortOne_2 == 0 {
		t.Errorf("Could not find the port mapping for %d!", containerPortOne)
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
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

// TestLinking ensures that container linking does allow networking to go
// through to a linked container.  this test specifically starts a server that
// prints "hello linker" and then links a container that proxies that data to
// a publicly exposed port, where the tests reads it
func TestLinking(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	testTask := createTestTask("TestLinking")
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[0].Command = []string{"-l=80", "-serve", "hello linker"}
	testTask.Containers[0].Name = "linkee"
	testTask.Containers[1].Command = []string{fmt.Sprintf("-l=%d", containerPortOne), "linkee_alias:80"}
	testTask.Containers[1].Links = []string{"linkee:linkee_alias"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: containerPortOne, HostPort: containerPortOne}}

	stateChangeEvents := taskEngine.StateChangeEvents()

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	var response []byte
	for i := 0; i < 10; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", localhost, containerPortOne), dialTimeout)
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
		time.Sleep(100 * time.Millisecond)
	}
	if string(response) != "hello linker" {
		t.Error("Got response: " + string(response) + " instead of 'hello linker'")
	}

	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyTaskIsStopped(stateChangeEvents, testTask)
}

func TestDockerCfgAuth(t *testing.T) {
	authString := base64.StdEncoding.EncodeToString([]byte(testAuthUser + ":" + testAuthPass))
	cfg := defaultTestConfigIntegTest()
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` + testAuthRegistryHost + `/v1/":{"auth":"` + authString + `"}}`))
	cfg.EngineAuthType = "dockercfg"

	removeImage(t, testAuthRegistryImage)
	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()
	defer func() {
		cfg.EngineAuthData = config.NewSensitiveRawMessage(nil)
		cfg.EngineAuthType = ""
	}()

	testTask := createTestTask("testDockerCfgAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	taskUpdate := createTestTask("testDockerCfgAuth")
	taskUpdate.Containers[0].Image = testAuthRegistryImage
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
}

func TestDockerAuth(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` + testAuthRegistryHost + `":{"username":"` + testAuthUser + `","password":"` + testAuthPass + `"}}`))
	cfg.EngineAuthType = "docker"
	defer func() {
		cfg.EngineAuthData = config.NewSensitiveRawMessage(nil)
		cfg.EngineAuthType = ""
	}()

	taskEngine, done, _ := setup(cfg, nil, t)
	defer done()
	removeImage(t, testAuthRegistryImage)

	testTask := createTestTask("testDockerAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	go taskEngine.AddTask(testTask)

	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	taskUpdate := createTestTask("testDockerAuth")
	taskUpdate.Containers[0].Image = testAuthRegistryImage
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(taskUpdate)

	verifyContainerStoppedStateChange(t, taskEngine)
	verifyTaskStoppedStateChange(t, taskEngine)
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
	testTask.Containers[1].Command = []string{"cat /data/test-file | nc -l -p 80"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: 80, HostPort: containerPortOne}}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(stateChangeEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(waitForDockerDuration) // wait for Docker
	conn, err := dialWithRetries("tcp", fmt.Sprintf("%s:%d", localhost, containerPortOne), 10, dialTimeout)
	if err != nil {
		t.Error("Could not dial listening container" + err.Error())
	}

	response, err := ioutil.ReadAll(conn)
	if err != nil {
		t.Error(err)
	}
	if strings.TrimSpace(string(response)) != "test" {
		t.Error("Got response: " + strings.TrimSpace(string(response)) + " instead of 'test'")
	}

	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyTaskIsStopped(stateChangeEvents, testTask)
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
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped, "Expected container to be STOPPED")

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

	testTask := createTestTask("signaltest")
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
	client, _ := docker.NewClient(endpoint)
	err := client.KillContainer(docker.KillContainerOptions{ID: cid, Signal: docker.Signal(int(syscall.SIGUSR1))})
	if err != nil {
		t.Error("Could not signal container", err)
	}

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
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(apitaskstatus.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

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
	assert.Equal(t, *testTask.Containers[0].GetKnownExitCode(), 0)
	assert.NotEqual(t, testTask.ResourcesMapUnsafe["dockerVolume"][0].(*taskresourcevolume.VolumeResource).VolumeConfig.Source(), "TestTaskLevelVolume", "task volume name is the same as specified in task definition")

	client := taskEngine.(*DockerTaskEngine).client
	client.RemoveVolume(context.TODO(), "TestTaskLevelVolume", 5*time.Second)
}
