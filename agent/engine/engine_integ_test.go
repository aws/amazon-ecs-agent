// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	docker "github.com/fsouza/go-dockerclient"
)

var testRegistryHost = "127.0.0.1:51670"
var testBusyboxImage = testRegistryHost + "/busybox:latest"
var testRegistryImage = "127.0.0.1:51670/amazon/amazon-ecs-netkitten:latest"
var testAuthRegistryHost = "127.0.0.1:51671"
var testAuthRegistryImage = "127.0.0.1:51671/amazon/amazon-ecs-netkitten:latest"
var testVolumeImage = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
var testAuthUser = "user"
var testAuthPass = "swordfish"

func setup(t *testing.T) (TaskEngine, func(), *ttime.TestTime) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}
	if os.Getenv("ECS_SKIP_ENGINE_INTEG_TEST") != "" {
		t.Skip("ECS_SKIP_ENGINE_INTEG_TEST")
	}
	taskEngine := NewDockerTaskEngine(cfg, false)
	taskEngine.Init()
	test_time := ttime.NewTestTime()
	ttime.SetTime(test_time)
	return taskEngine, func() {
		taskEngine.Shutdown()
		test_time.Cancel()
	}, test_time
}

func discardEvents(from interface{}) func() {
	done := make(chan bool)

	go func() {
		for {
			ndx, _, _ := reflect.Select([]reflect.SelectCase{
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(from),
				},
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(done),
				},
			})
			if ndx == 1 {
				break
			}
		}
	}()
	return func() {
		done <- true
	}
}

func createTestContainer() *api.Container {
	return &api.Container{
		Name:          "netcat",
		Image:         testRegistryImage,
		Command:       []string{},
		Essential:     true,
		DesiredStatus: api.ContainerRunning,
		Cpu:           100,
		Memory:        80,
	}
}

func createTestTask(arn string) *api.Task {
	return &api.Task{
		Arn:           arn,
		Family:        arn,
		Version:       "1",
		DesiredStatus: api.TaskRunning,
		Containers:    []*api.Container{createTestContainer()},
	}
}

var cfg *config.Config

func init() {
	cfg, _ = config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
}

var endpoint = utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)

func removeImage(img string) {
	endpoint := utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)
	client, _ := docker.NewClient(endpoint)

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
	taskEngine, done, _ := setup(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(testRegistryImage)

	testTask := createTestTask("testStartUnpulled")

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskRunning, api.TaskStopped}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if taskEvent.Status != expected_event {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expected_event.String())
		}
		if len(expected_events) == 0 {
			break
		}
	}
}

// TestStartStopUnpulledImageDigest ensures that an unpulled image with
// specified digest is successfully pulled, run, and stopped via docker.
func TestStartStopUnpulledImageDigest(t *testing.T) {
	imageDigest := "tianon/true@sha256:30ed58eecb0a44d8df936ce2efce107c9ac20410c915866da4c6a33a3795d057"
	taskEngine, done, _ := setup(t)
	defer done()
	// Ensure this image isn't pulled by deleting it
	removeImage(imageDigest)

	testTask := createTestTask("testStartUnpulledDigest")
	testTask.Containers[0].Image = imageDigest

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskRunning, api.TaskStopped}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if taskEvent.Status != expected_event {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expected_event.String())
		}
		if len(expected_events) == 0 {
			break
		}
	}
}

// TestPortForward runs a container serving data on the randomly chosen port
// 24751 and verifies that when you do forward the port you can access it and if
// you don't forward the port you can't
func TestPortForward(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testArn := "testPortForwardFail"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}

	// Port not forwarded; verify we can't access it
	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskRunning {
			break
		} else if taskEvent.Status > api.TaskRunning {
			t.Fatal("Task went straight to " + taskEvent.Status.String() + " without running")
		}
	}

	time.Sleep(50 * time.Millisecond) // wait for Docker
	_, err := net.DialTimeout("tcp", "127.0.0.1:24751", 200*time.Millisecond)
	if err == nil {
		t.Error("Did not expect to be able to dial 127.0.0.1:24751 but didn't get error")
	}

	// Kill the existing container now to make the test run more quickly.
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerId
	client, _ := docker.NewClient(endpoint)
	err = client.KillContainer(docker.KillContainerOptions{ID: cid})
	if err != nil {
		t.Error("Could not kill container", err)
	}
	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status >= api.TaskStopped {
			break
		}
	}

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}

	taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskRunning {
			break
		} else if taskEvent.Status > api.TaskRunning {
			t.Fatal("Task went straight to " + taskEvent.Status.String() + " without running")
		}
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
	taskUpdate.DesiredStatus = api.TaskStopped
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

// TestMultiplePortForwards tests that two links containers in the same task can
// both expose ports successfully
func TestMultiplePortForwards(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, containerEvents := taskEngine.TaskEvents()

	defer discardEvents(containerEvents)()

	// Forward it and make sure that works
	testArn := "testMultiplePortForwards"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container1"}
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}
	testTask.Containers[0].Essential = false
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Command = []string{"-l=24751", "-serve", "ecs test container2"}
	testTask.Containers[1].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24752}}

	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskRunning {
			break
		} else if taskEvent.Status > api.TaskRunning {
			t.Fatal("Task went straight to " + taskEvent.Status.String() + " without running")
		}
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
	taskUpdate.DesiredStatus = api.TaskStopped
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

// TestDynamicPortForward runs a container serving data on a port chosen by the
// docker deamon and verifies that the port is reported in the state-change
func TestDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	testArn := "testDynamicPortForward"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751}}

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
	taskUpdate.DesiredStatus = api.TaskStopped
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

func TestMultipleDynamicPortForward(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container", `-loop`}
	// No HostPort or 0 hostport; docker should pick two ports for us
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751}, api.PortBinding{ContainerPort: 24751, HostPort: 0}}

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
	taskUpdate.DesiredStatus = api.TaskStopped
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

// TestLinking ensures that container linking does allow networking to go
// through to a linked container.  this test specifically starts a server that
// prints "hello linker" and then links a container that proxies that data to
// a publicly exposed port, where the tests reads it
func TestLinking(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	testTask := createTestTask("TestLinking")
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[0].Command = []string{"-l=80", "-serve", "hello linker"}
	testTask.Containers[0].Name = "linkee"
	testTask.Containers[1].Command = []string{"-l=24751", "linkee_alias:80"}
	testTask.Containers[1].Links = []string{"linkee:linkee_alias"}
	testTask.Containers[1].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskRunning {
			break
		} else if taskEvent.Status > api.TaskRunning {
			t.Fatal("Task went straight to " + taskEvent.Status.String() + " without running")
		}
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
	taskUpdate.DesiredStatus = api.TaskStopped
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

func TestDockerCfgAuth(t *testing.T) {
	authString := base64.StdEncoding.EncodeToString([]byte(testAuthUser + ":" + testAuthPass))
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` + testAuthRegistryHost + `/v1/":{"auth":"` + authString + `"}}`))
	cfg.EngineAuthType = "dockercfg"

	removeImage(testAuthRegistryImage)
	taskEngine, done, _ := setup(t)
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

	expected_events := []api.TaskStatus{api.TaskRunning}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if taskEvent.Status != expected_event {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expected_event.String())
		}
		if len(expected_events) == 0 {
			break
		}
	}

	taskUpdate := *testTask
	taskUpdate.DesiredStatus = api.TaskStopped
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
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte(`{"http://` + testAuthRegistryHost + `":{"username":"` + testAuthUser + `","password":"` + testAuthPass + `"}}`))
	cfg.EngineAuthType = "docker"
	defer func() {
		cfg.EngineAuthData = config.NewSensitiveRawMessage(nil)
		cfg.EngineAuthType = ""
	}()

	taskEngine, done, _ := setup(t)
	defer done()
	removeImage(testAuthRegistryImage)

	testTask := createTestTask("testDockerAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskRunning}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if taskEvent.Status != expected_event {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expected_event.String())
		}
		if len(expected_events) == 0 {
			break
		}
	}

	taskUpdate := *testTask
	taskUpdate.DesiredStatus = api.TaskStopped
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
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestTask("testVolumeContainer")
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "test2"
	testTask.Containers[1].Image = testVolumeImage
	testTask.Containers[1].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[1].Command = []string{"cat /data/test-file | nc -l -p 80"}
	testTask.Containers[1].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 80, HostPort: 24751}}

	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskRunning {
			break
		}
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
	taskUpdate.DesiredStatus = api.TaskStopped
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

func TestVolumesFromRO(t *testing.T) {
	taskEngine, done, _ := setup(t)
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
	testTask.Containers[1].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name, ReadOnly: true}}
	testTask.Containers[1].Command = []string{"touch /data/readonly-fs || exit 42"}
	testTask.Containers[2].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[2].Command = []string{"touch /data/notreadonly-fs-1 || exit 42"}
	testTask.Containers[3].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name, ReadOnly: false}}
	testTask.Containers[3].Command = []string{"touch /data/notreadonly-fs-2 || exit 42"}

	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskStopped {
			break
		}
	}

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

func TestHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	tmpPath, _ := ioutil.TempDir("", "ecs_volume_test")
	defer os.RemoveAll(tmpPath)
	ioutil.WriteFile(filepath.Join(tmpPath, "test-file"), []byte("test-data"), 0644)

	testTask := createTestTask("testHostVolumeMount")
	testTask.Volumes = []api.TaskVolume{api.TaskVolume{Name: "test-tmp", Volume: &api.FSHostVolume{FSSourcePath: tmpPath}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []api.MountPoint{api.MountPoint{ContainerPath: "/host/tmp", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{`echo -n "hi" > /host/tmp/hello-from-container; if [[ "$(cat /host/tmp/test-file)" != "test-data" ]]; then exit 4; fi; exit 42`}
	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskStopped {
			break
		}
	}

	if testTask.Containers[0].KnownExitCode == nil || *testTask.Containers[0].KnownExitCode != 42 {
		t.Error("Wrong exit code; file contents wrong or other error", *testTask.Containers[0].KnownExitCode)
	}
	data, err := ioutil.ReadFile(filepath.Join(tmpPath, "hello-from-container"))
	if err != nil || string(data) != "hi" {
		t.Error("Could not read file container wrote: ", err, string(data))
	}
}

func TestEmptyHostVolumeMount(t *testing.T) {
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestTask("testEmptyHostVolumeMount")
	testTask.Volumes = []api.TaskVolume{api.TaskVolume{Name: "test-tmp", Volume: &api.EmptyHostVolume{}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []api.MountPoint{api.MountPoint{ContainerPath: "/empty", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{`while true; do [[ -f "/empty/file" ]] && exit 42; done`}
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "test2"
	testTask.Containers[1].Image = testVolumeImage
	testTask.Containers[1].MountPoints = []api.MountPoint{api.MountPoint{ContainerPath: "/alsoempty/", SourceVolume: "test-tmp"}}
	testTask.Containers[1].Command = []string{`touch /alsoempty/file`}
	testTask.Containers[1].Essential = false
	go taskEngine.AddTask(testTask)

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		if taskEvent.Status == api.TaskStopped {
			break
		}
	}

	if testTask.Containers[0].KnownExitCode == nil || *testTask.Containers[0].KnownExitCode != 42 {
		t.Error("Wrong exit code; file probably wasn't present")
	}
}

func TestSweepContainer(t *testing.T) {
	taskEngine, done, test_time := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()

	defer discardEvents(contEvents)()

	testTask := createTestTask("testSweepContainer")

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskRunning, api.TaskStopped}

	for taskEvent := range taskEvents {
		if taskEvent.TaskArn != testTask.Arn {
			continue
		}
		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if taskEvent.Status != expected_event {
			t.Error("Got event " + taskEvent.Status.String() + " but expected " + expected_event.String())
		}
		if len(expected_events) == 0 {
			break
		}
	}

	defer discardEvents(taskEvents)()

	// Should be stopped, let's verify it's still listed...
	_, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn("testSweepContainer")
	if !ok {
		t.Error("Expected task to be present still, but wasn't")
	}
	test_time.Warp(4 * time.Hour)
	for i := 0; i < 60; i++ {
		_, ok = taskEngine.(*DockerTaskEngine).State().TaskByArn("testSweepContainer")
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if ok {
		t.Error("Expected container to have been sweept but was not")
	}
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
	taskEngine, done, _ := setup(t)
	defer done()

	taskEvents, contEvents := taskEngine.TaskEvents()
	defer discardEvents(taskEvents)()

	testTask := createTestTask("oomtest")
	testTask.Containers[0].Memory = 20
	testTask.Containers[0].Image = testBusyboxImage
	testTask.Containers[0].Command = []string{"sh", "-c", `x="a"; while true; do x=$x$x$x; done`}
	// should cause sh to get oomkilled as pid 1

	go taskEngine.AddTask(testTask)

	expected_events := []api.ContainerStatus{api.ContainerRunning, api.ContainerStopped}

	var contEvent api.ContainerStateChange
	for contEvent = range contEvents {
		if contEvent.TaskArn != testTask.Arn {
			continue
		}

		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if contEvent.Status != expected_event {
			t.Error("Got event " + contEvent.Status.String() + " but expected " + expected_event.String())
		}
		if len(expected_events) == 0 {
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
	taskEngine, done, _ := setup(t)
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
	cid := containerMap[testTask.Containers[0].Name].DockerId
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
	taskUpdate.DesiredStatus = api.TaskStopped
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
