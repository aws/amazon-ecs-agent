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
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/utils/ttime"
	docker "github.com/fsouza/go-dockerclient"
)

var testRegistryHost = "127.0.0.1:51670"
var testRegistryImage = "127.0.0.1:51670/amazon/amazon-ecs-netkitten:latest"
var testAuthRegistryHost = "127.0.0.1:51671"
var testAuthRegistryImage = "127.0.0.1:51671/amazon/amazon-ecs-netkitten:latest"
var testVolumeImage = "127.0.0.1:51670/amazon/amazon-ecs-volumes-test:latest"
var testAuthUser = "user"
var testAuthPass = "swordfish"

var test_time = ttime.NewTestTime()

func init() {
	ttime.SetTime(test_time)
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

func runProxyAuthRegistry() {
	// Run a basic-auth registry that proxies through to the regular registry
	// Only need to proxy through gets (at least for now)
	http.ListenAndServe(testAuthRegistryHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/_ping" {
			w.Write([]byte(`true`))
			return
		}

		token := r.Header.Get("Authorization")
		validToken := "123abc"
		if !strings.Contains(token, validToken) {
			// No token, check basicauth
			// Don't use .BasicAuth method to be go1.3 compatible
			user := ""
			pass := ""
			basicAuth := r.Header.Get("Authorization")
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(basicAuth, "Basic "))
			if err == nil {
				parts := strings.Split(string(decoded), ":")
				user = parts[0]
				if len(parts) > 1 {
					pass = parts[1]
				}
			}
			if user != testAuthUser || pass != testAuthPass {
				w.WriteHeader(403)
				w.Write([]byte(`permission denied`))
				return
			}
			// Regular auth fine, set token
			tokenString := "signature=123abc,access=read"
			w.Header().Set("WWW-Authenticate", "Token "+tokenString)
			w.Header().Set("X-Docker-Token", tokenString)
		}

		// Else, proxy through
		resp, err := http.Get("http://" + testRegistryHost + r.URL.Path)
		if err != nil {
			w.WriteHeader(404)
			return
		}
		io.Copy(w, resp.Body)
	}))
}

var taskEngine TaskEngine
var cfg *config.Config

func init() {
	cfg, _ = config.NewConfig()
	taskEngine = NewTaskEngine(cfg)
	taskEngine.Init()
	go runProxyAuthRegistry()
}

var endpoint = utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)

func removeImage(img string) {
	endpoint := utils.DefaultIfBlank(os.Getenv(DOCKER_ENDPOINT_ENV_VARIABLE), DOCKER_DEFAULT_ENDPOINT)
	client, _ := docker.NewClient(endpoint)

	client.RemoveImage(img)
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
	removeImage(testRegistryImage)

	testTask := createTestTask("testStartUnpulled")

	task_events := taskEngine.TaskEvents()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskCreated, api.TaskRunning, api.TaskStopped}

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

	task_events := taskEngine.TaskEvents()

	testArn := "testPortForwardFail"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}

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

	// Kill the existing container now to make the test run more quickly.
	containerMap, _ := taskEngine.(*DockerTaskEngine).state.ContainerMapByArn(testTask.Arn)
	cid := containerMap[testTask.Containers[0].Name].DockerId
	client, _ := docker.NewClient(endpoint)
	err = client.KillContainer(docker.KillContainerOptions{ID: cid})
	if err != nil {
		t.Error("Could not kill container", err)
	}
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus >= api.TaskStopped {
			break
		}
	}

	// Now forward it and make sure that works
	testArn = "testPortForwardWorking"
	testTask = createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}
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

	response, err := ioutil.ReadAll(conn)
	if err != nil {
		t.Error("Error reading response", err)
	}
	if string(response) != "ecs test container" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container'")
	}

	// Stop the existing container now
	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
			break
		}
	}
}

// TestMultiplePortForwards tests that two links containers in the same task can
// both expose ports successfully
func TestMultiplePortForwards(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	task_events := taskEngine.TaskEvents()

	// Forward it and make sure that works
	testArn := "testMultiplePortForwards"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container1"}
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "nc2"
	testTask.Containers[1].Command = []string{"-l=24751", "-serve", "ecs test container2"}
	testTask.Containers[1].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24752}}

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
		t.Fatal("Error dialing simple container 1 " + err.Error())
	}
	response, _ := ioutil.ReadAll(conn)
	if string(response) != "ecs test container1" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container1'")
	}
	conn, err = net.DialTimeout("tcp", "127.0.0.1:24752", 20*time.Millisecond)
	if err != nil {
		t.Fatal("Error dialing simple container 2 " + err.Error())
	}
	response, _ = ioutil.ReadAll(conn)
	if string(response) != "ecs test container2" {
		t.Error("Got response: " + string(response) + " instead of 'ecs test container2'")
	}

	// Kill the existing container now
	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
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

	task_events := taskEngine.TaskEvents()

	testArn := "testDynamicPortForward"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container"}
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
	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
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

	task_events := taskEngine.TaskEvents()

	testArn := "testDynamicPortForward2"
	testTask := createTestTask(testArn)
	testTask.Containers[0].Command = []string{"-l=24751", "-serve", "ecs test container", `-loop`}
	// No HostPort or 0 hostport; docker should pick two ports for us
	testTask.Containers[0].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751}, api.PortBinding{ContainerPort: 24751, HostPort: 0}}

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
	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
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
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[0].Command = []string{"-l=80", "-serve", "hello linker"}
	testTask.Containers[0].Name = "linkee"
	testTask.Containers[1].Command = []string{"-l=24751", "linkee_alias:80"}
	testTask.Containers[1].Links = []string{"linkee:linkee_alias"}
	testTask.Containers[1].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 24751, HostPort: 24751}}

	task_events := taskEngine.TaskEvents()

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

	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
			break
		}
	}
}

func TestDockerCfgAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}
	removeImage(testAuthRegistryImage)

	authString := base64.StdEncoding.EncodeToString([]byte(testAuthUser + ":" + testAuthPass))
	cfg.EngineAuthData = []byte(`{"http://` + testAuthRegistryHost + `/v1/":{"auth":"` + authString + `"}}`)
	cfg.EngineAuthType = "dockercfg"
	defer func() {
		cfg.EngineAuthData = nil
		cfg.EngineAuthType = ""
	}()

	testTask := createTestTask("testDockerCfgAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	task_events := taskEngine.TaskEvents()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskCreated, api.TaskRunning}

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

	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn == testTask.Arn {
			if !(task_event.TaskStatus >= api.TaskStopped) {
				t.Error("Expected only terminal events; got " + task_event.TaskStatus.String())
			}
			break
		}
	}
}

func TestDockerAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}
	removeImage(testAuthRegistryImage)

	cfg.EngineAuthData = []byte(`{"http://` + testAuthRegistryHost + `":{"username":"` + testAuthUser + `","password":"` + testAuthPass + `"}}`)
	cfg.EngineAuthType = "docker"
	defer func() {
		cfg.EngineAuthData = nil
		cfg.EngineAuthType = ""
	}()

	testTask := createTestTask("testDockerAuth")
	testTask.Containers[0].Image = testAuthRegistryImage

	task_events := taskEngine.TaskEvents()

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskCreated, api.TaskRunning}

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

	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)
	for task_event := range task_events {
		if task_event.TaskArn == testTask.Arn {
			if !(task_event.TaskStatus >= api.TaskStopped) {
				t.Error("Expected only terminal events; got " + task_event.TaskStatus.String())
			}
			break
		}
	}
}

func TestVolumesFrom(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	task_events := taskEngine.TaskEvents()

	testTask := createTestTask("testVolumeContainer")
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers = append(testTask.Containers, createTestContainer())
	testTask.Containers[1].Name = "test2"
	testTask.Containers[1].Image = testVolumeImage
	testTask.Containers[1].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[1].Command = []string{"cat /data/test-file | nc -l -p 80"}
	testTask.Containers[1].Ports = []api.PortBinding{api.PortBinding{ContainerPort: 80, HostPort: 24751}}

	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskRunning {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:24751", 10*time.Millisecond)
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

	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
			break
		}
	}
}

func TestVolumesFromRO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	task_events := taskEngine.TaskEvents()

	testTask := createTestTask("testVolumeROContainer")
	testTask.Containers[0].Image = testVolumeImage
	for i := 0; i < 3; i++ {
		cont := createTestContainer()
		cont.Name = "test" + strconv.Itoa(i)
		cont.Image = testVolumeImage
		cont.Essential = false
		testTask.Containers = append(testTask.Containers, cont)
	}
	testTask.Containers[1].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name, ReadOnly: true}}
	testTask.Containers[1].Command = []string{"touch /data/readonly-fs || exit 42"}
	testTask.Containers[2].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name}}
	testTask.Containers[2].Command = []string{"touch /data/notreadonly-fs-1 || exit 42"}
	testTask.Containers[3].VolumesFrom = []api.VolumeFrom{api.VolumeFrom{SourceContainer: testTask.Containers[0].Name, ReadOnly: false}}
	testTask.Containers[3].Command = []string{"touch /data/notreadonly-fs-2 || exit 42"}

	go taskEngine.AddTask(testTask)

WaitStopped:
	for _ = range task_events {
		for i := 1; i <= 3; i++ {
			if testTask.Containers[i].KnownStatus < api.ContainerStopped {
				continue WaitStopped
			}
		}
		break
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

	testTask.DesiredStatus = api.TaskStopped
	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
			break
		}
	}
}

func TestHostVolumeMount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	task_events := taskEngine.TaskEvents()

	tmpPath, _ := ioutil.TempDir("", "ecs_volume_test")
	defer os.RemoveAll(tmpPath)
	ioutil.WriteFile(filepath.Join(tmpPath, "test-file"), []byte("test-data"), 0644)

	testTask := createTestTask("testHostVolumeMount")
	testTask.Volumes = []api.TaskVolume{api.TaskVolume{Name: "test-tmp", Volume: &api.FSHostVolume{FSSourcePath: tmpPath}}}
	testTask.Containers[0].Image = testVolumeImage
	testTask.Containers[0].MountPoints = []api.MountPoint{api.MountPoint{ContainerPath: "/host/tmp", SourceVolume: "test-tmp"}}
	testTask.Containers[0].Command = []string{`echo -n "hi" > /host/tmp/hello-from-container; if [[ "$(cat /host/tmp/test-file)" != "test-data" ]]; then exit 4; fi; exit 42`}
	go taskEngine.AddTask(testTask)

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
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
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	task_events := taskEngine.TaskEvents()

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

	for task_event := range task_events {
		if task_event.TaskArn != testTask.Arn {
			continue
		}
		if task_event.TaskStatus == api.TaskStopped {
			break
		}
	}

	if testTask.Containers[0].KnownExitCode == nil || *testTask.Containers[0].KnownExitCode != 42 {
		t.Error("Wrong exit code; file probably wasn't present")
	}
}

func TestSweepContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integ test in short mode")
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skip("Docker not running")
	}

	task_events := taskEngine.TaskEvents()

	testTask := createTestTask("testSweepContainer")

	go taskEngine.AddTask(testTask)

	expected_events := []api.TaskStatus{api.TaskCreated, api.TaskRunning, api.TaskStopped}

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
		time.Sleep(500 * time.Millisecond)
	}
	if ok {
		t.Error("Expected container to have been sweept but was not")
	}
}
