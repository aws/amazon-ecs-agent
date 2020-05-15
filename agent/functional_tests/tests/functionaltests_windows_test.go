// +build windows,functional

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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	. "github.com/aws/amazon-ecs-agent/agent/functional_tests/util"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/ssm"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	savedStateTaskDefinition        = "savedstate-windows"
	portResContentionTaskDefinition = "port-80-windows"
	labelsTaskDefinition            = "labels-windows"
	dockerEndpoint                  = "npipe:////./pipe/docker_engine"
	dockerEndpointEnvVariable       = "DOCKER_HOST"
	errCodeAccessDenied             = "AccessDenied"
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
	ctx := context.TODO()
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
	containerMetaData, err := agent.DockerClient.ContainerInspect(ctx, containerId)
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
		t.Fatal("Could not found AWS_CONTAINER_CREDENTIALS_RELATIVE_URI in the container environment variable")
	}

	// Task will only run one command "aws ec2 describe-regions"
	err = task.WaitStopped(waitTaskStateChangeDuration)
	if err != nil {
		t.Fatalf("Waiting task to stop error : %v", err)
	}

	containerMetaData, err = agent.DockerClient.ContainerInspect(ctx, containerId)
	if err != nil {
		t.Fatalf("Could not inspect container for task: %v", err)
	}

	if containerMetaData.State.ExitCode != 42 {
		t.Fatalf("Container exit code non-zero: %v", containerMetaData.State.ExitCode)
	}
}

func TestV3TaskEndpointDefaultNetworkMode(t *testing.T) {
	testV3TaskEndpoint(t, "v3-task-endpoint-validator-windows", "v3-task-endpoint-validator-windows", "", "ecs-functional-tests-v3-task-endpoint-validator-windows")
}

func TestV3TaskEndpointTags(t *testing.T) {
	testV3TaskEndpointTags(t, "v3-task-endpoint-validator-windows", "v3-task-endpoint-validator-windows", "")
}
func TestContainerMetadataFile(t *testing.T) {
	testContainerMetadataFile(t, "container-metadata-file-validator-windows", "ecs-functional-tests-container-metadata-file-validator-windows")
}

// TestTelemetry tests whether agent can send metrics to TACS, through streaming docker stats
func TestTelemetry(t *testing.T) {
	telemetryTest(t, "telemetry-windows")
}

// TestTelemetry tests whether agent can send metrics to TACS, through polling docker stats
func TestTelemetryWithStatsPolling(t *testing.T) {
	telemetryTestWithStatsPolling(t, "telemetry-windows")
}

// TestOOMContainer verifies that an OOM container returns an error
func TestOOMContainer(t *testing.T) {
	agent := RunAgent(t, nil)
	defer agent.Cleanup()

	testTask, err := agent.StartTask(t, "oom-windows")
	require.NoError(t, err, "Expected to start invalid-image task")
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

	agent.RequireVersion(">=1.17.0") //Required for awslogs driver multiline pattern option

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

	agent.RequireVersion(">=1.17.0") //Required for awslogs driver datetime format option

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

	wErr := wTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, wErr, "Expect task to be stopped")
	wExitCode := wTask.Containers[0].ExitCode
	assert.NotEqual(t, 42, wExitCode, fmt.Sprintf("Expected exit code of 42; got %d", wExitCode))

	rTask, rTaskErr := agent.StartTask(t, "task-shared-vol-read-windows")
	require.NoError(t, rTaskErr, "Register task definition failed")

	rErr := rTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, rErr, "Expect task to be stopped")
	rExitCode := rTask.Containers[0].ExitCode
	assert.NotEqual(t, 42, rExitCode, fmt.Sprintf("Expected exit code of 42; got %d", rExitCode))
}

func TestGMSAFile(t *testing.T) {
	RequireDockerAPIVersion(t, ">=1.24")

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION": "true",
		},
	}
	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.32.1")

	// Setup test gmsa file
	envProgramData := "ProgramData"
	dockerCredentialSpecDataDir := "docker/credentialspecs"
	programDataDir := os.Getenv(envProgramData)
	if programDataDir == "" {
		programDataDir = "C:/ProgramData"
	}
	testFileName := "test-gmsa.json"
	testCredSpecFilePath := filepath.Join(programDataDir, dockerCredentialSpecDataDir, testFileName)
	testCredSpecData := []byte(`{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "gmsa-acct-test",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "gmsa.test.com",
                             "DnsName":  "gmsa.test.com",
                             "NetBiosName":  "gmsa"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "gmsa-acct-test",
                                                                          "Scope":  "gmsa.test.com"
                                                                      }
                                                                  ]
                              }
}`)

	err := ioutil.WriteFile(testCredSpecFilePath, testCredSpecData, 0755)
	require.NoError(t, err)

	tdOverrides := make(map[string]string)
	tdOverrides["$$$TEST_REGION$$$"] = aws.StringValue(ECS.Config.Region)

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "gmsa-localfile", tdOverrides)
	require.NoError(t, err)

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// Verify container metadata
	cid, err := agent.ResolveTaskDockerID(testTask, "windows_sample_app")
	require.NoError(t, err, "Error resolving docker id for container in task")
	assert.NotEmpty(t, cid)

	// Setup docker client to validate credentialspec
	endpoint := utils.DefaultIfBlank(os.Getenv(dockerEndpointEnvVariable), dockerEndpoint)
	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))

	testCredentialSpecOption := "credentialspec=file://test-gmsa.json"
	err = verifyContainerCredentialSpec(client, cid, testCredentialSpecOption)
	assert.NoError(t, err)

	// Stop test task
	testTask.Stop()

	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, err)

	// Cleanup the test file
	err = os.Remove(testCredSpecFilePath)
	assert.NoError(t, err)
}

func TestGMSASSMFile(t *testing.T) {
	// Setup required role
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	// execution role arn is following the pattern arn:aws:iam::accountId:role/***
	executionRole := os.Getenv("ECS_FTS_EXECUTION_ROLE")
	if executionRole == "" {
		t.Skip("ECS_FTS_EXECUTION_ROLE was not set to run this test")
	}

	// Setup SSM parameter for gMSA test
	parameterName := "FunctionalTest-gMSA-SSM"
	testCredSpecData := `{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "gmsa-acct-test",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "gmsa.test.com",
                             "DnsName":  "gmsa.test.com",
                             "NetBiosName":  "gmsa"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "gmsa-acct-test",
                                                                          "Scope":  "gmsa.test.com"
                                                                      }
                                                                  ]
                              }
}`
	region := *ECS.Config.Region
	ssmClient := ssm.New(session.New(), aws.NewConfig().WithRegion(region))
	input := &ssm.PutParameterInput{
		Description: aws.String("Resource created for the ECS Agent Functional Test: TestGMSASSMFile"),
		Name:        aws.String(parameterName),
		Value:       aws.String(testCredSpecData),
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

	// read parameter back to obtain ARN
	paramInput := &ssm.GetParameterInput{
		Name: aws.String(parameterName),
	}
	data, err := ssmClient.GetParameter(paramInput)
	require.NoError(t, err)

	// Obtain ARN of SSM param
	gMSASSMParameterARN := *data.Parameter.ARN

	// Setup agent
	RequireDockerAPIVersion(t, ">=1.24")

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION": "true",
			"ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION":                  "1m",
		},
	}

	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.32.1")

	// Setup taskdef
	tdOverrides := make(map[string]string)
	tdOverrides["$$$EXECUTION_ROLE$$$"] = executionRole
	tdOverrides["$$$TEST_SSM_ARN$$$"] = gMSASSMParameterARN

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "gmsa-ssm", tdOverrides)
	require.NoError(t, err)

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// Verify container metadata
	cid, err := agent.ResolveTaskDockerID(testTask, "windows_sample_app")
	require.NoError(t, err, "Error resolving docker id for container in task")
	assert.NotEmpty(t, cid)

	// Setup docker client to validate credentialspec
	endpoint := utils.DefaultIfBlank(os.Getenv(dockerEndpointEnvVariable), dockerEndpoint)
	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))

	// credentialspec=file://ssm_c3ce6880-f815-4044-af03-29f934836293_FunctionalTest-gMSA-SSM
	testCredentialSpecOption := fmt.Sprintf("credentialspec=file://ssm_%s_%s", taskID, parameterName)

	err = verifyContainerCredentialSpec(client, cid, testCredentialSpecOption)
	assert.NoError(t, err)

	// Stop test task
	testTask.Stop()

	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, err)

	// Cleanup the test file
	testCredSpecFilePath := fmt.Sprintf("C:/ProgramData/docker/credentialspecs/ssm_%s_%s", taskID, parameterName)
	err = os.Remove(testCredSpecFilePath)
	assert.NoError(t, err)

	// Cleanup the ssm parameter
	_, err = ssmClient.DeleteParameter(&ssm.DeleteParameterInput{Name: aws.String(parameterName)})
	assert.NoError(t, err)
}

func TestGMSAS3File(t *testing.T) {
	// Setup required role
	if os.Getenv("TEST_DISABLE_EXECUTION_ROLE") == "true" {
		t.Skip("TEST_DISABLE_EXECUTION_ROLE was set to true")
	}

	// execution role arn is following the pattern arn:aws:iam::accountId:role/***
	executionRole := os.Getenv("ECS_FTS_EXECUTION_ROLE")
	if executionRole == "" {
		t.Skip("ECS_FTS_EXECUTION_ROLE was not set to run this test")
	}

	// Setup s3 artifact
	s3Bucket := os.Getenv("ECS_FTEST_S3_BUCKET")
	s3BucketRegion := os.Getenv("ECS_FTEST_S3_BUCKET_REGION")
	if s3Bucket == "" || s3BucketRegion == "" {
		t.Skip("Skipping test as it requires both ECS_FTEST_S3_BUCKET and ECS_FTEST_S3_BUCKET_REGION to be set.")
	}
	t.Logf("Using s3 bucket %s in region %s", s3Bucket, s3BucketRegion)

	sessWithRegion := session.Must(session.NewSession(aws.NewConfig().WithRegion(s3BucketRegion)))
	svc := s3manager.NewUploaderWithClient(s3.New(sessWithRegion))

	testCredSpecData := `{
    "CmsPlugins":  [
                       "ActiveDirectory"
                   ],
    "DomainJoinConfig":  {
                             "Sid":  "S-1-5-21-975084816-3050680612-2826754290",
                             "MachineAccountName":  "gmsa-acct-test",
                             "Guid":  "92a07e28-bd9f-4bf3-b1f7-0894815a5257",
                             "DnsTreeName":  "gmsa.test.com",
                             "DnsName":  "gmsa.test.com",
                             "NetBiosName":  "gmsa"
                         },
    "ActiveDirectoryConfig":  {
                                  "GroupManagedServiceAccounts":  [
                                                                      {
                                                                          "Name":  "gmsa-acct-test",
                                                                          "Scope":  "gmsa.test.com"
                                                                      }
                                                                  ]
                              }
}`

	_, err := svc.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String("gmsa-test-file.json"),
		Body:   strings.NewReader(testCredSpecData),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == errCodeAccessDenied {
			t.Skip("Skipping the test since lacking necessary s3 access permission")
		} else {
			t.Fatalf("Unable to upload config file to s3 which is needed for the test: %v", err)
		}
	}

	// Setup agent
	RequireDockerAPIVersion(t, ">=1.24")

	agentOptions := AgentOptions{
		ExtraEnvironment: map[string]string{
			"ZZZ_SKIP_DOMAIN_JOIN_CHECK_NOT_SUPPORTED_IN_PRODUCTION": "true",
			"ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION":                  "1m",
		},
	}

	agent := RunAgent(t, &agentOptions)
	defer agent.Cleanup()

	agent.RequireVersion(">=1.32.1")

	// Setup taskdef overrides
	tdOverrides := make(map[string]string)
	tdOverrides["$$$EXECUTION_ROLE$$$"] = executionRole
	tdOverrides["$$$TEST_S3_BUCKET$$$"] = s3Bucket

	testTask, err := agent.StartTaskWithTaskDefinitionOverrides(t, "gmsa-s3", tdOverrides)
	require.NoError(t, err)

	// Wait for the container to start
	testTask.WaitRunning(waitTaskStateChangeDuration)
	taskID, err := GetTaskID(aws.StringValue(testTask.TaskArn))
	require.NoError(t, err)
	assert.NotEmpty(t, taskID)

	// Verify container metadata
	cid, err := agent.ResolveTaskDockerID(testTask, "windows_sample_app")
	require.NoError(t, err, "Error resolving docker id for container in task")
	assert.NotEmpty(t, cid)

	// Setup docker client to validate credentialspec
	endpoint := utils.DefaultIfBlank(os.Getenv(dockerEndpointEnvVariable), dockerEndpoint)
	client, _ := sdkClient.NewClientWithOpts(sdkClient.WithHost(endpoint), sdkClient.WithVersion(sdkclientfactory.GetDefaultVersion().String()))

	// credentialspec=file://s3_c3ce6880-f815-4044-af03-29f934836293_gmsa-test-file.json
	testCredentialSpecOption := fmt.Sprintf("credentialspec=file://s3_%s_gmsa-test-file.json", taskID)

	err = verifyContainerCredentialSpec(client, cid, testCredentialSpecOption)
	assert.NoError(t, err)

	// Stop test task
	testTask.Stop()

	err = testTask.WaitStopped(waitTaskStateChangeDuration)
	assert.NoError(t, err)

	// Cleanup the test file
	testCredSpecFilePath := fmt.Sprintf("C:/ProgramData/docker/credentialspecs/s3_%s_gmsa-test-file.json", taskID)
	err = os.Remove(testCredSpecFilePath)
	assert.NoError(t, err)
}

func verifyContainerCredentialSpec(client *sdkClient.Client, id, credentialspecOpt string) error {
	dockerContainer, err := client.ContainerInspect(context.TODO(), id)
	if err != nil {
		return err
	}

	for _, opt := range dockerContainer.HostConfig.SecurityOpt {
		if strings.HasPrefix(opt, "credentialspec=") && opt == credentialspecOpt {
			return nil
		}
	}

	return errors.New("unable to obtain credentialspec")
}
