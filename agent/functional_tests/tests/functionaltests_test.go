// +build functional

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	ecsapi "github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/go-connections/nat"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	waitTaskStateChangeDuration            = 2 * time.Minute
	waitIdleMetricsInCloudwatchDuration    = 4 * time.Minute
	waitMinimalMetricsInCloudwatchDuration = 5 * time.Minute
	waitBusyMetricsInCloudwatchDuration    = 10 * time.Minute
	awslogsLogGroupName                    = "ecs-functional-tests"

	// Even when the test cluster is deleted, TACS could still post cluster
	// metrics to CW due to eventual consistency for upto a maximum of 2 minutes.
	// While doing this, it would recreate log groups even after their manual deletion.
	// Hence, the wait before deleting the tests' log groups.
	waitTimeBeforeDeletingCILogGroups = 2 * time.Minute

	// 'awsvpc' test parameters
	awsvpcTaskDefinition     = "nginx-awsvpc"
	awsvpcIPv4AddressKey     = "privateIPv4Address"
	awsvpcTaskRequestTimeout = 5 * time.Second
	cpuSharesPerCore         = 1024
	bytePerMegabyte          = 1024 * 1024
	minimumCPUShares         = 128
	maximumCPUShares         = 10240
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
	ctx := context.TODO()
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

	containerStopTimeout := 1 * time.Second
	err = agent.DockerClient.ContainerStop(ctx, dockerId, &containerStopTimeout)
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

func calculateCpuLimits(cpuPercentage float64) (int, float64) {
	cpuNum := runtime.NumCPU()
	// Try to let the container use 25% cpu, but bound it within valid range
	cpuShare := int(float64(cpuNum*cpuSharesPerCore) * cpuPercentage)
	if cpuShare < minimumCPUShares {
		cpuShare = minimumCPUShares
	} else if cpuShare > maximumCPUShares {
		cpuShare = maximumCPUShares
	}
	expectedCPUPercentage := float64(cpuShare) / float64(cpuNum*cpuSharesPerCore)

	return cpuShare, expectedCPUPercentage
}

func telemetryTest(t *testing.T, taskDefinition string) {
	// telemetry task requires 2GB of memory (for either linux or windows); requires a bit more to be stable
	RequireMinimumMemory(t, 2200)

	// Try to let the container use 25% cpu, but bound it within valid range
	cpuShare, expectedCPUPercentage := calculateCpuLimits(0.25)

	// account for docker stats / CloudWatch noise
	statsNoiseDelta := 5.0

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

	cwclient := cloudwatch.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	params := &cloudwatch.GetMetricStatisticsInput{
		Namespace: aws.String("AWS/ECS"),
		Period:    aws.Int64(60),
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

	// collect and validate idle state metrics
	params.StartTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.EndTime = aws.Time((*params.StartTime).Add(waitIdleMetricsInCloudwatchDuration).UTC())
	time.Sleep(waitIdleMetricsInCloudwatchDuration)

	params.MetricName = aws.String("CPUUtilization")
	_, err = VerifyMetrics(cwclient, params, true, statsNoiseDelta)
	assert.NoError(t, err, "Before task running, verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	_, err = VerifyMetrics(cwclient, params, true, statsNoiseDelta)
	assert.NoError(t, err, "Before task running, verify metrics for memory utilization failed")

	// start telemetry task
	tdOverrides := make(map[string]string)
	tdOverrides["$$$$CPUSHARE$$$$"] = strconv.Itoa(cpuShare)
	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, taskDefinition, tdOverrides)
	require.NoError(t, err, "Failed to start telemetry task")
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error wait telemetry task running")

	// collect and validate busy state metrics
	time.Sleep(waitBusyMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitBusyMetricsInCloudwatchDuration).UTC())

	params.MetricName = aws.String("CPUUtilization")
	metricsAverage, err := VerifyMetrics(cwclient, params, false, 0.0)
	assert.NoError(t, err, "Task is running, verify metrics for CPU utilization failed")
	// Also verify the cpu usage is around expectedCPUPercentage
	// +/- StatsNoiseDelta percentage
	assert.InDelta(t, expectedCPUPercentage*100.0, metricsAverage, statsNoiseDelta)

	params.MetricName = aws.String("MemoryUtilization")
	metricsAverage, err = VerifyMetrics(cwclient, params, false, 0.0)
	assert.NoError(t, err, "Task is running, verify metrics for memory utilization failed")
	// Verify the memory usage is around 1024/totalMemory
	// +/- StatsNoiseDelta percentage
	memInfo, err := system.ReadMemInfo()
	require.NoError(t, err, "Acquiring system info failed")
	totalMemory := memInfo.MemTotal / bytePerMegabyte
	assert.InDelta(t, float32(1024*100)/float32(totalMemory), metricsAverage, statsNoiseDelta)

	// stop telemetry task
	err = testTask.Stop()
	require.NoError(t, err, "Failed to stop the telemetry task")
	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Waiting for task stop failed")

	// collect and validate idle state metrics
	time.Sleep(waitIdleMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitIdleMetricsInCloudwatchDuration).UTC())

	params.MetricName = aws.String("CPUUtilization")
	_, err = VerifyMetrics(cwclient, params, true, statsNoiseDelta)
	assert.NoError(t, err, "Task stopped: verify metrics for CPU utilization failed")

	params.MetricName = aws.String("MemoryUtilization")
	_, err = VerifyMetrics(cwclient, params, true, statsNoiseDelta)
	assert.NoError(t, err, "Task stopped, verify metrics for memory utilization failed")
}

func telemetryTestWithStatsPolling(t *testing.T, taskDefinition string) {
	// telemetry task requires 2GB of memory (for either linux or windows); requires a bit more to be stable
	RequireMinimumMemory(t, 2200)

	// Try to let the container use 25% cpu, but bound it within valid range
	cpuShare, expectedCPUPercentage := calculateCpuLimits(0.25)

	// account for docker stats / CloudWatch noise
	statsNoiseDelta := 5.0

	// Try to use a new cluster for this test, ensure no other task metrics for this cluster
	newClusterName := "ecstest-telemetry-polling-" + uuid.New()
	_, err := ECS.CreateCluster(&ecsapi.CreateClusterInput{
		ClusterName: aws.String(newClusterName),
	})
	require.NoError(t, err, "Failed to create cluster")
	defer DeleteCluster(t, newClusterName)

	// additional config fields to use polling instead of stream
	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_CLUSTER":                       newClusterName,
			"ECS_POLL_METRICS":                  "true",
			"ECS_POLLING_METRICS_WAIT_DURATION": "15s",
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	cwclient := cloudwatch.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	params := &cloudwatch.GetMetricStatisticsInput{
		Namespace: aws.String("AWS/ECS"),
		Period:    aws.Int64(60),
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

	// start telemetry task
	tdOverrides := make(map[string]string)
	tdOverrides["$$$$CPUSHARE$$$$"] = strconv.Itoa(cpuShare)
	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, taskDefinition, tdOverrides)
	require.NoError(t, err, "Failed to start telemetry task")
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error wait telemetry task running")

	// collect and validate busy state metrics
	time.Sleep(waitBusyMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitBusyMetricsInCloudwatchDuration).UTC())

	params.MetricName = aws.String("CPUUtilization")
	metricsAverage, err := VerifyMetrics(cwclient, params, false, 0.0)
	assert.NoError(t, err, "Task is running, verify metrics for CPU utilization failed")
	// Also verify the cpu usage is around expectedCPUPercentage +/- 5%
	assert.InDelta(t, expectedCPUPercentage*100.0, metricsAverage, statsNoiseDelta)

	params.MetricName = aws.String("MemoryUtilization")
	metricsAverage, err = VerifyMetrics(cwclient, params, false, 0.0)
	assert.NoError(t, err, "Task is running, verify metrics for memory utilization failed")
	memInfo, err := system.ReadMemInfo()
	require.NoError(t, err, "Acquiring system info failed")
	totalMemory := memInfo.MemTotal / bytePerMegabyte
	// Verify the memory usage is around 1024/totalMemory +/- 5%
	assert.InDelta(t, float32(1024*100)/float32(totalMemory), metricsAverage, 5)

	err = testTask.Stop()
	require.NoError(t, err, "Failed to stop the telemetry task")

	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Waiting for task stop failed")
}

func telemetryStorageStatsTest(t *testing.T, taskDefinition string) {
	// telemetry task requires 2GB of memory (for either linux or windows); requires a bit more to be stable
	RequireMinimumMemory(t, 2200)

	newClusterName := "ecstest-storagestats-" + uuid.New()
	putAccountInsights := ecsapi.PutAccountSettingInput{
		Name:  aws.String("containerInsights"),
		Value: aws.String("enabled"),
	}
	_, err := ECS.PutAccountSetting(&putAccountInsights)
	require.NoError(t, err, "Failed to update account settings")

	_, err = ECS.CreateCluster(&ecsapi.CreateClusterInput{
		ClusterName: aws.String(newClusterName),
	})
	require.NoError(t, err, "Failed to create cluster")
	defer func() {
		DeleteCluster(t, newClusterName)
		time.Sleep(waitTimeBeforeDeletingCILogGroups)
		cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
		cwlClient.DeleteLogGroup(&cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(fmt.Sprintf("/aws/ecs/containerinsights/%s/performance", newClusterName)),
		})
	}()

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_CLUSTER": newClusterName,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.29.0")

	cwclient := cloudwatch.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	params := &cloudwatch.GetMetricStatisticsInput{
		Namespace: aws.String("ECS/ContainerInsights"),
		Period:    aws.Int64(60),
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

	// start storageStats task
	testTask, err := agent.StartTask(t, taskDefinition)
	require.NoError(t, err, "Failed to start storageStats task")
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error wait storageStats task running")

	// collect and validate minimal state metrics
	time.Sleep(waitMinimalMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitMinimalMetricsInCloudwatchDuration).UTC())

	// verify that metrics are flowing for StorageReadBytes
	params.MetricName = aws.String("StorageReadBytes")
	resp, err := cwclient.GetMetricStatistics(params)
	assert.NotNil(t, resp, "Task is running, no metrics available for StorageReadBytes")
	assert.NotNil(t, resp.Datapoints, "Task is running, nil datapoints returned for StorageReadBytes")
	metricsCount := len(resp.Datapoints)
	assert.NotZero(t, metricsCount, "Task is running, no datapoints returned for StorageReadBytes")

	// verify that metrics are flowing for StorageWriteBytes
	params.MetricName = aws.String("StorageWriteBytes")
	resp, err = cwclient.GetMetricStatistics(params)
	assert.NotNil(t, resp, "Task is running, no metrics available for StorageWriteBytes")
	assert.NotNil(t, resp.Datapoints, "Task is running, nil datapoints returned for StorageWriteBytes")
	metricsCount = len(resp.Datapoints)
	assert.NotZero(t, metricsCount, "Task is running, no datapoints returned for StorageWriteBytes")

	// stop storageStats task
	err = testTask.Stop()
	require.NoError(t, err, "Failed to stop the storageStats task")
	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Waiting for task stop failed")
}

func telemetryNetworkStatsTest(t *testing.T, networkMode string, taskDefinition string) {
	// telemetry task requires 2GB of memory (for either linux or windows); requires a bit more to be stable
	RequireMinimumMemory(t, 2200)

	newClusterName := "ecstest-networkstats-" + uuid.New()
	putAccountInsights := ecsapi.PutAccountSettingInput{
		Name:  aws.String("containerInsights"),
		Value: aws.String("enabled"),
	}
	_, err := ECS.PutAccountSetting(&putAccountInsights)
	require.NoError(t, err, "Failed to update account settings")

	_, err = ECS.CreateCluster(&ecsapi.CreateClusterInput{
		ClusterName: aws.String(newClusterName),
	})
	require.NoError(t, err, "Failed to create cluster")
	defer func() {
		DeleteCluster(t, newClusterName)
		time.Sleep(waitTimeBeforeDeletingCILogGroups)
		cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
		cwlClient.DeleteLogGroup(&cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(fmt.Sprintf("/aws/ecs/containerinsights/%s/performance", newClusterName)),
		})
	}()

	agentOptions := AgentOptions{
		EnableTaskENI: true,
		ExtraEnvironment: map[string]string{
			"ECS_CLUSTER": newClusterName,
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.29.0")

	cwclient := cloudwatch.New(session.New(), aws.NewConfig().WithRegion(*ECS.Config.Region))
	params := &cloudwatch.GetMetricStatisticsInput{
		Namespace: aws.String("ECS/ContainerInsights"),
		Period:    aws.Int64(60),
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

	tdOverrides := make(map[string]string)
	if networkMode != "" {
		tdOverrides["$$$NETWORK_MODE$$$"] = networkMode
	}

	var testTask *TestTask
	// start networkStats task
	if networkMode == "awsvpc" {
		testTask, err = agent.StartAWSVPCTask("network-stats", tdOverrides)
	} else {
		testTask, err = agent.StartTaskWithTaskDefinitionOverrides(t, "network-stats", tdOverrides)
	}
	require.NoError(t, err, "Failed to start networkStats task")
	err = testTask.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error wait networkStats task running")

	// collect and validate minimal state metrics
	time.Sleep(waitMinimalMetricsInCloudwatchDuration)
	params.EndTime = aws.Time(RoundTimeUp(time.Now(), time.Minute).UTC())
	params.StartTime = aws.Time((*params.EndTime).Add(-waitMinimalMetricsInCloudwatchDuration).UTC())

	// verify that metrics are flowing for NetworkRxBytes
	params.MetricName = aws.String("NetworkRxBytes")
	resp, err := cwclient.GetMetricStatistics(params)
	assert.NotNil(t, resp, "Task is running, no metrics available for NetworkRxBytes")
	if networkMode == "bridge" {
		assert.NotNil(t, resp.Datapoints, "Task is running, nil datapoints returned for NetworkRxBytes")
	} else {
		assert.Nil(t, resp.Datapoints, "Task is running, nil datapoints expected for NetworkRxBytes")
	}

	// verify that metrics are flowing for NetworkTxBytes
	params.MetricName = aws.String("NetworkTxBytes")
	resp, err = cwclient.GetMetricStatistics(params)
	assert.NotNil(t, resp, "Task is running, no metrics available for NetworkTxBytes")
	if networkMode == "bridge" {
		assert.NotNil(t, resp.Datapoints, "Task is running, nil datapoints returned for NetworkTxBytes")
	} else {
		assert.Nil(t, resp.Datapoints, "Task is running, nil datapoints expected for NetworkTxBytes")
	}

	// stop networkStats task
	err = testTask.Stop()
	require.NoError(t, err, "Failed to stop the networkStats task")
	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Waiting for task stop failed")
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

func waitCloudwatchLogsWithFilter(client *cloudwatchlogs.CloudWatchLogs, params *cloudwatchlogs.FilterLogEventsInput,
	timeout time.Duration) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	timer := time.NewTimer(timeout)

	waitEventErr := make(chan error, 1)
	cancelled := false

	var output *cloudwatchlogs.FilterLogEventsOutput
	go func() {
		for !cancelled {
			resp, err := client.FilterLogEvents(params)
			if err != nil {
				awsError, ok := err.(awserr.Error)
				if !ok || awsError.Code() != "ResourceNotFoundException" {
					waitEventErr <- err
				}
			} else if len(resp.Events) > 0 {
				output = resp
				waitEventErr <- nil
				break
			}
			time.Sleep(time.Second)
		}
	}()

	select {
	case err := <-waitEventErr:
		return output, err
	case <-timer.C:
		cancelled = true
		return nil, fmt.Errorf("timeout waiting for the logs to be sent to cloudwatch logs")
	}
}

func testV3TaskEndpoint(t *testing.T, taskName, containerName, networkMode, awslogsPrefix string) {
	agentOptions := &AgentOptions{
		EnableTaskENI: true,
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}

	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] = awslogsPrefix
	tdOverrides["$$$CHECK_TAGS$$$"] = "" // Tags are not checked in regular V3TaskEndpoint Test

	if networkMode != "" {
		tdOverrides["$$$NETWORK_MODE$$$"] = networkMode
		tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] = tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] + "-" + networkMode
	}

	var task *TestTask
	var err error

	if networkMode == "awsvpc" {
		task, err = agent.StartAWSVPCTask(taskName, tdOverrides)
	} else {
		task, err = agent.StartTaskWithTaskDefinitionOverrides(t, taskName, tdOverrides)
	}

	require.NoError(t, err, "Error start task")
	err = task.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to run")
	containerId, err := agent.ResolveTaskDockerID(task, containerName)
	require.NoError(t, err, "Error resolving docker id for container in task")

	// Container should have the ExtraEnvironment variable ECS_CONTAINER_METADATA_URI
	ctx := context.TODO()
	containerMetaData, err := agent.DockerClient.ContainerInspect(ctx, containerId)
	require.NoError(t, err, "Could not inspect container for task")
	v3TaskEndpointEnabled := false
	if containerMetaData.Config != nil {
		for _, env := range containerMetaData.Config.Env {
			if strings.HasPrefix(env, "ECS_CONTAINER_METADATA_URI=") {
				v3TaskEndpointEnabled = true
				break
			}
		}
	}
	if !v3TaskEndpointEnabled {
		task.Stop()
		t.Fatal("Could not found ECS_CONTAINER_METADATA_URI in the container environment variable")
	}

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to transition to STOPPED")

	exitCode, _ := task.ContainerExitcode(containerName)
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

// testContainerMetadataFile validates that the metadata file from the
// ECS_CONTAINER_METADATA_FILE environment variable contains all the required
// fields
func testContainerMetadataFile(t *testing.T, taskName, awslogsPrefix string) {
	ctx := context.TODO()
	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_ENABLE_CONTAINER_METADATA": "true",
			"ECS_AVAILABLE_LOGGING_DRIVERS": `["awslogs"]`,
		},
	}

	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()

	agent.RequireVersion(">1.27.0")

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] = awslogsPrefix

	ec2MetadataClient := ec2.NewEC2MetadataClient(nil)
	ip, err := ec2MetadataClient.PublicIPv4Address()
	if err != nil || ip == "" {
		tdOverrides["$$$HAS_PUBLIC_IP$$$"] = "false"
	} else {
		tdOverrides["$$$HAS_PUBLIC_IP$$$"] = "true"
	}

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, taskName, tdOverrides)
	containerName := "container-metadata-file-validator"

	require.NoError(t, err, "Error start task")
	err = task.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to run")
	containerId, err := agent.ResolveTaskDockerID(task, containerName)
	require.NoError(t, err, "Error resolving docker id for container in task")

	// Container should have the ExtraEnvironment variable ECS_CONTAINER_METADATA_URI
	containerMetaData, err := agent.DockerClient.ContainerInspect(ctx, containerId)
	require.NoError(t, err, "Could not inspect container for task")
	containerMetadataFileFound := false
	if containerMetaData.Config != nil {
		for _, env := range containerMetaData.Config.Env {
			if strings.HasPrefix(env, "ECS_CONTAINER_METADATA_FILE=") {
				containerMetadataFileFound = true
				break
			}
		}
	}
	if !containerMetadataFileFound {
		task.Stop()
		t.Fatal("Could not find ECS_CONTAINER_METADATA_FILE in the container environment variable")
	}

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to transition to STOPPED")

	exitCode, _ := task.ContainerExitcode(containerName)
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}

func TestContainerInstanceTags(t *testing.T) {
	// We need long container instance ARN for tagging APIs, PutAccountSettingInput
	// will enable long container instance ARN.
	putAccountSettingInput := ecsapi.PutAccountSettingInput{
		Name:  aws.String("containerInstanceLongArnFormat"),
		Value: aws.String("enabled"),
	}
	_, err := ECS.PutAccountSetting(&putAccountSettingInput)
	assert.NoError(t, err)

	ec2Key := "ec2Key"
	ec2Value := "ec2Value"
	localKey := "localKey"
	localValue := "localValue"
	commonKey := "commonKey"
	commonKeyEC2Value := "commonEC2Value"
	commonKeyLocalValue := "commonKeyLocalValue"

	// Get instance ID.
	ec2MetadataClient := ec2.NewEC2MetadataClient(nil)
	instanceID, err := ec2MetadataClient.InstanceID()
	assert.NoError(t, err)

	// Add tags to the instance.
	ec2Client := ec2.NewClientImpl(*ECS.Config.Region)
	createTagsInput := ec2sdk.CreateTagsInput{
		Resources: []*string{aws.String(instanceID)},
		Tags: []*ec2sdk.Tag{
			{
				Key:   aws.String(ec2Key),
				Value: aws.String(ec2Value),
			},
			{
				Key:   aws.String(commonKey),
				Value: aws.String(commonKeyEC2Value),
			},
		},
	}
	_, err = ec2Client.CreateTags(&createTagsInput)
	assert.NoError(t, err)

	// Set the env var for tags and start Agent.
	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM": "ec2_instance",
			"ECS_CONTAINER_INSTANCE_TAGS": fmt.Sprintf(`{"%s": "%s", "%s": "%s"}`,
				localKey, localValue, commonKey, commonKeyLocalValue),
		},
	}
	agent := RunAgent(t, agentOptions)
	defer agent.Cleanup()
	agent.RequireVersion(">=1.22.0")

	// Verify the tags are registered.
	ListTagsForResourceInput := ecsapi.ListTagsForResourceInput{
		ResourceArn: aws.String(agent.ContainerInstanceArn),
	}
	ListTagsForResourceOutput, err := ECS.ListTagsForResource(&ListTagsForResourceInput)
	assert.NoError(t, err)
	registeredTags := ListTagsForResourceOutput.Tags
	expectedTagsMap := map[string]string{
		ec2Key:    ec2Value,
		localKey:  localValue,
		commonKey: commonKeyLocalValue, // The value of common tag key should be local common tag value
	}

	// Here we only verify that the tags we've defined in the test are registered, because the
	// test instance may have some other tags that are unknown to us.
	for _, registeredTag := range registeredTags {
		registeredTagKey := aws.StringValue(registeredTag.Key)
		if expectedVal, ok := expectedTagsMap[registeredTagKey]; ok {
			assert.Equal(t, expectedVal, aws.StringValue(registeredTag.Value))
			delete(expectedTagsMap, registeredTagKey)
		}
	}
	assert.Zero(t, len(expectedTagsMap))
}

func testV3TaskEndpointTags(t *testing.T, taskName, containerName, networkMode string) {
	ctx := context.TODO()
	// We need long container instance ARN for tagging APIs, PutAccountSettingInput
	// will enable long container instance ARN.
	putAccountSettingInput := ecsapi.PutAccountSettingInput{
		Name:  aws.String("containerInstanceLongArnFormat"),
		Value: aws.String("enabled"),
	}
	_, err := ECS.PutAccountSetting(&putAccountSettingInput)
	assert.NoError(t, err)

	awslogsPrefix := "ecs-functional-tests-v3-task-endpoint-with-tags-validator"
	agentOptions := &AgentOptions{
		ExtraEnvironment: map[string]string{
			"ECS_AVAILABLE_LOGGING_DRIVERS":              `["awslogs"]`,
			"ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM": "ec2_instance",
			"ECS_CONTAINER_INSTANCE_TAGS": fmt.Sprintf(`{"%s": "%s"}`,
				"localKey", "localValue"),
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

	tdOverrides := make(map[string]string)
	tdOverrides["$$$CHECK_TAGS$$$"] = "CheckTags" // To enable Tag check in v3-task-endpoint-validator image

	tdOverrides["$$$TEST_REGION$$$"] = *ECS.Config.Region
	tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] = awslogsPrefix
	tdOverrides["$$$NETWORK_MODE$$$"] = networkMode
	tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] = tdOverrides["$$$TEST_AWSLOGS_STREAM_PREFIX$$$"] + "-" + networkMode

	task, err := agent.StartTaskWithTaskDefinitionOverrides(t, taskName, tdOverrides)

	require.NoError(t, err, "Error start task")
	err = task.WaitRunning(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to run")
	containerId, err := agent.ResolveTaskDockerID(task, containerName)
	require.NoError(t, err, "Error resolving docker id for container in task")

	// Container should have the ExtraEnvironment variable ECS_CONTAINER_METADATA_URI
	containerMetaData, err := agent.DockerClient.ContainerInspect(ctx, containerId)
	require.NoError(t, err, "Could not inspect container for task")
	v3TaskEndpointEnabled := false
	if containerMetaData.Config != nil {
		for _, env := range containerMetaData.Config.Env {
			if strings.HasPrefix(env, "ECS_CONTAINER_METADATA_URI=") {
				v3TaskEndpointEnabled = true
				break
			}
		}
	}
	if !v3TaskEndpointEnabled {
		task.Stop()
		t.Fatal("Could not found ECS_CONTAINER_METADATA_URI in the container environment variable")
	}

	err = task.WaitStopped(waitTaskStateChangeDuration)
	require.NoError(t, err, "Error waiting for task to transition to STOPPED")

	exitCode, _ := task.ContainerExitcode(containerName)
	assert.Equal(t, 42, exitCode, fmt.Sprintf("Expected exit code of 42; got %d", exitCode))
}
