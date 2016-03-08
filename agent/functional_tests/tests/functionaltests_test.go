// +build functional
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

package functional_tests

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	docker "github.com/fsouza/go-dockerclient"
)

// TestRunManyTasks runs several tasks in short succession and expects them to
// all run.
func TestRunManyTasks(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	numToRun := 15
	tasks := []*TestTask{}
	attemptsTaken := 0
	for numRun := 0; len(tasks) < numToRun; attemptsTaken++ {
		startNum := 10
		if numToRun-len(tasks) < 10 {
			startNum = numToRun - len(tasks)
		}
		startedTasks, err := agent.StartMultipleTasks(t, "simple-exit", startNum)
		if err != nil {
			continue
		}
		tasks = append(tasks, startedTasks...)
		numRun += 10
	}

	t.Logf("Ran %v containers; took %v tries\n", numToRun, attemptsTaken)
	for _, task := range tasks {
		err := task.WaitStopped(10 * time.Minute)
		if err != nil {
			t.Error(err)
		}
		if code, ok := task.ContainerExitcode("exit"); !ok || code != 42 {
			t.Error("Wrong exit code")
		}
	}
}

// TestPullInvalidImage verifies that an invalid image returns an error
func TestPullInvalidImage(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "invalid-image")
	if err != nil {
		t.Fatalf("Expected to start invalid-image task: %v", err)
	}
	if err = testTask.ExpectErrorType("error", "CannotPullContainerError", 1*time.Minute); err != nil {
		t.Error(err)
	}
}

// TestOOMContainer verifies that an OOM container returns an error
func TestOOMContainer(t *testing.T) {
	RequireDockerVersion(t, "<1.9.0,>1.9.1") // https://github.com/docker/docker/issues/18510
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "oom-container")
	if err != nil {
		t.Fatalf("Expected to start invalid-image task: %v", err)
	}
	if err = testTask.ExpectErrorType("error", "OutOfMemoryError", 1*time.Minute); err != nil {
		t.Error(err)
	}
}

// This test addresses a deadlock issue which was noted in GH:313 and fixed
// in GH:320. It runs a service with 10 containers, waits for cleanup, starts
// another two instances of that service and ensures that those tasks complete.
func TestTaskCleanupDoesNotDeadlock(t *testing.T) {
	// Set the ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION to its lowest permissible value
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "60s")
	defer os.Unsetenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	// This bug was fixed in v1.8.1
	agent.RequireVersion(">=1.8.1")

	// Run two Tasks after cleanup, as the deadlock does not consistently occur after
	// after just one task cleanup cycle.
	for i := 0; i < 3; i++ {

		// Start a task with ten containers
		testTask, err := agent.StartTask(t, "ten-containers")
		if err != nil {
			t.Fatalf("Cycle %d: There was an error starting the Task: %v", i, err)
		}

		isTaskRunning, err := agent.WaitRunningViaIntrospection(testTask)
		if err != nil || !isTaskRunning {
			t.Fatalf("Cycle %d: Task should be RUNNING but is not: %v", i, err)
		}

		// Get the dockerID so we can later check that the container has been cleaned up.
		dockerId, err := agent.ResolveTaskDockerID(testTask, "1")
		if err != nil {
			t.Fatalf("Cycle %d: Error resolving docker id for container in task: %v", i, err)
		}

		// 2 minutes should be enough for the Task to have completed. If the task has not
		// completed and is in PENDING, the agent is most likely deadlocked.
		err = testTask.WaitStopped(2 * time.Minute)
		if err != nil {
			t.Fatalf("Cycle %d: Task did not transition into to STOPPED in time: %v", i, err)
		}

		isTaskStopped, err := agent.WaitStoppedViaIntrospection(testTask)
		if err != nil || !isTaskStopped {
			t.Fatalf("Cycle %d: Task should be STOPPED but is not: %v", i, err)
		}

		// Wait for the tasks to be cleaned up
		time.Sleep(75 * time.Second)

		// Ensure that tasks are cleaned up. WWe should not be able to describe the
		// container now since it has been cleaned up.
		_, err = agent.DockerClient.InspectContainer(dockerId)
		if err == nil {
			t.Fatalf("Cycle %d: Expected error inspecting container in task.", i)
		}
	}
}

// TestSavedState verifies that stopping the agent, stopping a container under
// its control, and starting the agent results in that container being moved to
// 'stopped'
func TestSavedState(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "nginx")
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.WaitRunning(1 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	dockerId, err := agent.ResolveTaskDockerID(testTask, "nginx")
	if err != nil {
		t.Fatal(err)
	}

	err = agent.StopAgent()
	if err != nil {
		t.Fatal(err)
	}

	err = agent.DockerClient.StopContainer(dockerId, 1)
	if err != nil {
		t.Fatal(err)
	}

	err = agent.StartAgent()
	if err != nil {
		t.Fatal(err)
	}

	testTask.WaitStopped(1 * time.Minute)
}

// TestPortResourceContention verifies that running two tasks on the same port
// in quick-succession does not result in the second one failing to run. It
// verifies the 'seqnum' serialization stuff works.
func TestPortResourceContention(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "busybox-port-5180")
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.WaitRunning(2 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.Stop()
	if err != nil {
		t.Fatal(err)
	}

	testTask2, err := agent.StartTask(t, "busybox-port-5180")
	if err != nil {
		t.Fatal(err)
	}
	err = testTask2.WaitRunning(4 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	testTask2.Stop()

	go testTask.WaitStopped(2 * time.Minute)
	testTask2.WaitStopped(2 * time.Minute)
}

func strptr(s string) *string { return &s }

func TestCommandOverrides(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	task, err := agent.StartTaskWithOverrides(t, "simple-exit", []*ecs.ContainerOverride{
		&ecs.ContainerOverride{
			Name:    strptr("exit"),
			Command: []*string{strptr("sh"), strptr("-c"), strptr("exit 21")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = task.WaitStopped(2 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode, _ := task.ContainerExitcode("exit"); exitCode != 21 {
		t.Errorf("Expected exit code of 21; got %v", exitCode)
	}
}

func TestLabels(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.5.0")

	task, err := agent.StartTask(t, "labels")
	if err != nil {
		t.Fatal(err)
	}

	err = task.WaitStopped(2 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dockerId, err := agent.ResolveTaskDockerID(task, "labeled")
	if err != nil {
		t.Fatal(err)
	}
	container, err := agent.DockerClient.InspectContainer(dockerId)
	if err != nil {
		t.Fatal(err)
	}
	if container.Config.Labels["label1"] != "" || container.Config.Labels["com.foo.label2"] != "value" {
		t.Fatalf("Labels did not match expected; expected to contain label1: com.foo.label2:value, got %v", container.Config.Labels)
	}
}

func TestLogdriverOptions(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.5.0")

	task, err := agent.StartTask(t, "logdriver-jsonfile")
	if err != nil {
		t.Fatal(err)
	}

	err = task.WaitStopped(2 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	dockerId, err := agent.ResolveTaskDockerID(task, "exit")
	if err != nil {
		t.Fatal(err)
	}
	container, err := agent.DockerClient.InspectContainer(dockerId)
	if err != nil {
		t.Fatal(err)
	}
	if container.HostConfig.LogConfig.Type != "json-file" {
		t.Errorf("Expected json-file type logconfig, was %v", container.HostConfig.LogConfig.Type)
	}
	if !reflect.DeepEqual(map[string]string{"max-file": "50", "max-size": "50k"}, container.HostConfig.LogConfig.Config) {
		t.Errorf("Expected max-file:50 max-size:50k for logconfig options, got %v", container.HostConfig.LogConfig.Config)
	}
}

func TestDockerAuth(t *testing.T) {
	agent := RunAgent(t, &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENGINE_AUTH_TYPE": "dockercfg",
			"ECS_ENGINE_AUTH_DATA": `{"127.0.0.1:51671":{"auth":"dXNlcjpzd29yZGZpc2g=","email":"foo@example.com"}}`, // user:swordfish
		},
	})
	defer agent.Cleanup()

	task, err := agent.StartTask(t, "simple-exit-authed")
	if err != nil {
		t.Fatal(err)
	}

	err = task.WaitStopped(2 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode, _ := task.ContainerExitcode("exit"); exitCode != 42 {
		t.Errorf("Expected exit code of 42; got %v", exitCode)
	}

	// verify there's no sign of auth details in the config; action item taken as
	// a result of accidentally logging them once
	logdir := agent.Logdir
	badStrings := []string{"user:swordfish", "swordfish", "dXNlcjpzd29yZGZpc2g="}
	err = filepath.Walk(logdir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
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
			// Because of this, strip down to just the comma-seperated hex and look for that
			if strings.Contains(string(data), gobytes[len(`[]byte{`):len(gobytes)-1]) {
				t.Fatalf("log data contained byte-hex representation of bad string: %v, %v", string(data), badstring)
			}
		}
		return nil
	})

	if err != nil {
		t.Errorf("Could not walk logdir: %v", err)
	}
}

func TestSquidProxy(t *testing.T) {
	// Run a squid proxy manually, verify that the agent can connect through it
	client, err := docker.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}

	dockerConfig := docker.Config{
		Image: "127.0.0.1:51670/amazon/squid:latest",
	}
	dockerHostConfig := docker.HostConfig{}

	squidContainer, err := client.CreateContainer(docker.CreateContainerOptions{
		Config:     &dockerConfig,
		HostConfig: &dockerHostConfig,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.StartContainer(squidContainer.ID, &dockerHostConfig); err != nil {
		t.Fatal(err)
	}
	defer func() {
		client.RemoveContainer(docker.RemoveContainerOptions{
			Force:         true,
			ID:            squidContainer.ID,
			RemoveVolumes: true,
		})
	}()

	// Resolve the name so we can use it in the link below; the create returns an ID only
	squidContainer, err = client.InspectContainer(squidContainer.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Squid startup time
	time.Sleep(1 * time.Second)
	t.Logf("Started squid container: %v", squidContainer.Name)

	agent := RunAgent(t, &AgentOptions{
		ExtraEnvironment: map[string]string{
			"HTTP_PROXY": "squid:3128",
			"NO_PROXY":   "169.254.169.254,/var/run/docker.sock",
		},
		ContainerLinks: []string{squidContainer.Name + ":squid"},
	})
	defer agent.Cleanup()
	agent.RequireVersion(">1.5.0")
	task, err := agent.StartTask(t, "simple-exit")
	if err != nil {
		t.Fatal(err)
	}
	// Verify the agent can run a container using the proxy
	task.WaitStopped(1 * time.Minute)

	// stop the agent, thus forcing it to close its connections; this is needed
	// because squid's access logs are written on DC not connect
	err = agent.StopAgent()
	if err != nil {
		t.Fatal(err)
	}

	// Now verify it actually used the proxy via squids access logs. Get all the
	// unique addresses that squid proxied for (assume nothing else used the
	// proxy).
	// This should be '3' currently, for example I see the following at the time of writing
	//     ecs.us-west-2.amazonaws.com:443
	//     ecs-a-1.us-west-2.amazonaws.com:443
	//     ecs-t-1.us-west-2.amazonaws.com:443
	// Note, it connects multiple times to the first one which is an
	// implementation detail we might change/optimize, intentionally dedupe so
	// we're not tied to that sorta thing
	// Note, do a docker exec instead of bindmount the logs out because the logs
	// will not be permissioned correctly in the bindmount. Once we have proper
	// user namespacing we could revisit this
	logExec, err := client.CreateExec(docker.CreateExecOptions{
		AttachStdout: true,
		AttachStdin:  false,
		Container:    squidContainer.ID,
		// Takes a second to flush the file sometimes, so slightly complicated command to wait for it to be written
		Cmd: []string{"sh", "-c", "FILE=/var/log/squid/access.log; while [ ! -s $FILE ]; do sleep 1; done; cat $FILE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Execing cat of /var/log/squid/access.log on %v", squidContainer.ID)

	var squidLogs bytes.Buffer
	err = client.StartExec(logExec.ID, docker.StartExecOptions{
		OutputStream: &squidLogs,
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		tmp, _ := client.InspectExec(logExec.ID)
		if !tmp.Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Squid logs: %v", squidLogs.String())

	// Of the format:
	//    1445018173.730   3163 10.0.0.1 TCP_MISS/200 5706 CONNECT ecs.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.250.253 -
	//    1445018173.730   3103 10.0.0.1 TCP_MISS/200 3117 CONNECT ecs.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.250.253 -
	//    1445018173.730   3025 10.0.0.1 TCP_MISS/200 3336 CONNECT ecs-a-1.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.249.4 -
	//    1445018173.731   3086 10.0.0.1 TCP_MISS/200 3411 CONNECT ecs-t-1.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.254.59
	allAddressesRegex := regexp.MustCompile("CONNECT [^ ]+ ")
	// Match just the host+port it's proxying to
	matches := allAddressesRegex.FindAllStringSubmatch(squidLogs.String(), -1)
	t.Logf("Proxy connections: %v", matches)
	dedupedMatches := map[string]struct{}{}
	for _, match := range matches {
		dedupedMatches[match[0]] = struct{}{}
	}

	if len(dedupedMatches) < 3 {
		t.Errorf("Expected 3 matches, actually had %d matches: %+v", len(dedupedMatches), dedupedMatches)
	}
}

func TestTaskCleanup(t *testing.T) {
	// Set the task cleanup time to just over a minute.
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "70s")
	agent := RunAgent(t, nil)
	defer func() {
		agent.Cleanup()
		os.Unsetenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")
	}()

	// Start a task and get the container id once the task transitions to RUNNING.
	task, err := agent.StartTask(t, "nginx")
	if err != nil {
		t.Fatalf("Error starting task: %v", err)
	}

	err = task.WaitRunning(2 * time.Minute)
	if err != nil {
		t.Fatalf("Error waiting for running task: %v", err)
	}

	dockerId, err := agent.ResolveTaskDockerID(task, "nginx")
	if err != nil {
		t.Fatalf("Error resolving docker id for container in task: %v", err)
	}

	// We should be able to inspect the container ID from docker at this point.
	_, err = agent.DockerClient.InspectContainer(dockerId)
	if err != nil {
		t.Fatalf("Error inspecting container in task: %v", err)
	}

	// Stop the task and sleep for 2 minutes to let the task be cleaned up.
	err = agent.DockerClient.StopContainer(dockerId, 1)
	if err != nil {
		t.Fatalf("Error stoppping task: %v", err)
	}

	err = task.WaitStopped(1 * time.Minute)
	if err != nil {
		t.Fatalf("Error waiting for task stopped: %v", err)
	}

	time.Sleep(2 * time.Minute)

	// We should not be able to describe the container now since it has been cleaned up.
	_, err = agent.DockerClient.InspectContainer(dockerId)
	if err == nil {
		t.Fatalf("Expected error inspecting container in task")
	}
}
