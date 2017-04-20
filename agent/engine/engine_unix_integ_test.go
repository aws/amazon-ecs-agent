// +build !windows,integration

// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	"github.com/aws/aws-sdk-go/aws"
	docker "github.com/fsouza/go-dockerclient"
	"golang.org/x/net/context"
)

const (
	dockerEndpoint        = "unix:///var/run/docker.sock"
	testRegistryHost      = "127.0.0.1:51670"
	testBusyboxImage      = testRegistryHost + "/busybox:latest"
	testRegistryImage     = "127.0.0.1:51670/amazon/amazon-ecs-netkitten:latest"
	testAuthRegistryHost  = "127.0.0.1:51671"
	testAuthRegistryImage = "127.0.0.1:51671/amazon/amazon-ecs-netkitten:latest"
	testVolumeImage       = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
	testAuthUser          = "user"
	testAuthPass          = "swordfish"
)

func isDockerRunning() bool {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return false
	}
	return true
}

func createTestContainer() *api.Container {
	return &api.Container{
		Name:                "netcat",
		Image:               testRegistryImage,
		Command:             []string{},
		Essential:           true,
		DesiredStatusUnsafe: api.ContainerRunning,
		CPU:                 100,
		Memory:              80,
	}
}

var endpoint = utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)

func removeImage(img string) {
	removeEndpoint := utils.DefaultIfBlank(os.Getenv(DockerEndpointEnvVariable), DockerDefaultEndpoint)
	client, _ := docker.NewClient(removeEndpoint)

	client.RemoveImage(img)
}

func dialWithRetries(proto string, address string, tries int, timeout time.Duration) (net.Conn, error) {
	var err error
	var conn net.Conn
	for i := 0; i < tries; i++ {
		conn, err = net.DialTimeout(proto, address, timeout)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return conn, err
}

// TestStartStopUnpulledImage ensures that an unpulled image is successfully
// pulled, run, and stopped via docker.
func TestStartStopUnpulledImage(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(testRegistryImage)

	testTask := createTestTask("testStartUnpulled")

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expectedEvents := []api.TaskStatus{api.TaskRunning, api.TaskStopped}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expectedEvent := expectedEvents[0]
		expectedEvents = expectedEvents[1:]
		if taskEvent.Status != expectedEvent {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expectedEvent.String())
		}
		if len(expectedEvents) == 0 {
			break
		}
	}
}

// TestStartStopUnpulledImageDigest ensures that an unpulled image with
// specified digest is successfully pulled, run, and stopped via docker.
func TestStartStopUnpulledImageDigest(t *testing.T) {
	imageDigest := "tianon/true@sha256:30ed58eecb0a44d8df936ce2efce107c9ac20410c915866da4c6a33a3795d057"
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(imageDigest)

	testTask := createTestTask("testStartUnpulledDigest")
	testTask.Containers[0].Image = imageDigest

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expectedEvents := []api.TaskStatus{api.TaskRunning, api.TaskStopped}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expectedEvent := expectedEvents[0]
		expectedEvents = expectedEvents[1:]
		if taskEvent.Status != expectedEvent {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expectedEvent.String())
		}
		if len(expectedEvents) == 0 {
			break
		}
	}
}

// TestPortForward runs a container serving data on the randomly chosen port
// 24751 and verifies that when you do forward the port you can access it and if
// you don't forward the port you can't
func TestPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testArn := "testPortForwardFail"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}

	// Port not forwarded; verify we can't access it
	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	_, err = net.DialTimeout("tcp", "127.0.0.1:24751", 200*time.Millisecond)
	if err == nil {
		t.Error("Did not expect to be able to dial 127.0.0.1:24751 but didn't get error")
	}

	// Kill the existing container now to make the test run more quickly.
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerID
	client, _ := docker.NewClient(endpoint)
	err = client.KillContainer(docker.KillContainerOptions{ID: cid})
	if err != nil {
		t.Error("Could not kill container", err)
	}

	verifyTaskIsStopped(taskEvents, testTask)

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}
	testTask.Containers[0].Ports = []api.PortBinding{{ContainerPort: 24751, HostPort: 24751}}

	taskEngine.AddTask(testTask)

	err = verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	conn, err := dialWithRetries("tcp", "127.0.0.1:24751", 10, 20*time.Millisecond)
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
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Stop the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	verifyTaskIsStopped(taskEvents, testTask)
}

// TestMultiplePortForwards tests that two links containers in the same task can
// both expose ports successfully
func TestMultiplePortForwards(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, containerEvents := taskEngine.TaskEvents()

	defer discardEvents(containerEvents)()

	// Forward it and make sure that works
	testArn := "testMultiplePortForwards"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container1"}
	testTask.Containers[0].Ports = []api.PortBinding{{ContainerPort: 24751, HostPort: 24751}}
	testTask.Containers[0].Essential = false
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Command = []string{"-l=24751", "-serve", "ecs test container2"}
	testTask.Containers[1].Ports = []api.PortBinding{{ContainerPort: 24751, HostPort: 24752}}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	conn, err := dialWithRetries("tcp", "127.0.0.1:24751", 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container 1 " + err.Error())
	}
	t.Log("Dialed first container")
	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container1" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container1'")
	}
	t.Log("Read first container")
	conn, err = dialWithRetries("tcp", "127.0.0.1:24752", 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container 2 " + err.Error())
	}
	t.Log("Dialed second container")
	response, _ = ioutil.ReadAll(conn)
	if string(response) != "ecs test container2" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container2'")
	}
	t.Log("Read second container")

	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	verifyTaskIsStopped(taskEvents, testTask)
}

// TestDynamicPortForward runs a container serving data on a port chosen by the
// docker deamon and verifies that the port is reported in the state-change
func TestDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	testArn := "testDynamicPortForward"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []api.PortBinding{{ContainerPort: 24751}}

	go taskEngine.AddTask(testTask)

	var portBindings []api.PortBinding
PortsBound:
	for {
		select {
		case contEvent := <-contEvents:
			if contEvent.TaskArn != testTask.Arn {
				continue
			}
			if contEvent.Status == api.ContainerRunning {
				portBindings = contEvent.PortBindings
				break PortsBound
			} else if contEvent.Status > api.ContainerRunning {
				t.Fatal("Container went straight to " + contEvent.Status.String() + " without running")
			}
		case taskEvent := <-taskEvents:
			if taskEvent.Status == api.TaskStopped {
				t.Fatal("Task stopped")
			}
		}
	}
	defer discardEvents(contEvents)()

	if len(portBindings) != 1 {
		t.Error("PortBindings was not set; should have been len 1", portBindings)
	}
	var bindingFor24751 uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == 24751 {
			bindingFor24751 = binding.HostPort
		}
	}
	if bindingFor24751 == 0 {
		t.Error("Could not find the port mapping for 24751!")
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	conn, err := dialWithRetries("tcp", "127.0.0.1:"+strconv.Itoa(int(bindingFor24751)), 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Kill the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	verifyTaskIsStopped(taskEvents, testTask)
}

func TestMultipleDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container", `-loop`}
	// No HostPort or 0 hostport; docker should pick two ports for us
	testTask.Containers[0].Ports = []api.PortBinding{{ContainerPort: 24751}, {ContainerPort: 24751, HostPort: 0}}

	go taskEngine.AddTask(testTask)

	var portBindings []api.PortBinding
	for contEvent := range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}
		if contEvent.Status == api.ContainerRunning {
			portBindings = contEvent.PortBindings
			break
		} else if contEvent.Status > api.ContainerRunning {
			t.Fatal("Task went straight to " + contEvent.Status.String() + " without running")
		}
	}
	defer discardEvents(contEvents)()

	if len(portBindings) != 2 {
		t.Error("Could not bind to two ports from one container port", portBindings)
	}
	var bindingFor24751_1 uint16
	var bindingFor24751_2 uint16
	for _, binding := range portBindings {
		if binding.ContainerPort == 24751 {
			if bindingFor24751_1 == 0 {
				bindingFor24751_1 = binding.HostPort
			} else {
				bindingFor24751_2 = binding.HostPort
			}
		}
	}
	if bindingFor24751_1 == 0 {
		t.Error("Could not find the port mapping for 24751!")
	}
	if bindingFor24751_2 == 0 {
		t.Error("Could not find the port mapping for 24751!")
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	conn, err := dialWithRetries("tcp", "127.0.0.1:"+strconv.Itoa(int(bindingFor24751_1)), 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	conn, err = dialWithRetries("tcp", "127.0.0.1:"+strconv.Itoa(int(bindingFor24751_2)), 10, 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ = ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Kill the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyTaskIsStopped(taskEvents, testTask)
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
	testTask.Containers[1].Command = []string{"-l=24751", "linkee_alias:80"}
	testTask.Containers[1].Links = []string{"linkee:linkee_alias"}
	testTask.Containers[1].Ports = []api.PortBinding{{ContainerPort: 24751, HostPort: 24751}}

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	var response []byte
	for i := 0; i < 10; i++ {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:24751", 10*time.Millisecond)
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
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyTaskIsStopped(taskEvents, testTask)
}

func TestDockerCfgAuth(t *testing.T) {
	authString := base64.StdEncoding.EncodeToString([]byte(testAuthUser + ":" + testAuthPass))
	cfg := defaultTestConfigIntegTest()
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` + testAuthRegistryHost + `/v1/":{"auth":"` + authString + `"}}`))
	cfg.EngineAuthType = "dockercfg"

	removeImage(testAuthRegistryImage)
	taskEngine, done, _ := setup(cfg, t)
	defer done()
	defer func() {
		cfg.EngineAuthData = config.NewSensitiveRawMessage(nil)
		cfg.EngineAuthType = ""
	}()

	testTask := createTestTask("testDockerCfgAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expectedEvents := []api.TaskStatus{api.TaskRunning}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expectedEvent := expectedEvents[0]
		expectedEvents = expectedEvents[1:]
		if taskEvent.Status != expectedEvent {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expectedEvent.String())
		}
		if len(expectedEvents) == 0 {
			break
		}
	}

	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	for taskEvent := range taskEvents {
		if taskEvent.TaskArn == testTask.Arn {
			if !(taskEvent.Status >= api.TaskStopped) {
				t.Error("Expected only terminal events; got " + taskEvent.Status.String())
			}
			break
		}
	}
}

func TestDockerAuth(t *testing.T) {
	cfg := defaultTestConfigIntegTest()
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` + testAuthRegistryHost + `":{"username":"` + testAuthUser + `","password":"` + testAuthPass + `"}}`))
	cfg.EngineAuthType = "docker"
	defer func() {
		cfg.EngineAuthData = config.NewSensitiveRawMessage(nil)
		cfg.EngineAuthType = ""
	}()

	taskEngine, done, _ := setup(cfg, t)
	defer done()
	removeImage(testAuthRegistryImage)

	testTask := createTestTask("testDockerAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expectedEvents := []api.TaskStatus{api.TaskRunning}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expectedEvent := expectedEvents[0]
		expectedEvents = expectedEvents[1:]
		if taskEvent.Status != expectedEvent {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expectedEvent.String())
		}
		if len(expectedEvents) == 0 {
			break
		}
	}

	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	for taskEvent := range taskEvents {
		if taskEvent.TaskArn == testTask.Arn {
			if !(taskEvent.Status >= api.TaskStopped) {
				t.Error("Expected only terminal events; got " + taskEvent.Status.String())
			}
			break
		}
	}
}

func TestVolumesFrom(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestTask("testVolumeContainer")
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "test2"
	testTask.Containers[1].Image = testVolumeImage
	testTask.Containers[1].VolumesFrom = []api.VolumeFrom{{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[1].Command = []string{"cat /data/test-file | nc -l -p 80"}
	testTask.Containers[1].Ports = []api.PortBinding{{ContainerPort: 80, HostPort: 24751}}

	go taskEngine.AddTask(testTask)

	err := verifyTaskIsRunning(taskEvents, testTask)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	conn, err := dialWithRetries("tcp", "127.0.0.1:24751", 10, 10*time.Millisecond)
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
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)

	verifyTaskIsStopped(taskEvents, testTask)
}

func TestVolumesFromRO(t *testing.T) {
	taskEngine, done, _ := setupWithDefaultConfig(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestTask("testVolumeROContainer")
	testTask.Containers[0].Image = testVolumeImage
	for i := 0; i < 3; i++ {
		cont := createTestContainer()
		cont.Name = "test" + strconv.Itoa(i)
		cont.Image = testVolumeImage
		cont.Essential = i > 0
		testTask.Containers = append(testTask.Containers, cont)
	}
	testTask.Containers[1].VolumesFrom = []api.VolumeFrom{{SourceContainer: testTask.Containers[0].Name, ReadOnly: true}}
	testTask.Containers[1].Command = []string{"touch /data/readonly-fs || exit 42"}
	testTask.Containers[2].VolumesFrom = []api.VolumeFrom{{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[2].Command = []string{"touch /data/notreadonly-fs-1 || exit 42"}
	testTask.Containers[3].VolumesFrom = []api.VolumeFrom{{SourceContainer: testTask.Containers[0].Name, ReadOnly: false}}
	testTask.Containers[3].Command = []string{"touch /data/notreadonly-fs-2 || exit 42"}

	go taskEngine.AddTask(testTask)

	verifyTaskIsStopped(taskEvents, testTask)

	if testTask.Containers[1].KnownExitCode == nil || *testTask.Containers[1].KnownExitCode != 42 {
		t.Error("Didn't exit due to failure to touch ro fs as expected: ", *testTask.Containers[1].KnownExitCode)
	}
	if testTask.Containers[2].KnownExitCode == nil || *testTask.Containers[2].KnownExitCode != 0 {
		t.Error("Couldn't touch with default of rw")
	}
	if testTask.Containers[3].KnownExitCode == nil || *testTask.Containers[3].KnownExitCode != 0 {
		t.Error("Couldn't touch with explicit rw")
	}
}

func createTestHostVolumeMountTask(tmpPath string) *api.Task {
	testTask := createTestTask("testHostVolumeMount")
	testTask.Volumes = []api.TaskVolume{{Name: "test-tmp", Volume: &api.FSHostVolume{FSSourcePath: tmpPath}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []api.MountPoint{{ContainerPath: "/host/tmp", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{`echo -n "hi" > /host/tmp/hello-from-container; if [[ "$(cat /host/tmp/test-file)" != "test-data" ]]; then exit 4; fi; exit 42`}
	return testTask
}

func createTestEmptyHostVolumeMountTask() *api.Task {
	testTask := createTestTask("testEmptyHostVolumeMount")
	testTask.Volumes = []api.TaskVolume{{Name: "test-tmp", Volume: &api.EmptyHostVolume{}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []api.MountPoint{{ContainerPath: "/empty", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{`while true; do [[ -f "/empty/file" ]] && exit 42; done`}
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "test2"
	testTask.Containers[1].Image = testVolumeImage
	testTask.Containers[1].MountPoints = []api.MountPoint{{ContainerPath: "/alsoempty/", SourceVolume: "test-tmp"}}
	testTask.Containers[1].Command = []string{`touch /alsoempty/file`}
	testTask.Containers[1].Essential = false
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

	taskEvents, contEvents := taskEngine.TaskEvents()
	defer discardEvents(taskEvents)()

	testTask := createTestTask("oomtest")
	testTask.Containers[0].Memory = 20
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Command = []string{"sh", "-c", `x="a"; while true; do x=$x$x$x; done`}
	// should cause sh to get oomkilled as pid 1

	go taskEngine.AddTask(testTask)

	expectedEvents := []api.ContainerStatus{api.ContainerRunning, api.ContainerStopped}

	var contEvent api.ContainerStateChange
	for contEvent = range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}

		expectedEvent := expectedEvents[0]
		expectedEvents = expectedEvents[1:]
		if contEvent.Status != expectedEvent {
			t.Error("Got event " + contEvent.Status.String() + " but expected " + expectedEvent.String())
		}
		if len(expectedEvents) == 0 {
			break
		}
	}

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
	if !strings.HasPrefix(contEvent.Reason, OutOfMemoryError{}.ErrorName()) {
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

	taskEvents, contEvents := taskEngine.TaskEvents()
	defer discardEvents(taskEvents)()

	testTask := createTestTask("signaltest")
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Command = []string{
		"sh",
		"-c",
		fmt.Sprintf(`trap "exit 42" %d; trap "echo signal!" %d; while true; do sleep 1; done`, int(syscall.SIGTERM), int(syscall.SIGUSR1)),
	}

	go taskEngine.AddTask(testTask)
	var contEvent api.ContainerStateChange

	for contEvent = range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}
		if contEvent.Status == api.ContainerRunning {
			break
		} else if contEvent.Status > api.ContainerRunning {
			t.Fatal("Task went straight to " + contEvent.Status.String() + " without running")
		}
	}

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
		case contEvent = <-contEvents:
			if contEvent.TaskArn != testTask.Arn {
				continue
			}
			t.Fatalf("Expected no events; got " + contEvent.Status.String())
		default:
			break check_events
		}
	}

	// Stop the container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	for contEvent = range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}
		if !(contEvent.Status >= api.ContainerStopped) {
			t.Error("Expected only terminal events; got " + contEvent.Status.String())
		}
		break
	}

	if testTask.Containers[0].KnownExitCode == nil || *testTask.Containers[0].KnownExitCode != 42 {
		t.Error("Wrong exit code; file probably wasn't present")
	}
}

func TestDockerStopTimeout(t *testing.T) {
	os.Setenv("ECS_CONTAINER_STOP_TIMEOUT", testDockerStopTimeout.String())
	defer os.Unsetenv("ECS_CONTAINER_STOP_TIMEOUT")
	cfg := defaultTestConfigIntegTest()

	taskEngine, done, _ := setup(cfg, t)

	dockerTaskEngine := taskEngine.(*DockerTaskEngine)

	if dockerTaskEngine.cfg.DockerStopTimeout != testDockerStopTimeout {
		t.Errorf("Expect the docker stop timeout read from environment variable when ECS_CONTAINER_STOP_TIMEOUT is set, %v", dockerTaskEngine.cfg.DockerStopTimeout)
	}
	testTask := createTestTask("TestDockerStopTimeout")
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[0].Command = []string{"sh", "-c", "while true; do echo `date +%T`; sleep 1s; done;"}
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Name = "test-docker-timeout"

	taskEvents, contEvents := dockerTaskEngine.TaskEvents()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-taskEvents:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		done()
		cancel()
	}()

	go dockerTaskEngine.AddTask(testTask)

	for contEvent := range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}
		if contEvent.Status == api.ContainerRunning {
			break
		}
		if contEvent.Status > api.ContainerRunning {
			t.Error("Expect container to run not stop")
		}
	}

	startTime := ttime.Now()
	dockerTaskEngine.stopContainer(testTask, testTask.Containers[0])
	for contEvent := range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}
		if contEvent.Status == api.ContainerRunning {
			break
		}
		if contEvent.Status > api.ContainerStopped {
			t.Error("Expect container to stop")
		}
	}
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

	taskEvents, contEvents := taskEngine.TaskEvents()

	testArn := "testSecurityOptionNoNewPrivileges"
	testTask := createTestTask(testArn)
	testTask.Containers[0].DockerConfig = api.DockerConfig{HostConfig: aws.String(`{"SecurityOpt":["no-new-privileges"]}`)}

	go taskEngine.AddTask(testTask)

	for contEvent := range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}
		if contEvent.Status == api.ContainerRunning {
			break
		}
		if contEvent.Status > api.ContainerRunning {
			t.Fatal("Expect container to run not stop")
		}
	}

	defer discardEvents(contEvents)()
	defer discardEvents(taskEvents)()

	// Kill the existing container now
	taskUpdate := *testTask
	taskUpdate.SetDesiredStatus(api.TaskStopped)
	go taskEngine.AddTask(&taskUpdate)
	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskStopped {
			break
		}
	}
}
