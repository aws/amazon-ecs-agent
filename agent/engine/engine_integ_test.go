// Copyright 2014 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	docker "github.com/fsouza/go-dockerclient"
)

func createTestContainer() *api.Container {
	return &api.Container{
		Name:          "busybox",
		Image:         "busybox:latest",
		Essential:     true,
		DesiredStatus: api.ContainerRunning,
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

func runningContainer(name, image string, links, volumes []string) *api.Container {
	return &api.Container{
		Name:          name,
		Image:         image,
		Links:         links,
		Essential:     true,
		VolumesFrom:   volumes,
		DesiredStatus: api.ContainerRunning,
	}
}

// TestStartStopUnpulledImage ensures that an unpulled image is successfully
// pulled, run, and stopped via docker.
func TestStartStopUnpulledImage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	// Ensure this image isn't pulled by deleting it
	endpoint := utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)
	client, _ := docker.NewClient(endpoint)

	// Removing and pulling scratch should be very quick :)
	client.RemoveImage("scratch:latest")

	// This is going to fail hard because scratch doesn't have the entrypoint,
	// but if we see a "create" event we know it at least pulled the image
	// correctly
	testTask := createTestTask("testStartUnpulled")
	testTask.Containers[0].Command = []string{"echo", "hello world"}
	testTask.Containers[0].Image = "scratch:latest"

	taskEngine := MustTaskEngine()
	task_events, errs := taskEngine.TaskEvents()
	go func() {
		t.Fatal(<-errs)
	}()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskCreated, api.TaskRunning, api.TaskDead}

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		expected_event := expected_events[0]
		expected_events = expected_events[1:]
		if task_event.TaskStatus != expected_event {
			t.Error("Got event " + task_event.TaskStatus.String() + " but expected " + expected_event.String())
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
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	taskEngine := MustTaskEngine()
	task_events, errs := taskEngine.TaskEvents()
	go func() {
		t.Fatal(<-errs)
	}()

	testArn := "testPortForwardFail"
	testTask := createTestTask(testArn)
	// Busybox nc doesn't respond to sigterm so in order to speed things up we exit ourselves rather than wait for the sigkill
	testTask.Containers[0].Command = []string{"sh", "-c", `trap "exit 0" TERM; echo -n \"hello world\" | nc -l -p 24751 & while true; do sleep 1; done`}

	// Port not forwarded; verify we can't access it
	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskRunning {
			break
		} else if task_event.TaskStatus > api.TaskRunning {
			t.Fatal("Task went straight to " + task_event.TaskStatus.String() + " without running")
		}
	}
	_, err := net.DialTimeout("tcp", "127.0.0.1:24751", 20*time.Millisecond)
	if err == nil {
		t.Error("Did not expect to be able to dial 127.0.0.1:24751 but didn't get error")
	}

	// Kill the existing container now
	testTask.DesiredStatus = api.TaskDead
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskDead {
			break
		}
	}

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	testTask.Containers[0].Command = []string{"sh", "-c", `echo -n "ecs test container" | nc -l -p 24751`}
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}

	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskRunning {
			break
		} else if task_event.TaskStatus > api.TaskRunning {
			t.Fatal("Task went straight to " + task_event.TaskStatus.String() + " without running")
		}
	}

	time.Sleep(10 * time.Millisecond) // Give nc time to liseten

	conn, err := net.DialTimeout("tcp", "127.0.0.1:24751", 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Kill the existing container now
	testTask.DesiredStatus = api.TaskDead
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskDead {
			break
		}
	}
}

// TestDynamicPortForward runs a container serving data on a port chosen by the
// docker deamon and verifies that the port is reported in the state-change
func TestDynamicPortForward(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	taskEngine := MustTaskEngine()
	task_events, errs := taskEngine.TaskEvents()
	go func() {
		t.Fatal(<-errs)
	}()

	testArn := "testDynamicPortForward"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"sh", "-c", `echo -n "ecs test container" | nc -l -p 24751`}
	// No HostPort = docker should pick
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751}}

	go taskEngine.AddTask(testTask)

	var portBindings []api.PortBinding
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskRunning {
			portBindings = task_event.PortBindings
			break
		} else if task_event.TaskStatus > api.TaskRunning {
			t.Fatal("Task went straight to " + task_event.TaskStatus.String() + " without running")
		}
	}

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

	time.Sleep(10 * time.Millisecond) // Give nc time to liseten

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(bindingFor24751)), 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Kill the existing container now
	testTask.DesiredStatus = api.TaskDead
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskDead {
			break
		}
	}
}

func TestMultipleDynamicPortForward(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	taskEngine := MustTaskEngine()
	task_events, errs := taskEngine.TaskEvents()
	go func() {
		t.Fatal(<-errs)
	}()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"sh", "-c", `echo -n "ecs test container" | nc -l -p 24751; echo -n "ecs test container" | nc -l -p 24751`}
	// No HostPort = docker should pick two ports for us
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751}, api.PortBinding{ContainerPort: 24751}}

	go taskEngine.AddTask(testTask)

	var portBindings []api.PortBinding
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskRunning {
			portBindings = task_event.PortBindings
			break
		} else if task_event.TaskStatus > api.TaskRunning {
			t.Fatal("Task went straight to " + task_event.TaskStatus.String() + " without running")
		}
	}

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

	time.Sleep(10 * time.Millisecond) // Give nc time to liseten

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(bindingFor24751_1)), 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	time.Sleep(10 * time.Millisecond) // Give nc time to liseten

	conn, err = net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(bindingFor24751_2)), 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container " + err.Error())
	}

	response, _ = ioutil.ReadAll(conn)
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Kill the existing container now
	testTask.DesiredStatus = api.TaskDead
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskDead {
			break
		}
	}
}

// TestLinking ensures that container linking does allow networking to go
// through to a linked container.  this test specifically starts a server that
// prints "hello linker" and then links a container that proxies that data to
// a publicly exposed port, where the tests reads it
func TestLinking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	testTask := createTestTask("TestLinking")
	linkee := runningContainer("linkee", "busybox:latest", []string{}, []string{})
	linkee.Command = []string{"sh", "-c", `trap "exit 0" TERM; echo -n "hello linker" | nc -l -p 80; sleep 1`}
	linker := runningContainer("linker", "busybox:latest", []string{"linkee:linkee_alias"}, []string{})
	linker.Command = []string{"sh", "-c", `trap "exit 0" TERM; nc -l -p 24751 -e nc linkee_alias 80`}
	linker.Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}

	testTask.Containers = []*api.Container{linkee, linker}

	taskEngine := MustTaskEngine()
	task_events, errs := taskEngine.TaskEvents()
	go func() {
		t.Fatal(<-errs)
	}()

	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskRunning {
			break
		} else if task_event.TaskStatus > api.TaskRunning {
			t.Fatal("Task went straight to " + task_event.TaskStatus.String() + " without running")
		}
	}

	time.Sleep(10 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", "127.0.0.1:24751", 10*time.Millisecond)
	if err != nil {
		t.Error("Error dialing simple container" + err.Error())
	}

	response, err := ioutil.ReadAll(conn)
	if err != nil {
		t.Error(err)
	}
	if string(response) != "hello linker" {
		t.Error("Got response: " + string(response) + " instead of 'hello linker'")
	}

	testTask.DesiredStatus = api.TaskDead
	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskDead {
			break
		}
	}
}
