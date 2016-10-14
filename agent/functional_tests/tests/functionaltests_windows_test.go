// +build windows
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
	//"bytes"
	"fmt"
	//"io/ioutil"
	//"os"
	//"path/filepath"
	"reflect"
	//"regexp"
	//"strconv"
	"strings"
	"testing"
	"time"
	"os"

	//"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	//"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	//"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	//docker "github.com/fsouza/go-dockerclient"
	//"github.com/pborman/uuid"
)

const (
	waitTaskStateChangeDuration     = 2 * time.Minute
	waitMetricsInCloudwatchDuration = 4 * time.Minute
	awslogsLogGroupName             = "ecs-functional-tests"
)

// TestRunManyTasks runs several tasks in short succession and expects them to
// all run.
func TestRunManyTasks(t *testing.T) {
fmt.Println("starting")
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	numToRun := 1
	tasks := []*TestTask{}
	attemptsTaken := 0
  fmt.Println("get td")
	td, err := GetTaskDefinition("simple-exit-windows")
	if err != nil {
		t.Fatalf("Get task definition error: %v", err)
	}
	fmt.Println("Starting Tasks")
  t.Logf("Starting Tasks")
	for numRun := 0; len(tasks) < numToRun; attemptsTaken++ {
		startNum := 1
		if numToRun-len(tasks) < 10 {
			startNum = numToRun - len(tasks)
		}

		startedTasks, err := agent.StartMultipleTasks(t, td, startNum)
		if err != nil {
			t.Error(err)
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
		fmt.Println("Container exit cod:", os.Getenv("LASTEXITCODE"))
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
		testTask, err := agent.StartTask(t, "ten-containers-windows")
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
		time.Sleep(90 * time.Second)

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

	testTask, err := agent.StartTask(t, "savedstate-windows")
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.WaitRunning(1 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	dockerId, err := agent.ResolveTaskDockerID(testTask, "savedstate-windows")
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

	testTask, err := agent.StartTask(t, "busybox-port-5180-windows")
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

	testTask2, err := agent.StartTask(t, "busybox-port-5180-windows")
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


func TestLabels(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.5.0")

	task, err := agent.StartTask(t, "labels-windows")
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

	task, err := agent.StartTask(t, "logdriver-jsonfile-windows")
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

// TestAwslogsDriver verifies that container logs are sent to Amazon CloudWatch Logs with awslogs as the log driver
func TestAwslogsDriver(t *testing.T) {
	RequireDockerVersion(t, ">=1.9.0") // awslogs drivers available from docker 1.9.0
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	// Test whether the log group existed or not
	respDescribeLogGroups, err := cwlClient.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(awslogsLogGroupName),
	})
	if err != nil {
		t.Fatalf("CloudWatchLogs describe log groups error: %v", err)
	}
	logGroupExists := false
	for i := 0; i < len(respDescribeLogGroups.LogGroups); i++ {
		if *respDescribeLogGroups.LogGroups[i].LogGroupName == awslogsLogGroupName {
			logGroupExists = true
			break
		}
	}

	if !logGroupExists {
		_, err := cwlClient.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(awslogsLogGroupName),
		})
		if err != nil {
			t.Fatalf("Failed to create log group %s : %v", awslogsLogGroupName, err)
		}
	}

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.9.0") //Required for awslogs driver

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs-windows", tdOverrides)
	if err != nil {
		t.Fatalf("Expected to start task using awslogs driver failed: %v", err)
	}

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	strs := strings.Split(*testTask.TaskArn, "/")
	taskId := strs[len(strs)-1]

	// Delete the log stream after the test
	defer func() {
		cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
			LogGroupName:  aws.String(awslogsLogGroupName),
			LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskId)),
		})
	}()

	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskId)),
	}
	resp, err := cwlClient.GetLogEvents(params)
	if err != nil {
		t.Fatalf("CloudWatchLogs get log failed: %v", err)
	}

	if len(resp.Events) != 1 {
		t.Errorf("Get unexpected number of log events: %d", len(resp.Events))
	} else if *resp.Events[0].Message != "hello world" {
		t.Errorf("Got log events message unexpected: %s", *resp.Events[0].Message)
	}
}
