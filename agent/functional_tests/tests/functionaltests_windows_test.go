// +build windows,functional

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	savedStateTaskDefinition        = "savedstate-windows"
	portResContentionTaskDefinition = "port-80-windows"
	labelsTaskDefinition            = "labels-windows"
	logDriverTaskDefinition         = "logdriver-jsonfile-windows"
	cleanupTaskDefinition           = "cleanup-windows"
	networkModeTaskDefinition       = "network-mode-windows"
)

// TestAWSLogsDriver verifies that container logs are sent to Amazon CloudWatch Logs with awslogs as the log driver
func TestAWSLogsDriver(t *testing.T) {
	RequireDockerVersion(t, ">=1.9.0") // awslogs drivers available from docker 1.9.0
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	// Test whether the log group existed or not
	respDescribeLogGroups, err := cwlClient.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(awslogsLogGroupName),
	})
	require.NoError(t, err, "CloudWatchLogs describe log groups failed")
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
		require.NoError(t, err, "Failed to create log group %s", awslogsLogGroupName)
	}

	agentOptions := AgentOptions{}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.9.0") //Required for awslogs driver

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = aws.StringValue(ECS.Config.Region)

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs-windows", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskID)),
	})

	// Added a delay of 1 minute to allow the task to be stopped - Windows only.
	testTask.WaitStopped(1 * time.Minute)
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskID)),
	}

	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 1, fmt.Sprintf("Get unexpected number of log events: %d", len(resp.Events)))
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
		t.Fatalf("Could not found AWS_CONTAINER_CREDENTIALS_RELATIVE_URI in the container environment variable")
	}

	// Task will only run one command "aws ec2 describe-regions"
	err = task.WaitStopped(waitTaskStateChangeDuration)
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

// TestMetadataServiceValidator Tests that the metadata file can be accessed from the
// container using the ECS_CONTAINER_METADATA_FILE environment variables
func TestMetadataServiceValidator(t *testing.T) {
	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENABLE_CONTAINER_METADATA": "true",
		},
	}

	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.15.0")

	tdOverride := make(map[string]string)
	tdOverride["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverride["$$$NETWORK_MODE$$$"] = ""

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "mdservice-validator-windows", tdOverride)
	if err != nil {
		t.Fatalf("Error starting mdservice-validator-windows: %v", err)
	}

	// clean up
	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to transition to STOPPED")

	containerID, err := agent.ResolveTaskDockerID(task, "mdservice-validator-windows")
	if err != nil {
		t.Fatalf("Error resolving docker id for container in task: %v", err)
	}

	containerMetaData, err := agent.DockerClient.InspectContainer(containerID)
	require.NoError(t, err, "Could not inspect container for task")

	exitCode := containerMetaData.State.ExitCode
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

// TestTelemetry tests whether agent can send metrics to TACS
func TestTelemetry(t *testing.T) {
	telemetryTest(t, "telemetry-windows")
}

// TestOOMContainer verifies that an OOM container returns an error
func TestOOMContainer(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "oom-windows")
	require.NoError(t, err, "Expected to start invalid-image task")
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	assert.NoError(t, err, "Expect task to be running")
	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, err, "Expect task to be stopped")
	assert.NotEqual(t, 0, testTask.Containers[0].ExitCode, "container should fail with memory error")
}

// TestAWSLogsDriverMultilinePattern verifies that multiple log lines with a certain
// pattern, specified using 'awslogs-multiline-pattern' option, are sent to a single
// CloudWatch log event
func TestAWSLogsDriverMultilinePattern(t *testing.T) {
	RequireDockerAPIVersion(t, ">=1.30")
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(aws.StringValue(ECS.Config.Region)))
	cwlClient.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(awslogsLogGroupName),
	})

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	// TODO: Change the version required
	agent.RequireVersion(">=1.16.1") //Required for awslogs driver multiline pattern option

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = aws.StringValue(ECS.Config.Region)

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs-multiline-windows", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-multiline-windows/%s", taskID)),
	})

	// Added a delay of 1 minute to allow the task to be stopped - Windows only.
	testTask.WaitStopped(1 * time.Minute)
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-multiline-windows/%s", taskID)),
	}
	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 2, fmt.Sprintf("Got unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "INFO: ECS Agent\nRunning\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
	assert.Equal(t, *resp.Events[1].Message, "INFO: Instance\r\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[1].Message))
}

// TestAWSLogsDriverDatetimeFormat verifies that multiple log lines with a certain
// pattern of timestamp, specified using 'awslogs-datetime-format' option, are sent
// to a single CloudWatch log event
func TestAWSLogsDriverDatetimeFormat(t *testing.T) {
	RequireDockerAPIVersion(t, ">=1.30")
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(aws.StringValue(ECS.Config.Region)))
	cwlClient.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(awslogsLogGroupName),
	})

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	// TODO: Change the version required
	agent.RequireVersion(">=1.16.1") //Required for awslogs driver datetime format option

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = aws.StringValue(ECS.Config.Region)

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs-datetime-windows", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-datetime-windows/%s", taskID)),
	})

	// Added a delay of 1 minute to allow the task to be stopped - Windows only.
	testTask.WaitStopped(1 * time.Minute)
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-datetime-windows/%s", taskID)),
	}
	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 2, fmt.Sprintf("Got unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "May 01, 2017 19:00:01 ECS\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
	assert.Equal(t, *resp.Events[1].Message, "May 01, 2017 19:00:04 Agent\nRunning\nin the instance\r\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[1].Message))
}

// TestContainerHealthMetrics tests the container health metrics was sent to backend
func TestContainerHealthMetrics(t *testing.T) {
	containerHealthWithoutStartPeriodTest(t, "container-health-windows")
}

// TestContainerHealthMetricsWithStartPeriod tests the container health metrics
// with start period configured in the task definition
func TestContainerHealthMetricsWithStartPeriod(t *testing.T) {
	containerHealthWithStartPeriodTest(t, "container-health-windows")
}

func TestTwoTasksSharedLocalVolume(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.20.0")

	wTask, wTaskErr := agent.StartTask(t, "task-shared-vol-write-windows")
	require.NoError(t, wTaskErr, "Register task definition failed")

	rTask, rTaskErr := agent.StartTask(t, "task-shared-vol-read-windows")
	require.NoError(t, rTaskErr, "Register task definition failed")

	wErr := wTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, wErr, "Expect task to be stopped")
	wExitCode := wTask.Containers[0].ExitCode
	assert.NotEqual(t, 42, wExitCode, fmt.Sprintf("Expected exit code of 42; got %d", wExitCode))

	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, rErr, "Expect task to be stopped")
	rExitCode := rTask.Containers[0].ExitCode
	assert.NotEqual(t, 42, rExitCode, fmt.Sprintf("Expected exit code of 42; got %d", rExitCode))
}
