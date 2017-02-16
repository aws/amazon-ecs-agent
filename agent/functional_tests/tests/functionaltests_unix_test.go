// +build !windows,functional

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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	ecsapi "github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/pborman/uuid"
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
	RequireDockerVersion(t, "<1.9.0,>1.9.1") // https://github.com/docker/docker/issues/18510
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "oom-container")
	require.NoError(t, err, "Expected to start invalid-image task")
	err = testTask.ExpectErrorType("error", "OutOfMemoryError", 1*time.Minute)
	assert.NoError(t, err)
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
		require.NoError(t, err, fmt.Sprintf("Cycle %d: There was an error starting the Task", i))

		isTaskRunning, err := agent.WaitRunningViaIntrospection(testTask)
		require.NoError(t, err, "Waiting for task running failed")
		require.True(t, isTaskRunning, fmt.Sprintf("Cycle %d: Task should be RUNNING but is not", i))

		// Get the dockerID so we can later check that the container has been cleaned up.
		dockerId, err := agent.ResolveTaskDockerID(testTask, "1")
		require.NoError(t, err, fmt.Sprintf("Cycle %d: Error resolving docker id for container in task", i))

		// 2 minutes should be enough for the Task to have completed. If the task has not
		// completed and is in PENDING, the agent is most likely deadlocked.
		err = testTask.WaitStopped(2 * time.Minute)
		require.NoError(t, err, fmt.Sprintf("Cycle %d: Task did not transition into to STOPPED in time", i))

		isTaskStopped, err := agent.WaitStoppedViaIntrospection(testTask)
		require.NoError(t, err, "Waiting for task stopped failed")
		require.True(t, isTaskStopped, fmt.Sprintf("Cycle %d: Task should be STOPPED but is not", i))

		// Wait for the tasks to be cleaned up
		time.Sleep(90 * time.Second)

		// Ensure that tasks are cleaned up. WWe should not be able to describe the
		// container now since it has been cleaned up.
		_, err = agent.DockerClient.InspectContainer(dockerId)
		require.Error(t, err, fmt.Sprintf("Cycle %d: Expected error inspecting container in task.", i))
	}
}

func strptr(s string) *string { return &s }

func TestCommandOverrides(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	task, err := agent.StartTaskWithOverrides(t, "simple-exit", []*ecsapi.ContainerOverride{
		&ecsapi.ContainerOverride{
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
			// Because of this, strip down to just the comma-seperated hex and look for that
			if strings.Contains(string(data), gobytes[len(`[]byte{`):len(gobytes)-1]) {
				t.Fatalf("log data contained byte-hex representation of bad string: %v, %v", string(data), badstring)
			}
		}
		return nil
	})

	assert.NoError(t, err, "Could not walk logdir")
}

func TestSquidProxy(t *testing.T) {
	// Run a squid proxy manually, verify that the agent can connect through it
	client, err := docker.NewVersionedClientFromEnv("1.17")
	require.NoError(t, err)

	squidImage := "127.0.0.1:51670/amazon/squid:latest"
	dockerConfig := docker.Config{
		Image: squidImage,
	}
	dockerHostConfig := docker.HostConfig{}

	err = client.PullImage(docker.PullImageOptions{Repository: squidImage}, docker.AuthConfiguration{})
	require.NoError(t, err)

	squidContainer, err := client.CreateContainer(docker.CreateContainerOptions{
		Config:     &dockerConfig,
		HostConfig: &dockerHostConfig,
	})
	require.NoError(t, err)
	err = client.StartContainer(squidContainer.ID, &dockerHostConfig)
	require.NoError(t, err)
	defer func() {
		client.RemoveContainer(docker.RemoveContainerOptions{
			Force:         true,
			ID:            squidContainer.ID,
			RemoveVolumes: true,
		})
	}()

	// Resolve the name so we can use it in the link below; the create returns an ID only
	squidContainer, err = client.InspectContainer(squidContainer.ID)
	require.NoError(t, err)

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
	logExec, err := client.CreateExec(docker.CreateExecOptions{
		AttachStdout: true,
		AttachStdin:  false,
		Container:    squidContainer.ID,
		// Takes a second to flush the file sometimes, so slightly complicated command to wait for it to be written
		Cmd: []string{"sh", "-c", "FILE=/var/log/squid/access.log; while [ ! -s $FILE ]; do sleep 1; done; cat $FILE"},
	})
	require.NoError(t, err)
	t.Logf("Execing cat of /var/log/squid/access.log on %v", squidContainer.ID)

	var squidLogs bytes.Buffer
	err = client.StartExec(logExec.ID, docker.StartExecOptions{
		OutputStream: &squidLogs,
	})
	require.NoError(t, err)
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

// TestAwslogsDriver verifies that container logs are sent to Amazon CloudWatch Logs with awslogs as the log driver
func TestAwslogsDriver(t *testing.T) {
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
	require.NoError(t, err, "CloudWatchLogs get log failed")

	assert.Len(t, resp.Events, 1, fmt.Sprintf("Get unexpected number of log events: %d", len(resp.Events)))
	assert.Equal(t, *resp.Events[0].Message, "hello world", fmt.Sprintf("Got log events message unexpected: %s", *resp.Events[0].Message))
}

// TestTelemetry tests whether agent can send metrics to TACS
func TestTelemetry(t *testing.T) {
	// Try to use a new cluster for this test, ensure no other task metrics for this cluster
	newClusterName := "ecstest-telemetry-" + uuid.New()
	_, err := ECS.CreateCluster(&ecsapi.CreateClusterInput{
		ClusterName: aws.String(newClusterName),
	})
	require.NoError(t, err, "Failed to create cluster")
	defer DeleteCluster(t, newClusterName)

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_CLUSTER": newClusterName,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	params := &cloudwatch.GetMetricStatisticsInput{
		MetricName: aws.String("CPUUtilization"),
		Namespace:  aws.String("AWS/ECS"),
		Period:     aws.Int64(60),
		Statistics: []*string{
			aws.String("Average"),
			aws.String("SampleCount"),
		},
		Dimensions: []*cloudwatch.Dimension{
			{
				Name:  aws.String("ClusterName"),
				Value: aws.String(newClusterName),
			},
		},
	}
	params.StartTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.EndTime = aws.Time((*params.StartTime).Add(waitMetricsInCloudwatchDuration).UTC())
	// wait for the agent start and ensure no task is running
	time.Sleep(waitMetricsInCloudwatchDuration)

	cwclient := cloudwatch.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Before task running, verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Before task running, verify metrics for memory utilization failed")

	testTask, err := agent.StartTask(t, "telemetry")
	require.NoError(t, err, "Failed to start telemetry task")
	// Wait for the task to run and the agent to send back metrics
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error wait telemetry task running")

	time.Sleep(waitMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitMetricsInCloudwatchDuration).UTC())
	params.MetricName = aws.String("CPUUtilization")
	err = VerifyMetrics(cwclient, params, false)
	assert.NoError(t, err, "Task is running, verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	err = VerifyMetrics(cwclient, params, false)
	assert.NoError(t, err, "Task is running, verify metrics for memory utilization failed")

	err = testTask.Stop()
	require.NoError(t, err, "Failed to stop the telemetry task")

	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Waiting for task stop failed")

	time.Sleep(waitMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitMetricsInCloudwatchDuration).UTC())
	params.MetricName = aws.String("CPUUtilization")
	err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Task stopped: verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Task stopped, verify metrics for memory utilization failed")
}

func TestTaskIamRolesNetHostMode(t *testing.T) {
	// The test runs only when the environment TEST_IAM_ROLE was set
	if os.Getenv("TEST_TASK_IAM_ROLE_NET_HOST") != "true" {
		t.Skip("Skipping test TaskIamRole in host network mode, as TEST_TASK_IAM_ROLE_NET_HOST isn't set true")
	}
	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST": "true",
			"ECS_ENABLE_TASK_IAM_ROLE":              "true",
		},
		PortBindings: map[docker.Port]map[string]string{
			"51679/tcp": map[string]string{
				"HostIP":   "0.0.0.0",
				"HostPort": "51679",
			},
		},
	}
	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	taskIamRolesTest("host", agent, t)
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
		PortBindings: map[docker.Port]map[string]string{
			"51679/tcp": map[string]string{
				"HostIP":   "0.0.0.0",
				"HostPort": "51679",
			},
		},
	}
	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	taskIamRolesTest("bridge", agent, t)
}

func taskIamRolesTest(networkMode string, agent *TestAgent, t *testing.T) {
	RequireDockerVersion(t, ">=1.11.0") // TaskIamRole is available from agent 1.11.0
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

	// TaskIAMRoles enabled contaienr should have the ExtraEnvironment variable AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
	containerMetaData, err := agent.DockerClient.InspectContainer(containerId)
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
		t.Fatalf("Could not found AWS_CONTAINER_CREDENTIALS_RELATIVE_URI in the container envrionment variable")
	}

	// Task will only run one command "aws ec2 describe-regions"
	err = task.WaitStopped(30 * time.Second)
	require.NoError(t, err, "Waiting task to stop error")

	containerMetaData, err = agent.DockerClient.InspectContainer(containerId)
	require.NoError(t, err, "Could not inspect container for task")

	require.Equal(t, 0, containerMetaData.State.ExitCode, fmt.Sprintf("Container exit code non-zero: %v", containerMetaData.State.ExitCode))

	// Search the audit log to verify the credential request
	err = SearchStrInDir(filepath.Join(agent.TestDir, "log"), "audit.log.", *task.TaskArn)
	require.NoError(t, err, "Verify credential request failed")
}

// TestMemoryOvercommit tests the MemoryReservation of container can be configured in task definition
func TestMemoryOvercommit(t *testing.T) {
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

	containerMetaData, err := agent.DockerClient.InspectContainer(containerId)
	require.NoError(t, err, "Could not inspect container for task")

	require.Equal(t, memoryReservation*1024*1024, containerMetaData.HostConfig.MemoryReservation,
		fmt.Sprintf("MemoryReservation in container metadata is not as expected: %v, expected: %v",
			containerMetaData.HostConfig.MemoryReservation, memoryReservation*1024*1024))
}

// TestNetworkModeBridge tests the container network can be configured
// as host mode in task definition
func TestNetworkModeHost(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	err := networkModeTest(t, agent, "host")
	require.NoError(t, err, "Networking mode host testing failed")
}

// TestNetworkModeBridge tests the container network can be configured
// as bridge mode in task definition
func TestNetworkModeBridge(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	err := networkModeTest(t, agent, "bridge")
	require.NoError(t, err, "Networking mode bridge testing failed")
}
