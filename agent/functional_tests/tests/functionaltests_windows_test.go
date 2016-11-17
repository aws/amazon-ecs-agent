// +build windows,functional

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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

const (
	savedStateTaskDefinition        = "savedstate-windows"
	portResContentionTaskDefinition = "port-80-windows"
	labelsTaskDefinition            = "labels-windows"
	logDriverTaskDefinition         = "logdriver-jsonfile-windows"
	cleanupTaskDefinition           = "cleanup-windows"
	networkModeTaskDefinition       = "network-mode-windows"
)

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
	for i := 0; i < 2; i++ {

		// Start a task with ten containers
		testTask, err := agent.StartTask(t, "ten-containers-windows")
		if err != nil {
			t.Fatalf("Cycle %d: There was an error starting the Task: %v", i, err)
		}

		// Added 1 minute delay to allow the task to be in running state - Required only on Windows
		testTask.WaitRunning(1 * time.Minute)

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

	// Added a delay of 1 minute to allow the task to be stopped - Windows only.
	testTask.WaitStopped(1 * time.Minute)
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskId)),
	}
	resp, err := cwlClient.GetLogEvents(params)
	if err != nil {
		t.Fatalf("CloudWatchLogs get log failed: %v", err)
	}

	if len(resp.Events) != 1 {
		t.Errorf("Get number of log events: %d", len(resp.Events))
	}
}

func TestTaskIamRolesDefaultNetworkMode(t *testing.T) {
	// The test runs only when the environment TEST_IAM_ROLE was set
	if os.Getenv("TEST_TASK_IAM_ROLE") != "true" {
		t.Skip("Skipping test TaskIamRole in default network mode, as TEST_TASK_IAM_ROLE isn't set true")
	}

	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENABLE_TASK_IAM_ROLE": "true",
		},
	}
	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	taskIamRolesTest("", agent, t)
}

func taskIamRolesTest(networkMode string, agent *TestAgent, t *testing.T) {
	RequireDockerVersion(t, ">=1.11.0") // TaskIamRole is available from agent 1.11.0
	roleArn := os.Getenv("TASK_IAM_ROLE_ARN")
	if utils.ZeroOrNil(roleArn) {
		t.Logf("TASK_IAM_ROLE_ARN not set, will try to use the role attached to instance profile")
		role, err := GetInstanceIAMRole()
		if err != nil {
			t.Fatalf("Error getting IAM Roles from instance profile, err: %v", err)
		}
		roleArn = *role.Arn
	}

	tdOverride := make(map[string]string)
	tdOverride["$$$TASK_ROLE$$$"] = roleArn
	tdOverride["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverride["$$$NETWORK_MODE$$$"] = networkMode

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "iam-roles-windows", tdOverride)
	if err != nil {
		t.Fatalf("Error start iam-roles task: %v", err)
	}
	err = task.WaitRunning(waitTaskStateChangeDuration)
	if err != nil {
		t.Fatalf("Error waiting for task to run: %v", err)
	}
	containerId, err := agent.ResolveTaskDockerID(task, "container-with-iamrole-windows")
	if err != nil {
		t.Fatalf("Error resolving docker id for container in task: %v", err)
	}

	// TaskIAMRoles enabled contaienr should have the ExtraEnvironment variable AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
	containerMetaData, err := agent.DockerClient.InspectContainer(containerId)
	if err != nil {
		t.Fatalf("Could not inspect container for task: %v", err)
	}
	iamRoleEnabled := false
	if containerMetaData.Config != nil {
		for _, env := range containerMetaData.Config.Env {
			if strings.HasPrefix(env, "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI=") {
				iamRoleEnabled = true
				break
			}
		}
	}
	if !iamRoleEnabled {
		task.Stop()
		t.Fatalf("Could not found AWS_CONTAINER_CREDENTIALS_RELATIVE_URI in the container envrionment variable")
	}

	// Task will only run one command "aws ec2 describe-regions"
	err = task.WaitStopped(2 * time.Minute)
	if err != nil {
		t.Fatalf("Waiting task to stop error : %v", err)
	}

	containerMetaData, err = agent.DockerClient.InspectContainer(containerId)
	if err != nil {
		t.Fatalf("Could not inspect container for task: %v", err)
	}

	if containerMetaData.State.ExitCode != 42 {
		t.Fatalf("Container exit code non-zero: %v", containerMetaData.State.ExitCode)
	}

}
