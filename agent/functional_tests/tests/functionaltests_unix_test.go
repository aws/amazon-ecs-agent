// +build !windows,functional

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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	ecsapi "github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	savedStateTaskDefinition        = "nginx"
	portResContentionTaskDefinition = "busybox-port-5180"
	labelsTaskDefinition            = "labels"
	logDriverTaskDefinition         = "logdriver-jsonfile"
	cleanupTaskDefinition           = "nginx"
	networkModeTaskDefinition       = "network-mode"
	fluentdLogPath                  = "/tmp/ftslog"
)

// TestRunManyTasks runs several tasks in short succession and expects them to
// all run.
func TestRunManyTasks(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	numToRun := 15
	tasks := []*TestTask{}
	attemptsTaken := 0

	td, err := GetTaskDefinition("simple-exit")
	require.NoError(t, err, "Register task definition failed")
	for numRun := 0; len(tasks) < numToRun; attemptsTaken++ {
		startNum := 10
		if numToRun-len(tasks) < 10 {
			startNum = numToRun - len(tasks)
		}

		startedTasks, err := agent.StartMultipleTasks(t, td, startNum)
		if err != nil {
			continue
		}
		tasks = append(tasks, startedTasks...)
		numRun += 10
	}

	t.Logf("Ran %v containers; took %v tries\n", numToRun, attemptsTaken)
	for _, task := range tasks {
		err := task.WaitStopped(10 * time.Minute)
		assert.NoError(t, err)
		code, ok := task.ContainerExitcode("exit")
		assert.True(t, ok, "Get exit code failed")
		assert.Equal(t, 42, code, "Wrong exit code")
	}
}

// TestOOMContainer verifies that an OOM container returns an error
func TestOOMContainer(t *testing.T) {
	// oom container task requires 500MB of memory; requires a bit more to be stable
	RequireMinimumMemory(t, 600)

	RequireDockerVersion(t, "<1.9.0,>1.9.1") // https://github.com/docker/docker/issues/18510
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "oom-container")
	require.NoError(t, err, "Expected to start invalid-image task")
	err = testTask.ExpectErrorType("error", "OutOfMemoryError", 1*time.Minute)
	assert.NoError(t, err)
}

// TestOOMTask verifies that a task with a memory limit returns an error
func TestOOMTask(t *testing.T) {
	// oom container task requires 500MB of memory; requires a bit more to be stable
	RequireMinimumMemory(t, 600)

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.16.0")

	testTask, err := agent.StartTask(t, "oom-task")
	require.NoError(t, err, "Expected to start invalid-image task")
	err = testTask.ExpectErrorType("error", "OutOfMemoryError", 1*time.Minute)
	assert.NoError(t, err)
}

func strptr(s string) *string { return &s }

func TestCommandOverrides(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	task, err := agent.StartTaskWithOverrides(t, "simple-exit", []*ecsapi.ContainerOverride{
		{
			Name:    strptr("exit"),
			Command: []*string{strptr("sh"), strptr("-c"), strptr("exit 21")},
		},
	})
	require.NoError(t, err)

	err = task.WaitStopped(2 * time.Minute)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("exit")
	assert.Equal(t, 21, exitCode, fmt.Sprintf("Expected exit code of 21; got %d", exitCode))
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
	require.NoError(t, err)

	err = task.WaitStopped(2 * time.Minute)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("exit")
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))

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
			// Because of this, strip down to just the comma-separated hex and look for that
			if strings.Contains(string(data), gobytes[len(`[]byte{`):len(gobytes)-1]) {
				t.Fatalf("log data contained byte-hex representation of bad string: %v, %v", string(data), badstring)
			}
		}
		return nil
	})

	assert.NoError(t, err, "Could not walk logdir")
}

func TestSquidProxy(t *testing.T) {
	ctx := context.TODO()
	// Run a squid proxy manually, verify that the agent can connect through it
	client, err := docker.NewClientWithOpts(docker.WithVersion("1.17"))
	require.NoError(t, err)

	squidImage := "127.0.0.1:51670/amazon/squid:latest"
	dockerConfig := dockercontainer.Config{
		Image: squidImage,
	}
	dockerHostConfig := dockercontainer.HostConfig{}

	_, err = client.ImagePull(ctx, squidImage, types.ImagePullOptions{})
	require.NoError(t, err)
	// Squid pull time
	time.Sleep(2 * time.Second)

	squidContainer, err := client.ContainerCreate(ctx,
		&dockerConfig,
		&dockerHostConfig,
		&network.NetworkingConfig{},
		"") // containerName
	require.NoError(t, err)
	err = client.ContainerStart(ctx, squidContainer.ID, types.ContainerStartOptions{})
	require.NoError(t, err)
	defer func() {
		client.ContainerRemove(ctx, squidContainer.ID, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		})
	}()

	// Resolve the name so we can use it in the link below; the create returns an ID only
	squidContainerJSON, err := client.ContainerInspect(ctx, squidContainer.ID)
	require.NoError(t, err)

	// Squid startup time
	time.Sleep(1 * time.Second)
	t.Logf("Started squid container: %v", squidContainerJSON.Name)

	agent := RunAgent(t, &AgentOptions{
		ExtraEnvironment: map[string]string{
			"HTTP_PROXY": "squid:3128",
			"NO_PROXY":   "169.254.169.254,/var/run/docker.sock",
		},
		ContainerLinks: []string{squidContainerJSON.Name + ":squid"},
	})
	defer agent.Cleanup()
	agent.RequireVersion(">1.5.0")
	task, err := agent.StartTask(t, "simple-exit")
	require.NoError(t, err)
	// Verify the agent can run a container using the proxy
	task.WaitStopped(1 * time.Minute)

	// stop the agent, thus forcing it to close its connections; this is needed
	// because squid's access logs are written on DC not connect
	err = agent.StopAgent()
	require.NoError(t, err)

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
	idResponse, err := client.ContainerExecCreate(
		ctx,
		squidContainer.ID,
		types.ExecConfig{
			AttachStdout: true,
			AttachStdin:  false,
			Cmd:          []string{"sh", "-c", "FILE=/var/log/squid/access.log; while [ ! -s $FILE ]; do sleep 1; done; cat $FILE"},
		})
	require.NoError(t, err)
	t.Logf("Execing cat of /var/log/squid/access.log on %v", squidContainer.ID)

	hijackedResp, err := client.ContainerExecAttach(ctx, idResponse.ID, types.ExecStartCheck{})
	defer hijackedResp.Close()
	require.NoError(t, err)
	for {
		inspect, _ := client.ContainerExecInspect(ctx, idResponse.ID)
		if !inspect.Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	squidLogs, err := ioutil.ReadAll(hijackedResp.Reader)
	require.NoError(t, err)
	squildLogStr := string(squidLogs[:])
	t.Logf("Squid logs: %v", squildLogStr)

	// Of the format:
	//    1445018173.730   3163 10.0.0.1 TCP_MISS/200 5706 CONNECT ecs.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.250.253 -
	//    1445018173.730   3103 10.0.0.1 TCP_MISS/200 3117 CONNECT ecs.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.250.253 -
	//    1445018173.730   3025 10.0.0.1 TCP_MISS/200 3336 CONNECT ecs-a-1.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.249.4 -
	//    1445018173.731   3086 10.0.0.1 TCP_MISS/200 3411 CONNECT ecs-t-1.us-west-2.amazonaws.com:443 - HIER_DIRECT/54.240.254.59
	allAddressesRegex := regexp.MustCompile("CONNECT [^ ]+ ")
	// Match just the host+port it's proxying to
	matches := allAddressesRegex.FindAllStringSubmatch(squildLogStr, -1)
	t.Logf("Proxy connections: %v", matches)
	dedupedMatches := map[string]struct{}{}
	for _, match := range matches {
		dedupedMatches[match[0]] = struct{}{}
	}

	if len(dedupedMatches) < 3 {
		t.Errorf("Expected 3 matches, actually had %d matches: %+v", len(dedupedMatches), dedupedMatches)
	}
}

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
		require.NoError(t, err, fmt.Sprintf("Failed to create log group %s", awslogsLogGroupName))
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

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer func() {
		cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
			LogGroupName:  aws.String(awslogsLogGroupName),
			LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskID)),
		})
	}()

	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs/%s", taskID)),
	}

	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 1, fmt.Sprintf("Get unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "hello world", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
}

// TestTelemetry tests whether agent can send metrics to TACS, through streaming docker stats
func TestTelemetry(t *testing.T) {
	telemetryTest(t, "telemetry")
}

// TestTelemetry tests whether agent can send metrics to TACS, through polling docker stats
func TestTelemetryWithStatsPolling(t *testing.T) {
	telemetryTestWithStatsPolling(t, "telemetry")
}

func TestTaskIAMRolesNetHostMode(t *testing.T) {
	// The test runs only when the environment TEST_IAM_ROLE was set
	if os.Getenv("TEST_DISABLE_TASK_IAM_ROLE_NET_HOST") == "true" {
		t.Skip("Skipping test TaskIamRole in host network mode, as TEST_DISABLE_TASK_IAM_ROLE_NET_HOST is set true")
	}
	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST": "true",
			"ECS_ENABLE_TASK_IAM_ROLE":              "true",
		},
		PortBindings: map[nat.Port]map[string]string{
			"51679/tcp": {
				"HostIP":   "0.0.0.0",
				"HostPort": "51679",
			},
		},
	}
	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	taskIAMRoles("host", agent, t)
}

func TestTaskIAMRolesDefaultNetworkMode(t *testing.T) {
	// The test runs only when the environment TEST_IAM_ROLE was set
	if os.Getenv("TEST_DISABLE_TASK_IAM_ROLE") == "true" {
		t.Skip("Skipping test TaskIamRole in default network mode, as TEST_DISABLE_TASK_IAM_ROLE is set true")
	}

	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENABLE_TASK_IAM_ROLE": "true",
		},
		PortBindings: map[nat.Port]map[string]string{
			"51679/tcp": {
				"HostIP":   "0.0.0.0",
				"HostPort": "51679",
			},
		},
	}
	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	taskIAMRoles("bridge", agent, t)
}

func taskIAMRoles(networkMode string, agent *TestAgent, t *testing.T) {
	ctx := context.TODO()
	agent.RequireVersion(">=1.11.0") // TaskIamRole is available from agent 1.11.0
	roleArn := os.Getenv("TASK_IAM_ROLE_ARN")
	if utils.ZeroOrNil(roleArn) {
		t.Logf("TASK_IAM_ROLE_ARN not set, will try to use the role attached to instance profile")
		role, err := GetInstanceIAMRole()
		require.NoError(t, err, "Error getting IAM Roles from instance profile")
		roleArn = *role.Arn
	}

	tdOverride := make(map[string]string)
	tdOverride["$$$TASK_ROLE$$$"] = roleArn
	tdOverride["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverride["$$$NETWORK_MODE$$$"] = networkMode

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "iam-roles", tdOverride)
	require.NoError(t, err, "Error start iam-roles task")
	err = task.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to run")
	containerId, err := agent.ResolveTaskDockerID(task, "container-with-iamrole")
	require.NoError(t, err, "Error resolving docker id for container in task")

	// TaskIAMRoles enabled container should have the ExtraEnvironment variable AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
	containerMetaData, err := agent.DockerClient.ContainerInspect(ctx, containerId)
	require.NoError(t, err, "Could not inspect container for task")
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
		t.Fatal("Could not found AWS_CONTAINER_CREDENTIALS_RELATIVE_URI in the container environment variable")
	}

	// Task will only run one command "aws ec2 describe-regions"
	err = task.WaitStopped(30 * time.Second)
	require.NoError(t, err, "Waiting task to stop error")

	containerMetaData, err = agent.DockerClient.ContainerInspect(ctx, containerId)
	require.NoError(t, err, "Could not inspect container for task")

	require.Equal(t, 0, containerMetaData.State.ExitCode, fmt.Sprintf("Container exit code non-zero: %v", containerMetaData.State.ExitCode))

	// Search the audit log to verify the credential request
	err = SearchStrInDir(filepath.Join(agent.TestDir, "log"), "audit.log.", *task.TaskArn)
	require.NoError(t, err, "Verify credential request failed")
}

func TestV3TaskEndpointAWSVPCNetworkMode(t *testing.T) {
	testV3TaskEndpoint(t, "v3-task-endpoint-validator", "v3-task-endpoint-validator", "awsvpc", "ecs-functional-tests-v3-task-endpoint-validator")
}

func TestV3TaskEndpointBridgeNetworkMode(t *testing.T) {
	testV3TaskEndpoint(t, "v3-task-endpoint-validator", "v3-task-endpoint-validator", "bridge", "ecs-functional-tests-v3-task-endpoint-validator")
}

func TestV3TaskEndpointHostNetworkMode(t *testing.T) {
	testV3TaskEndpoint(t, "v3-task-endpoint-validator", "v3-task-endpoint-validator", "host", "ecs-functional-tests-v3-task-endpoint-validator")
}

func TestV3TaskEndpointTags(t *testing.T) {
	testV3TaskEndpointTags(t, "v3-task-endpoint-validator", "v3-task-endpoint-validator", "host")
}

func TestContainerMetadataFile(t *testing.T) {
	testContainerMetadataFile(t, "container-metadata-file-validator", "ecs-functional-tests-container-metadata-file-validator")
}

// TestMemoryOvercommit tests the MemoryReservation of container can be configured in task definition
func TestMemoryOvercommit(t *testing.T) {
	ctx := context.TODO()
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	memoryReservation := int64(50)
	tdOverride := make(map[string]string)

	tdOverride["$$$$MEMORY_RESERVATION$$$$"] = strconv.FormatInt(memoryReservation, 10)
	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "memory-overcommit", tdOverride)
	require.NoError(t, err, "Error starting task")
	defer task.Stop()

	err = task.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for running task")

	containerId, err := agent.ResolveTaskDockerID(task, "memory-overcommit")
	require.NoError(t, err, "Error resolving docker id for container in task")

	containerMetaData, err := agent.DockerClient.ContainerInspect(ctx, containerId)
	require.NoError(t, err, "Could not inspect container for task")

	require.Equal(t, memoryReservation*1024*1024, containerMetaData.HostConfig.MemoryReservation,
		fmt.Sprintf("MemoryReservation in container metadata is not as expected: %v, expected: %v",
			containerMetaData.HostConfig.MemoryReservation, memoryReservation*1024*1024))
}

// TestNetworkModeHost tests the container network can be configured
// as host mode in task definition
func TestNetworkModeHost(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	err := networkModeTest(t, agent, "host")
	require.NoError(t, err, "Networking mode 'host' testing failed")
}

// TestNetworkModeBridge tests the container network can be configured
// as bridge mode in task definition
func TestNetworkModeBridge(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	err := networkModeTest(t, agent, "bridge")
	require.NoError(t, err, "Networking mode 'bridge' testing failed")
}

func TestNetworkModeAWSVPC(t *testing.T) {
	RequireDockerVersion(t, ">=17.06.0")
	agent := RunAgent(t, &AgentOptions{EnableTaskENI: true})
	defer agent.Cleanup()
	agent.RequireVersion(">=1.15.0")

	err := awsvpcNetworkModeTest(t, agent)
	require.NoError(t, err, "Networking mode 'awsvpc' testing failed")
}

// TestFluentdTag tests the fluentd logging driver option "tag"
func TestFluentdTag(t *testing.T) {
	// tag was added in docker 1.9.0
	RequireDockerVersion(t, ">=1.9.0")

	if runtime.GOOS == "arm64" {
		t.Skip()
	}

	fluentdDriverTest("fluentd-tag", t)
}

// TestFluentdLogTag tests the fluentd logging driver option "log-tag"
func TestFluentdLogTag(t *testing.T) {
	// fluentd was added in docker 1.8.0
	// and deprecated in 1.12.0
	RequireDockerVersion(t, ">=1.8.0")
	RequireDockerVersion(t, "<1.12.0")

	if runtime.GOOS == "arm64" {
		t.Skip()
	}

	fluentdDriverTest("fluentd-log-tag", t)
}

func fluentdDriverTest(taskDefinition string, t *testing.T) {
	ctx := context.TODO()

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["fluentd"]`,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	// fluentd is supported in agnet >=1.5.0
	agent.RequireVersion(">=1.5.0")

	driverTask, err := agent.StartTask(t, "fluentd-driver")
	require.NoError(t, err)

	err = driverTask.WaitRunning(2 * time.Minute)
	require.NoError(t, err)

	testTask, err := agent.StartTask(t, taskDefinition)
	require.NoError(t, err)

	err = testTask.WaitRunning(2 * time.Minute)
	assert.NoError(t, err)

	dockerID, err := agent.ResolveTaskDockerID(testTask, "fluentd-test")
	assert.NoError(t, err, "failed to resolve the container id from agent state")

	container, err := agent.DockerClient.ContainerInspect(ctx, dockerID)
	assert.NoError(t, err, "failed to inspect the container")

	logTag := fmt.Sprintf("ecs.%v.%v", strings.Replace(container.Name, "/", "", 1), dockerID)

	// clean up
	err = testTask.WaitStopped(1 * time.Minute)
	assert.NoError(t, err, "task failed to be stopped")

	driverTask.Stop()
	err = driverTask.WaitStopped(1 * time.Minute)
	assert.NoError(t, err, "task failed to be stopped")

	// Verify the log file existed and also the content contains the expected format
	err = SearchStrInDir(fluentdLogPath, "ecsfts", "hello, this is fluentd functional test")
	assert.NoError(t, err, "failed to find the content in the fluent log file")

	err = SearchStrInDir(fluentdLogPath, "ecsfts", logTag)
	assert.NoError(t, err, "failed to find the log tag specified in the task definition")
}

// TestAgentIntrospectionValidator tests that the agent introspection endpoint can
// be accessed from within the container.
func TestAgentIntrospectionValidator(t *testing.T) {
	// Best effort to create a log group. It should be safe to even not do this
	// as the log group gets created in the TestAWSLogsDriver functional test.
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	cwlClient.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(awslogsLogGroupName),
	})
	agent := RunAgent(t, &AgentOptions{
		EnableTaskENI: true,
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	})
	defer agent.Cleanup()
	agent.RequireVersion(">1.20.1")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "agent-introspection-validator", tdOverrides)
	require.NoError(t, err, "Unable to start task")
	defer func() {
		if err := task.Stop(); err != nil {
			return
		}
		task.WaitStopped(waitTaskStateChangeDuration)
	}()

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to transition to STOPPED")
	exitCode, _ := task.ContainerExitcode("agent-introspection-validator")

	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

func TestTaskMetadataValidator(t *testing.T) {
	RequireDockerVersion(t, ">=17.06.0-ce")

	// Added to test presence of tags in metadata endpoint
	// We need long container instance ARN for tagging APIs, PutAccountSettingInput
	// will enable long container instance ARN.
	putAccountSettingInput := ecsapi.PutAccountSettingInput{
		Name:  aws.String("containerInstanceLongArnFormat"),
		Value: aws.String("enabled"),
	}
	_, err := ECS.PutAccountSetting(&putAccountSettingInput)
	assert.NoError(t, err)

	// Best effort to create a log group. It should be safe to even not do this
	// as the log group gets created in the TestAWSLogsDriver functional test.
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	cwlClient.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(awslogsLogGroupName),
	})
	agent := RunAgent(t, &AgentOptions{
		EnableTaskENI: true,
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS":              `["awslogs"]`,
			"ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM": "ec2_instance",
			"ECS_CONTAINER_INSTANCE_TAGS": fmt.Sprintf(`{"%s": "%s"}`,
				"localKey", "localValue"),
		},
	})
	defer agent.Cleanup()
	agent.RequireVersion(">1.20.1")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverrides["$$$CHECK_TAGS$$$"] = "CheckTags" // Added to test presence of tags in metadata endpoint

	task, err := agent.StartAWSVPCTask("taskmetadata-validator-awsvpc", tdOverrides)
	require.NoError(t, err, "Unable to start task with 'awsvpc' network mode")
	defer func() {
		if err := task.Stop(); err != nil {
			return
		}
		task.WaitStopped(waitTaskStateChangeDuration)
	}()

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to transition to STOPPED")
	exitCode, _ := task.ContainerExitcode("taskmetadata-validator")

	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))

	DeleteAccountSettingInput := ecsapi.DeleteAccountSettingInput{
		Name: aws.String("containerInstanceLongArnFormat"),
	}
	_, err = ECS.DeleteAccountSetting(&DeleteAccountSettingInput)
	assert.NoError(t, err)
}

// TestExecutionRole verifies that task can use the execution credentials to pull from ECR and
// send logs to cloudwatch with awslogs driver
func TestExecutionRole(t *testing.T) {
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	RequireDockerVersion(t, ">=17.06.2-ce") // awslogs drivers with execution role available from docker 17.06.2
	accountID, err := GetAccountID()
	assert.NoError(t, err, "acquiring account id failed")

	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS":             `["awslogs"]`,
			"ECS_ENABLE_AWSLOGS_EXECUTIONROLE_OVERRIDE": "true",
		},
	}

	// Run the agent container with host network mode
	os.Setenv("ECS_FTEST_FORCE_NET_HOST", "true")
	defer os.Unsetenv("ECS_FTEST_FORCE_NET_HOST")

	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.16.0")

	tdOverrides := make(map[string]string)
	testImage := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/executionrole:fts", accountID, *ECS.Config.Region)

	tdOverrides["$$$$TEST_REGION$$$$"] = aws.StringValue(ECS.Config.Region)
	tdOverrides["$$$$EXECUTION_ROLE$$$$"] = os.Getenv("ECS_FTS_EXECUTION_ROLE")
	tdOverrides["$$$$IMAGE$$$$"] = testImage

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "execution-role", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/executionrole-awslogs-test/%s", taskID)),
	})

	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/executionrole-awslogs-test/%s", taskID)),
	}

	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 1, fmt.Sprintf("Get unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "hello world", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
	// Search the audit log to verify the credential request from awslogs driver
	err = SearchStrInDir(filepath.Join(agent.TestDir, "log"), "audit.log.", "GetCredentialsExecutionRole")
	err = SearchStrInDir(filepath.Join(agent.TestDir, "log"), "audit.log.", *testTask.TaskArn)
	require.NoError(t, err, "Verify credential request failed")
}

// TestAWSLogsDriverMultilinePattern verifies that multiple log lines with a certain
// pattern, specified using 'awslogs-multiline-pattern' option, are sent to a single
// CloudWatch log event
func TestAWSLogsDriverMultilinePattern(t *testing.T) {
	RequireDockerVersion(t, ">=17.06.0-ce")
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	// Test whether the log group exists or not
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
		require.NoError(t, err, fmt.Sprintf("Failed to create log group %s", awslogsLogGroupName))
	}

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.16.0") //Required for awslogs driver multiline pattern option

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs-multiline", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer func() {
		cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
			LogGroupName:  aws.String(awslogsLogGroupName),
			LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-multiline/%s", taskID)),
		})
	}()

	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-multiline/%s", taskID)),
	}
	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 2, fmt.Sprintf("Got unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "INFO: ECS Agent\nRunning\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
	assert.Equal(t, *resp.Events[1].Message, "INFO: Instance\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[1].Message))
}

// TestAWSLogsDriverDatetimeFormat verifies that multiple log lines with a certain
// pattern of timestamp, specified using 'awslogs-datetime-format' option, are sent
// to a single CloudWatch log event
func TestAWSLogsDriverDatetimeFormat(t *testing.T) {
	RequireDockerVersion(t, ">=17.06.0-ce")
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	// Test whether the log group exists or not
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
		require.NoError(t, err, fmt.Sprintf("Failed to create log group %s", awslogsLogGroupName))
	}

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.16.0") //Required for awslogs driver datetime format option

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "awslogs-datetime", tdOverrides)
	require.NoError(t, err, "Expected to start task using awslogs driver failed")

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)

	// Delete the log stream after the test
	defer func() {
		cwlClient.DeleteLogStream(&cloudwatchlogs.DeleteLogStreamInput{
			LogGroupName:  aws.String(awslogsLogGroupName),
			LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-datetime/%s", taskID)),
		})
	}()

	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(awslogsLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("ecs-functional-tests/awslogs-datetime/%s", taskID)),
	}
	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err, "CloudWatchLogs get log failed")
	assert.Len(t, resp.Events, 2, fmt.Sprintf("Got unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "May 01, 2017 19:00:01 ECS\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
	assert.Equal(t, *resp.Events[1].Message, "May 01, 2017 19:00:04 Agent\nRunning\nin the instance\n", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[1].Message))
}

// TestPrivateRegistryAuthOverASM tests the workflow for retriving private registry authentication data
// from AWS Secrets Manager
func TestPrivateRegistryAuthOverASM(t *testing.T) {
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	if IsCNPartition() {
		t.Skip("Skip TestPrivateRegistryAuthOverASM in China partition")
	}

	secretName := "FunctionalTest-PrivateRegistryAuth"
	asmClient := secretsmanager.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	input := &secretsmanager.CreateSecretInput{
		Description:  aws.String("Resource created for the ECS Agent Functional Test: TestPrivateRegistryAuthOverASM"),
		Name:         aws.String(secretName),
		SecretString: aws.String("{\"username\":\"user\",\"password\":\"swordfish\"}"),
	}

	// create secret value if it does not exist
	_, err := asmClient.CreateSecret(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeResourceExistsException:
				t.Logf("AWS Secrets Manager resource already exists")
				break
			default:
				require.NoError(t, err, "AWS Secrets Manager CreateSecret call failed")
			}
		}
	}

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.19.0")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$REPOSITORY_CREDENTIALS$$$"] = secretName
	tdOverrides["$$$EXECUTION_ROLE$$$"] = os.Getenv("ECS_FTS_EXECUTION_ROLE")

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "private-registry-auth-asm-validator-unix", tdOverrides)
	require.NoError(t, err, "Expected to start task using private registry authentication over asm failed")

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("private-registry-auth-asm-validator")
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

// TestContainerHealthMetrics tests the container health metrics was sent to backend
func TestContainerHealthMetrics(t *testing.T) {
	containerHealthWithoutStartPeriodTest(t, "container-health")
}

// TestContainerHealthMetricsWithStartPeriod tests the container health metrics
// with start period configured in the task definition
func TestContainerHealthMetricsWithStartPeriod(t *testing.T) {
	containerHealthWithStartPeriodTest(t, "container-health")
}

// TestTwoTasksSharedLocalVolume tests shared volume between two tasks
func TestTwoTasksSharedLocalVolume(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.20.0")

	// start writer task first
	wTask, err := agent.StartTask(t, "task-shared-vol-write")
	require.NoError(t, err, "Register task definition failed")

	// Wait for the first task to create the volume
	wErr := wTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, wErr, "Error waiting for task to transition to STOPPED")
	wExitCode, _ := wTask.ContainerExitcode("task-shared-vol-write")
	assert.Equal(t, 42, wExitCode, fmt.Sprintf("Expected exit code of 42; got %d", wExitCode))

	// then reader task try to read from the volume
	rTask, err := agent.StartTask(t, "task-shared-vol-read")
	require.NoError(t, err, "Register task definition failed")

	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, rErr, "Error waiting for task to transition to STOPPED")
	rExitCode, _ := rTask.ContainerExitcode("task-shared-vol-read")
	assert.Equal(t, 42, rExitCode, fmt.Sprintf("Expected exit code of 42; got %d", rExitCode))
}

// TestHostPIDNamespaceSharing tests the visibility of an executable running on one Task from
// another Task. Both tasks share their PID namespace with Host. Second Task should see the
// running executable.
func TestHostPIDNamespaceSharing(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	sTask, err := agent.StartTask(t, "pid-namespace-host-share")
	require.NoError(t, err, "Register task definition failed")
	sTask.WaitRunning(waitTaskStateChangeDuration)

	rTask, err := agent.StartTask(t, "pid-namespace-host-read")
	require.NoError(t, err, "Register task definition failed")
	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, rErr, "Error waiting for task to transition to STOPPED")
	rExitCode, _ := rTask.ContainerExitcode("pidConsumer")

	sErr := sTask.Stop()
	require.NoError(t, sErr, "Error stopping pid-namespace-host-share task")
	sErr = sTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, sErr, "Error waiting for pid-namespace-host-share task to stop")
	assert.Equal(t, 1, rExitCode, "Container could not see pidNamespaceTest process, but should")
}

// TestTaskPIDNamespaceSharing tests the visibility of an executable running on one Task from
// another Task. Both tasks share their PID namespace within their respective Tasks. Second
// Task should not see the running executable.
func TestTaskPIDNamespaceSharing(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	sTask, err := agent.StartTask(t, "pid-namespace-task-share")
	require.NoError(t, err, "Register task definition failed")
	sTask.WaitRunning(waitTaskStateChangeDuration)

	rTask, err := agent.StartTask(t, "pid-namespace-task-read")
	require.NoError(t, err, "Register task definition failed")
	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, rErr, "Error waiting for task to transition to STOPPED")
	rExitCode, _ := rTask.ContainerExitcode("pidConsumer")

	sErr := sTask.Stop()
	require.NoError(t, sErr, "Error stopping pid-namespace-task-share task")
	sErr = sTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, sErr, "Error waiting for pid-namespace-task-share task to stop")
	assert.Equal(t, 2, rExitCode, "Container could see pidNamespaceTest process, but shouldn't")
}

// TestHostIPCNamespaceSharing tests the visibility of an IPC semaphore created on one Task from
// another Task. Both tasks share their IPC namespace with Host. Second Task should see the
// created semaphore.
func TestHostIPCNamespaceSharing(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	sTask, err := agent.StartTask(t, "ipc-namespace-host-share")
	require.NoError(t, err, "Register task definition failed")
	sTask.WaitRunning(waitTaskStateChangeDuration)

	rTask, err := agent.StartTask(t, "ipc-namespace-host-read")
	require.NoError(t, err, "Register task definition failed")
	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, rErr, "Error waiting for task to transition to STOPPED")
	rExitCode, _ := rTask.ContainerExitcode("ipcConsumer")

	sErr := sTask.Stop()
	require.NoError(t, sErr, "Error stopping ipc-namespace-host-share task")
	sErr = sTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, sErr, "Error waiting for ipc-namespace-host-share task to stop")
	assert.Equal(t, 1, rExitCode, "Container could not see IPC resource, but should")
}

// TestTaskIPCNamespaceSharing tests the visibility of an IPC semaphore created on one Task from
// another Task. Both tasks share their IPC namespace within their respective Tasks. Second
// Task should not see the created semaphore.
func TestTaskIPCNamespaceSharing(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	sTask, err := agent.StartTask(t, "ipc-namespace-task-share")
	require.NoError(t, err, "Register task definition failed")
	sTask.WaitRunning(waitTaskStateChangeDuration)

	rTask, err := agent.StartTask(t, "ipc-namespace-task-read")
	require.NoError(t, err, "Register task definition failed")
	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, rErr, "Error waiting for task to transition to STOPPED")
	rExitCode, _ := rTask.ContainerExitcode("ipcConsumer")

	sErr := sTask.Stop()
	require.NoError(t, sErr, "Error stopping ipc-namespace-task-share task")
	sErr = sTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, sErr, "Error waiting for ipc-namespace-task-share task to stop")
	assert.Equal(t, 2, rExitCode, "Container could see IPC resource, but shouldn't")
}

// TestSSMSecretsNonEncryptedParameter tests the workflow for retrieving secrets from SSM Parameter Store,
// here secret is a non encrypted parameter
func TestSSMSecretsNonEncryptedParameterARN(t *testing.T) {
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	executionRole := os.Getenv("ECS_FTS_EXECUTION_ROLE")
	// execution role arn is following the pattern arn:aws:iam::accountId:role/***
	executionRoleArr := strings.Split(executionRole, ":")
	partition := executionRoleArr[1]
	accountId := executionRoleArr[4]

	parameterName := "FunctionalTest-SSMSecretsString"
	secretName := "SECRET_NAME"
	region := *ECS.Config.Region
	ssmClient := ssm.New(session.New(), aws.NewConfig().WithRegion(region))
	input := &ssm.PutParameterInput{
		Description: aws.String("Resource created for the ECS Agent Functional Test: TestSSMSecretsNonEncryptedParameter"),
		Name:        aws.String(parameterName),
		Value:       aws.String("secretValue"),
		Type:        aws.String("String"),
	}

	// create parameter in parameter store if it does not exist
	_, err := ssmClient.PutParameter(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ssm.ErrCodeParameterAlreadyExists:
				t.Logf("Parameter %v already exists in SSM Parameter Store", parameterName)
				break
			default:
				require.NoError(t, err, "SSM PutParameter call failed")
			}
		}
	}

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.22.0")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$SECRET_NAME$$$"] = secretName

	arn := fmt.Sprintf("arn:%s:ssm:%s:%s:parameter/%s", partition, region, accountId, parameterName)
	tdOverrides["$$$SECRET_VALUE_FROM$$$"] = arn
	tdOverrides["$$$EXECUTION_ROLE$$$"] = executionRole

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "secrets-environment-variables", tdOverrides)
	require.NoError(t, err, "Failed to start task for ssmsecrets environment variables")

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("secrets-environment-variables")
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

// TestSSMSecretsEncryptedParameter tests the workflow for retrieving secrets from SSM Parameter Store,
// here secret is an encrypted parameter
func TestSSMSecretsEncryptedParameter(t *testing.T) {
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	if IsCNPartition() {
		t.Skip("Skip TestSSMSecretsEncryptedParameter in China partition")
	}

	parameterName := "FunctionalTest-SSMSecretsSecureString"
	secretName := "SECRET_NAME"
	ssmClient := ssm.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	input := &ssm.PutParameterInput{
		Description: aws.String("Resource created for the ECS Agent Functional Test: TestSSMSecretsEncryptedParameter"),
		Name:        aws.String(parameterName),
		Value:       aws.String("secretValue"),
		Type:        aws.String("SecureString"),
	}

	// create parameter in parameter store if it does not exist
	_, err := ssmClient.PutParameter(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ssm.ErrCodeParameterAlreadyExists:
				t.Logf("Parameter %v already exists in SSM Parameter Store", parameterName)
				break
			default:
				require.NoError(t, err, "SSM PutParameter call failed")
			}
		}
	}

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.22.0")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$SECRET_NAME$$$"] = secretName
	tdOverrides["$$$SECRET_VALUE_FROM$$$"] = parameterName
	tdOverrides["$$$EXECUTION_ROLE$$$"] = os.Getenv("ECS_FTS_EXECUTION_ROLE")

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "secrets-environment-variables", tdOverrides)
	require.NoError(t, err, "Failed to start task for secrets environment variables")

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("secrets-environment-variables")
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

// TestSSMSecretsEncryptedASMSecrets tests the workflow for retrieving secrets from SSM Parameter Store,
// here secret is a secret in secrets manager passing through parameter store
func TestSSMSecretsEncryptedASMSecrets(t *testing.T) {
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	if IsCNPartition() {
		t.Skip("Skip TestSSMSecretsEncryptedParameter in China partition")
	}

	parameterName := "/aws/reference/secretsmanager/FunctionalTest-SSMSecretsSecretFromASM"
	secretName := "SECRET_NAME"
	asmClient := secretsmanager.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	input := &secretsmanager.CreateSecretInput{
		Description:  aws.String("Resource created for the ECS Agent Functional Test: TestSSMSecretsEncryptedASMSecrets"),
		Name:         aws.String("FunctionalTest-SSMSecretsSecretFromASM"),
		SecretString: aws.String("secretValue"),
	}

	// create parameter in parameter store if it does not exist
	_, err := asmClient.CreateSecret(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeResourceExistsException:
				t.Logf("Secret FunctionalTest-SSMSecretsSecretFromASM already exists in AWS Secrets Manager")
				break
			default:
				require.NoError(t, err, "Secrets Manager CreateSecret call failed")
			}
		}
	}

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.22.0")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$SECRET_NAME$$$"] = secretName
	tdOverrides["$$$SECRET_VALUE_FROM$$$"] = parameterName
	tdOverrides["$$$EXECUTION_ROLE$$$"] = os.Getenv("ECS_FTS_EXECUTION_ROLE")

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "secrets-environment-variables", tdOverrides)
	require.NoError(t, err, "Failed to start task for secrets environment variables")

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("secrets-environment-variables")
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

// TestASMSecretsARN tests the workflow for retrieving secrets directly from AWS Secrets Manager.
func TestASMSecretsARN(t *testing.T) {
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	if IsCNPartition() {
		t.Skip("Skip TestASMSecretsARN in China partition")
	}

	secret := "FunctionalTest-SSMSecretsSecretFromASM"
	asmClient := secretsmanager.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	input := &secretsmanager.CreateSecretInput{
		Description:  aws.String("Resource created for the ECS Agent Functional Test: TestASMSecretsARN"),
		Name:         aws.String(secret),
		SecretString: aws.String("secretValue"),
	}

	// create secret in secrets manager if it does not exist
	_, err := asmClient.CreateSecret(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeResourceExistsException:
				t.Logf("Secret %s already exists in AWS Secrets Manager", secret)
				break
			default:
				require.NoError(t, err, "Secrets Manager CreateSecret call failed")
			}
		}
	}

	// get secret arn
	secretInput := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secret),
	}
	res, err := asmClient.GetSecretValue(secretInput)
	if err != nil {
		require.NoError(t, err, "Secrets Manager GetSecretValue call failed")
	}
	secretARN := aws.StringValue(res.ARN)

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.23.0")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$SECRET_NAME$$$"] = "SECRET_NAME"
	tdOverrides["$$$SECRET_VALUE_FROM$$$"] = secretARN
	tdOverrides["$$$EXECUTION_ROLE$$$"] = os.Getenv("ECS_FTS_EXECUTION_ROLE")

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, "secrets-environment-variables", tdOverrides)
	require.NoError(t, err, "Failed to start task for secrets environment variables")

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err)
	exitCode, _ := task.ContainerExitcode("secrets-environment-variables")
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}
