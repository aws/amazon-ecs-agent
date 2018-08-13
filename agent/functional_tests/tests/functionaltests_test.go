// +build functional

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
	"os"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	ecsapi "github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/docker/docker/pkg/system"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	waitTaskStateChangeDuration     = 2 * time.Minute
	waitMetricsInCloudwatchDuration = 4 * time.Minute
	awslogsLogGroupName             = "ecs-functional-tests"

	// 'awsvpc' test parameters
	awsvpcTaskDefinition     = "nginx-awsvpc"
	awsvpcIPv4AddressKey     = "privateIPv4Address"
	awsvpcTaskRequestTimeout = 5 * time.Second
	cpuSharesPerCore         = 1024
	bytePerMegabyte          = 1024 * 1024
)

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

// TestSavedState verifies that stopping the agent, stopping a container under
// its control, and starting the agent results in that container being moved to
// 'stopped'
func TestSavedState(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()
	testTask, err := agent.StartTask(t, savedStateTaskDefinition)
	if err != nil {
		t.Fatal(err)
	}
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	if err != nil {
		t.Fatal(err)
	}

	dockerId, err := agent.ResolveTaskDockerID(testTask, savedStateTaskDefinition)
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

	testTask.WaitStopped(waitTaskStateChangeDuration)
}

// TestSavedStateWithInvalidImageAndCleanup verifies that a task definition with an invalid image does not prevent the
// agnet from starting again after the task has been cleaned up.  See
// https://github.com/aws/amazon-ecs-agent/issues/1024 for details.
func TestSavedStateWithInvalidImageAndCleanup(t *testing.T) {
	// Set the task cleanup time to just over a minute.
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "70s")
	agent := RunAgent(t, nil)
	defer func() {
		agent.Cleanup()
		os.Unsetenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")
	}()

	testTask, err := agent.StartTask(t, "invalid-image")
	require.NoError(t, err, "failed to start task")
	assert.NoError(t, testTask.ExpectErrorType("error", "CannotPullContainerError", 1*time.Minute))

	resp, err := agent.CallTaskIntrospectionAPI(testTask)
	assert.NoError(t, err, "should be able to introspect the task")
	assert.NotNil(t, resp, "should receive a response")
	assert.Equal(t, *testTask.TaskArn, resp.Arn, "arn should be equal")

	// wait two minutes for it to be cleaned up
	fmt.Println("Sleeping...")
	time.Sleep(2 * time.Minute)

	resp, err = agent.CallTaskIntrospectionAPI(testTask)
	assert.NoError(t, err, "should be able to call introspection api") // is there a reason we don't 404?
	assert.NotNil(t, resp, "should receive a response")                // why?
	assert.Equal(t, "", resp.Arn, "arn is blank")

	err = agent.StopAgent()
	require.NoError(t, err, "failed to stop agent")

	err = agent.StartAgent()
	require.NoError(t, err, "failed to start agent again")
}

// TestPortResourceContention verifies that running two tasks on the same port
// in quick-succession does not result in the second one failing to run. It
// verifies the 'seqnum' serialization stuff works.
func TestPortResourceContention(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, portResContentionTaskDefinition)
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

	testTask2, err := agent.StartTask(t, portResContentionTaskDefinition)
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

	task, err := agent.StartTask(t, labelsTaskDefinition)
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

	task, err := agent.StartTask(t, logDriverTaskDefinition)
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

func TestTaskCleanup(t *testing.T) {
	// Set the task cleanup time to just over a minute.
	os.Setenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION", "70s")
	agent := RunAgent(t, nil)
	defer func() {
		agent.Cleanup()
		os.Unsetenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")
	}()

	// Start a task and get the container id once the task transitions to RUNNING.
	task, err := agent.StartTask(t, cleanupTaskDefinition)
	if err != nil {
		t.Fatalf("Error starting task: %v", err)
	}

	err = task.WaitRunning(2 * time.Minute)
	if err != nil {
		t.Fatalf("Error waiting for running task: %v", err)
	}

	dockerId, err := agent.ResolveTaskDockerID(task, cleanupTaskDefinition)
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

// TestNetworkModeNone tests if the 'none' contaienr network mode is configured
// correctly in task definition
func TestNetworkModeNone(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	err := networkModeTest(t, agent, "none")
	if err != nil {
		t.Fatalf("Networking mode none testing failed, err: %v", err)
	}
}

func networkModeTest(t *testing.T, agent *TestAgent, mode string) error {
	tdOverride := make(map[string]string)

	// Test the host network mode
	tdOverride["$$$$NETWORK_MODE$$$$"] = mode
	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, networkModeTaskDefinition, tdOverride)
	if err != nil {
		return fmt.Errorf("error starting task with network %v, err: %v", mode, err)
	}
	defer task.Stop()

	err = task.WaitRunning(waitTaskStateChangeDuration)
	if err != nil {
		return fmt.Errorf("error waiting for task running, err: %v", err)
	}
	containerId, err := agent.ResolveTaskDockerID(task, "network-"+mode)
	if err != nil {
		return fmt.Errorf("error resolving docker id for container \"network-%s\": %v", mode, err)
	}

	networks, err := agent.GetContainerNetworkMode(containerId)
	if err != nil {
		return err
	}
	if len(networks) != 1 {
		return fmt.Errorf("found multiple networks in container config")
	}
	if networks[0] != mode {
		return fmt.Errorf("did not found the expected network mode")
	}
	return nil
}

// awsvpcNetworkModeTest tests if the 'awsvpc' network mode works properly
func awsvpcNetworkModeTest(t *testing.T, agent *TestAgent) error {
	// Start task with network mode set to 'awsvpc'
	task, err := agent.StartAWSVPCTask(awsvpcTaskDefinition, nil)
	if err != nil {
		return fmt.Errorf("unable to start task with 'awsvpc' network mode: %v", err)
	}
	defer func() {
		if err := task.Stop(); err != nil {
			return
		}
		task.WaitStopped(2 * time.Minute)
	}()

	// Wait for task to be running
	err = task.WaitRunning(waitTaskStateChangeDuration)
	if err != nil {
		return fmt.Errorf("error waiting for task running, err: %v", err)
	}

	return nil
}

// TestCustomAttributesWithMaxOptions tests the ECS_INSTANCE_ATTRIBUTES
// upon agent registration with maximum number of supported key, value pairs
func TestCustomAttributesWithMaxOptions(t *testing.T) {
	maxAttributes := 10
	customAttributes := `{
                "key1": "val1",
                "key2": "val2",
                "key3": "val3",
                "key4": "val4",
                "key5": "val5",
                "key6": "val6",
                "key7": "val7",
                "key8": "val8",
                "key9": "val9",
                "key0": "val0"
        }`
	os.Setenv("ECS_INSTANCE_ATTRIBUTES", customAttributes)
	defer os.Unsetenv("ECS_INSTANCE_ATTRIBUTES")

	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	params := &ecsapi.DescribeContainerInstancesInput{
		Cluster:            &agent.Cluster,
		ContainerInstances: []*string{&agent.ContainerInstanceArn},
	}

	resp, err := ECS.DescribeContainerInstances(params)
	require.NoError(t, err)
	require.NotEmpty(t, resp.ContainerInstances)
	require.Len(t, resp.ContainerInstances, 1)

	attribMap := AttributesToMap(resp.ContainerInstances[0].Attributes)
	assert.NotEmpty(t, attribMap)

	for i := 0; i < maxAttributes; i++ {
		k := "key" + strconv.Itoa(i)
		v := "val" + strconv.Itoa(i)
		assert.Equal(t, v, attribMap[k], "Values should match")
	}

	_, ok := attribMap["ecs.os-type"]
	assert.True(t, ok, "OS attribute not found")
}

func waitForContainerHealthStatus(t *testing.T, testTask *TestTask) {
	ctx, _ := context.WithTimeout(context.TODO(), waitTaskStateChangeDuration)
	for {
		select {
		case <-ctx.Done():
			t.Error("Timed out waiting for container health status")
		default:
			testTask.Redescribe()
			if aws.StringValue(testTask.Containers[0].HealthStatus) == "UNKNOWN" {
				time.Sleep(time.Second)
				continue
			}
			return
		}
	}
}

func containerHealthWithoutStartPeriodTest(t *testing.T, taskDefinition string) {
	RequireDockerVersion(t, ">=1.12.0") // container health check was added in Docker 1.12.0
	// StartPeriod of container health check was added in 17.05.0,
	// don't test it here, it should be tested in containerHealthWithStartPeriodTest
	RequireDockerAPIVersion(t, "<1.29")

	tdOverrides := map[string]string{
		"$$$$START_PERIOD$$$$": "",
	}

	containerHealthMetricsTest(t, taskDefinition, tdOverrides)
}

func containerHealthWithStartPeriodTest(t *testing.T, taskDefinition string) {
	RequireDockerAPIVersion(t, ">=1.29") // StartPeriod of container health check was added in 17.05.0

	tdOverrides := map[string]string{
		"$$$$START_PERIOD$$$$": `"startPeriod": 1,`,
	}

	containerHealthMetricsTest(t, taskDefinition, tdOverrides)
}

func telemetryTest(t *testing.T, taskDefinition string) {
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
	_, err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Before task running, verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	_, err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Before task running, verify metrics for memory utilization failed")

	cpuNum := runtime.NumCPU()

	tdOverrides := make(map[string]string)
	// Set the container cpu percentage 25%
	tdOverrides["$$$$CPUSHARE$$$$"] = strconv.Itoa(int(float64(cpuNum*cpuSharesPerCore) * 0.25))

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, taskDefinition, tdOverrides)
	require.NoError(t, err, "Failed to start telemetry task")
	// Wait for the task to run and the agent to send back metrics
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error wait telemetry task running")

	time.Sleep(waitMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitMetricsInCloudwatchDuration).UTC())
	params.MetricName = aws.String("CPUUtilization")
	metrics, err := VerifyMetrics(cwclient, params, false)
	assert.NoError(t, err, "Task is running, verify metrics for CPU utilization failed")
	// Also verify the cpu usage is around 25% +/- 5%
	assert.InDelta(t, 25, *metrics.Average, 5)

	params.MetricName = aws.String("MemoryUtilization")
	metrics, err = VerifyMetrics(cwclient, params, false)
	assert.NoError(t, err, "Task is running, verify metrics for memory utilization failed")
	memInfo, err := system.ReadMemInfo()
	require.NoError(t, err, "Acquiring system info failed")
	totalMemory := memInfo.MemTotal / bytePerMegabyte
	// Verify the memory usage is around 1024/totalMemory +/- 5%
	assert.InDelta(t, float32(1024*100)/float32(totalMemory), *metrics.Average, 5)

	err = testTask.Stop()
	require.NoError(t, err, "Failed to stop the telemetry task")

	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Waiting for task stop failed")

	time.Sleep(waitMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitMetricsInCloudwatchDuration).UTC())
	params.MetricName = aws.String("CPUUtilization")
	_, err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Task stopped: verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	_, err = VerifyMetrics(cwclient, params, true)
	assert.NoError(t, err, "Task stopped, verify metrics for memory utilization failed")
}

// containerHealthMetricsTest tests the container health metrics based on the task definition
func containerHealthMetricsTest(t *testing.T, taskDefinition string, overrides map[string]string) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	agent.RequireVersion(">1.16.2") //Required for container health check option

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, taskDefinition, overrides)
	require.NoError(t, err, "expect task to be started without error")

	testTask.WaitRunning(waitTaskStateChangeDuration)

	waitForContainerHealthStatus(t, testTask)
	assert.Equal(t, aws.StringValue(testTask.Containers[0].HealthStatus), "HEALTHY", "container health status is not HEALTHY")
	err = testTask.Stop()
	assert.NoError(t, err, "stop task failed")
	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, err, "waiting for task stopped failed")
}

// waitCloudwatchLogs wait until the logs has been sent to cloudwatchlogs
func waitCloudwatchLogs(client *cloudwatchlogs.CloudWatchLogs, params *cloudwatchlogs.GetLogEventsInput) (*cloudwatchlogs.GetLogEventsOutput, error) {
	// The test could fail for timing issue, so retry for 30 seconds to make this test more stable
	for i := 0; i < 30; i++ {
		resp, err := client.GetLogEvents(params)
		if err != nil {
			awsError, ok := err.(awserr.Error)
			if !ok || awsError.Code() != "ResourceNotFoundException" {
				return nil, err
			}
		} else if len(resp.Events) > 0 {
			return resp, nil
		}
		time.Sleep(time.Second)
	}

	return nil, fmt.Errorf("Timeout waiting for the logs to be sent to cloud watch logs")
}
