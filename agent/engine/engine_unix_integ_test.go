//go:build !windows && integration
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
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

const (
	testRegistryHost = "127.0.0.1:51670"
	testBusyboxImage = testRegistryHost + "/busybox:latest"
	testVolumeImage  = testRegistryHost + "/amazon/amazon-ecs-volumes-test:latest"
	testFluentdImage = testRegistryHost + "/amazon/fluentd:latest"
)

var (
	endpoint            = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)
	TestGPUInstanceType = []string{"p2", "p3", "g3", "g4dn", "p4d"}
)

// Starting from Docker version 20.10.6, a behavioral change was introduced in docker container port bindings,
// where both IPv4 and IPv6 port bindings will be exposed.
// To mitigate this issue, Agent introduced an environment variable ECS_EXCLUDE_IPV6_PORTBINDING,
// which is true by default in the [PR#3025](https://github.com/aws/amazon-ecs-agent/pull/3025).
// However, the PR does not modify port bindings in the container state change object, it only filters out IPv6 port
// bindings while building the container state change payload. Thus the invalid IPv6 port bindings still exists
// in ContainerStateChange, and need to be filtered out in this integration test.
//
// The getValidPortBinding function and the ECS_EXCLUDE_IPV6_PORTBINDING environment variable should be removed once
// the IPv6 issue is resolved by Docker and is fully supported by ECS.
func getValidPortBinding(portBindings []apicontainer.PortBinding) []apicontainer.PortBinding {
	validPortBindings := []apicontainer.PortBinding{}
	for _, binding := range portBindings {
		if binding.BindIP == "::" {
			seelog.Debugf("Exclude IPv6 port binding %v", binding)
			continue
		}
		validPortBindings = append(validPortBindings, binding)
	}
	return validPortBindings
}

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
	imageDigest := "public.ecr.aws/amazonlinux/amazonlinux@sha256:1b6599b4846a765106350130125e2480f6c1cb7791df0ce3e59410362f311259"
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
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(port2), HostPort: port2}}

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
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(port1), HostPort: port1}}
	testTask.Containers[0].Essential = false
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Command = []string{fmt.Sprintf("-l=%d", port1), "-serve", serverContent + "2"}
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(port1), HostPort: port2}}

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
// docker daemon and verifies that the port is reported in the state-change.
func TestDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	stateChangeEvents := taskEngine.StateChangeEvents()

	testArn := "testDynamicPortForward"
	testTask := createTestTask(testArn)
	port := getUnassignedPort()
	testTask.Containers[0].Command = []string{fmt.Sprintf("-l=%d", port), "-serve", serverContent}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(port)}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	require.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings
	// See comments on the getValidPortBinding() function for why ports need to be filtered.
	validPortBindings := getValidPortBinding(portBindings)

	verifyTaskRunningStateChange(t, taskEngine)

	if len(validPortBindings) != 1 {
		t.Error("PortBindings was not set; should have been len 1", portBindings)
	}
	var bindingForcontainerPortOne uint16
	for _, binding := range validPortBindings {
		if port == aws.Uint16Value(binding.ContainerPort) {
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
	testTask.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(port)}, {ContainerPort: aws.Uint16(port), HostPort: 0}}

	go taskEngine.AddTask(testTask)

	event := <-stateChangeEvents
	require.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning, "Expected container to be RUNNING")

	portBindings := event.(api.ContainerStateChange).PortBindings
	// See comments on the getValidPortBinding() function for why ports need to be filtered.
	validPortBindings := getValidPortBinding(portBindings)

	verifyTaskRunningStateChange(t, taskEngine)

	if len(validPortBindings) != 2 {
		t.Error("Could not bind to two ports from one container port", portBindings)
	}
	var bindingForcontainerPortOne_1 uint16
	var bindingForcontainerPortOne_2 uint16
	for _, binding := range validPortBindings {
		if port == aws.Uint16Value(binding.ContainerPort) {
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
	testTask.Containers[1].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(port), HostPort: port}}

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
	testTaskFleuntdDriver.Containers[0].Ports = []apicontainer.PortBinding{{ContainerPort: aws.Uint16(24224), HostPort: 24224}}
	go taskEngine.AddTask(testTaskFleuntdDriver)
	verifyContainerRunningStateChange(t, taskEngine)
	verifyTaskRunningStateChange(t, taskEngine)

	// Sleep before starting the test task so that fluentd driver is setup
	time.Sleep(30 * time.Second)

	// start fluentd log task
	testTaskFluentdLogTag := createTestTask("testFleuntdTag")
	testTaskFluentdLogTag.Containers[0].Command = []string{"/bin/echo", "hello, this is fluentd integration test"}
	testTaskFluentdLogTag.Containers[0].Image = testBusyboxImage
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
		User:   "0",
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
		err1 := taskEngine.(*DockerTaskEngine).client.StartContainerExec(ctx, execContainerOut.ID, types.ExecStartCheck{Detach: true, Tty: false},
			dockerclient.ContainerExecStartTimeout)
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
