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

package util

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	v1 "github.com/aws/amazon-ecs-agent/agent/handlers/v1"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	arnResourceSections                 = 2
	arnResourceDelimiter                = "/"
	bytePerMegabyte                     = 1024 * 1024
	waitContainerInstanceStatusDuration = time.Minute
)

// GetTaskDefinition is a helper that provies the family:revision for the named
// task definition where the name matches the folder in which the task
// definition is present. In order to avoid re-registering a task definition
// when it has already been regestered in the past, this registers a task
// definition of the pattern 'family-md5sum' with md5sum being the input task
// definition json's md5. This special family name is checked for existence
// before a new one is registered and it is assumed that if it exists, the task
// definition currently represented by the file was registered as such already.
func GetTaskDefinition(name string) (string, error) {
	return GetTaskDefinitionWithOverrides(name, make(map[string]string))
}

func GetTaskDefinitionWithOverrides(name string, overrides map[string]string) (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	tdDataFromFile, err := ioutil.ReadFile(filepath.Join(path.Dir(filename), "..", "testdata", "taskdefinitions", name, "task-definition.json"))
	if err != nil {
		return "", err
	}

	tdStr := string(tdDataFromFile)
	for key, value := range overrides {
		tdStr = strings.Replace(tdStr, key, value, -1)
	}
	tdData := []byte(tdStr)

	registerRequest := &ecs.RegisterTaskDefinitionInput{}
	err = json.Unmarshal(tdData, registerRequest)
	if err != nil {
		return "", err
	}

	tdHash := fmt.Sprintf("%x", md5.Sum(tdData))
	idempotentFamily := *registerRequest.Family + "-" + tdHash

	existing, err := ECS.DescribeTaskDefinition(&ecs.DescribeTaskDefinitionInput{
		TaskDefinition: &idempotentFamily,
	})
	if err == nil {
		return fmt.Sprintf("%s:%d", *existing.TaskDefinition.Family, *existing.TaskDefinition.Revision), nil
	}

	registerRequest.Family = &idempotentFamily

	registered, err := ECS.RegisterTaskDefinition(registerRequest)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", *registered.TaskDefinition.Family, *registered.TaskDefinition.Revision), nil
}

func IsCNPartition() bool {
	partitions := endpoints.DefaultPartitions()
	p, _ := endpoints.PartitionForRegion(partitions, *ECS.Config.Region)

	if p.ID() == endpoints.AwsCnPartition().ID() {
		return true
	}
	return false
}

type TestAgent struct {
	Image                string
	DockerID             string
	IntrospectionURL     string
	Version              string
	ContainerInstanceArn string
	Cluster              string
	TestDir              string
	Logdir               string
	Options              *AgentOptions
	Process              *os.Process

	DockerClient *docker.Client
	t            *testing.T
}

type AgentOptions struct {
	ExtraEnvironment map[string]string
	PortBindings     map[nat.Port]map[string]string
	EnableTaskENI    bool
	GPUEnabled       bool
	TempDirOverride  string
}

// verifyIntrospectionAPI verifies that we can talk to the agent's introspection http endpoint.
// This is a platform-independent piece of Agent Startup.
func (agent *TestAgent) verifyIntrospectionAPI() error {
	// Wait up to 10s for it to register
	var localMetadata v1.MetadataResponse
	for i := 0; i < 10; i++ {
		func() {
			agentMetadataResp, err := http.Get(agent.IntrospectionURL + "/v1/metadata")
			if err != nil {
				return
			}
			metadata, err := ioutil.ReadAll(agentMetadataResp.Body)
			if err != nil {
				return
			}

			json.Unmarshal(metadata, &localMetadata)
		}()
		if localMetadata.ContainerInstanceArn != nil && *localMetadata.ContainerInstanceArn != "" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	ctx := context.TODO()
	if localMetadata.ContainerInstanceArn == nil {
		stopTimeout := 1 * time.Second
		agent.DockerClient.ContainerStop(ctx, agent.DockerID, &stopTimeout)
		return errors.New("Could not get agent metadata after launching it")
	}

	agent.ContainerInstanceArn = *localMetadata.ContainerInstanceArn
	agent.Cluster = localMetadata.Cluster
	agent.Version = utils.ExtractVersion(localMetadata.Version)
	agent.t.Logf("Found agent metadata: ContainerInstanceArn: %s, Cluster: %s, Version: %s",
		agent.ContainerInstanceArn, agent.Cluster, agent.Version)
	return nil
}

// Platform Independent piece of Agent Cleanup. Gets executed on both linux and Windows.
func (agent *TestAgent) platformIndependentCleanup() {
	agent.StopAgent()
	if agent.t.Failed() {
		agent.t.Logf("Preserving test dir for failed test %s", agent.TestDir)
	} else {
		agent.t.Logf("Removing test dir for passed test %s", agent.TestDir)
		os.RemoveAll(agent.TestDir)
	}

	ECS.DeregisterContainerInstance(&ecs.DeregisterContainerInstanceInput{
		Cluster:           &agent.Cluster,
		ContainerInstance: &agent.ContainerInstanceArn,
		Force:             aws.Bool(true),
	})
}

// Cleanup without stopping the Agent and deregistering.
func (agent *TestAgent) TestCleanup() {
	if agent.t.Failed() {
		agent.t.Logf("Preserving test dir for failed test %s", agent.TestDir)
	} else {
		agent.t.Logf("Removing test dir for passed test %s", agent.TestDir)
		os.RemoveAll(agent.TestDir)
	}
}

func (agent *TestAgent) StartMultipleTasks(t *testing.T, taskDefinition string, num int) ([]*TestTask, error) {
	t.Logf("Task definition: %s", taskDefinition)
	cis := make([]*string, num)
	for i := 0; i < num; i++ {
		cis[i] = &agent.ContainerInstanceArn
	}

	resp, err := ECS.StartTask(&ecs.StartTaskInput{
		Cluster:            &agent.Cluster,
		ContainerInstances: cis,
		TaskDefinition:     &taskDefinition,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Failures) != 0 || len(resp.Tasks) == 0 {
		return nil, errors.New("Failure starting task: " + *resp.Failures[0].Reason)
	}

	testTasks := make([]*TestTask, num)
	for i, task := range resp.Tasks {
		agent.t.Logf("Started task: %s\n", *task.TaskArn)
		testTasks[i] = &TestTask{task}
	}
	return testTasks, nil
}

func (agent *TestAgent) StartTask(t *testing.T, task string) (*TestTask, error) {
	td, err := GetTaskDefinition(task)
	if err != nil {
		return nil, err
	}

	tasks, err := agent.StartMultipleTasks(t, td, 1)
	if err != nil {
		return nil, err
	}
	return tasks[0], nil
}

func (agent *TestAgent) StartTaskWithTaskDefinitionOverrides(t *testing.T, task string, overrides map[string]string) (*TestTask, error) {
	td, err := GetTaskDefinitionWithOverrides(task, overrides)
	if err != nil {
		return nil, err
	}

	tasks, err := agent.StartMultipleTasks(t, td, 1)
	if err != nil {
		return nil, err
	}

	return tasks[0], nil
}

// StartAWSVPCTask starts a task with "awsvpc" networking mode
func (agent *TestAgent) StartAWSVPCTask(task string, overrides map[string]string) (*TestTask, error) {
	td, err := GetTaskDefinitionWithOverrides(task, overrides)
	if err != nil {
		return nil, err
	}

	return agent.startAWSVPCTask(td)
}

func (agent *TestAgent) startAWSVPCTask(taskDefinition string) (*TestTask, error) {
	agent.t.Logf("Task definition: %s", taskDefinition)
	// Get the subnet ID, which is a required parameter for starting
	// tasks in 'awsvpc' network mode
	subnet, err := GetSubnetID()
	if err != nil {
		return nil, err
	}

	agent.t.Logf("Starting 'awsvpc' task in subnet: %s", subnet)
	resp, err := ECS.StartTask(&ecs.StartTaskInput{
		Cluster:            &agent.Cluster,
		ContainerInstances: []*string{&agent.ContainerInstanceArn},
		TaskDefinition:     &taskDefinition,
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				Subnets: []*string{&subnet},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Failures) != 0 || len(resp.Tasks) == 0 {
		return nil, errors.New("Failure starting task: " + *resp.Failures[0].Reason)
	}

	task := resp.Tasks[0]
	agent.t.Logf("Started task: %s\n", *task.TaskArn)
	return &TestTask{task}, nil
}

func (agent *TestAgent) StartTaskWithOverrides(t *testing.T, task string, overrides []*ecs.ContainerOverride) (*TestTask, error) {
	td, err := GetTaskDefinition(task)
	if err != nil {
		return nil, err
	}
	t.Logf("Task definition: %s", td)

	resp, err := ECS.StartTask(&ecs.StartTaskInput{
		Cluster:            &agent.Cluster,
		ContainerInstances: []*string{&agent.ContainerInstanceArn},
		TaskDefinition:     &td,
		Overrides: &ecs.TaskOverride{
			ContainerOverrides: overrides,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Failures) != 0 || len(resp.Tasks) == 0 {
		return nil, errors.New("Failure starting task: " + *resp.Failures[0].Reason)
	}

	agent.t.Logf("Started task: %s\n", *resp.Tasks[0].TaskArn)
	return &TestTask{resp.Tasks[0]}, nil
}

// RoundTimeUp rounds the time to the next second/minute/hours depending on the duration
func RoundTimeUp(realTime time.Time, duration time.Duration) time.Time {
	tmpTime := realTime.Round(duration)
	if tmpTime.Before(realTime) {
		return tmpTime.Add(duration)
	}
	return tmpTime
}

func DeleteCluster(t *testing.T, clusterName string) {
	_, err := ECS.DeleteCluster(&ecs.DeleteClusterInput{
		Cluster: aws.String(clusterName),
	})
	if err != nil {
		t.Fatalf("Failed to delete the cluster: %s: %v", clusterName, err)
	}
}

// gets metrics for given time interval and metricName
// validates metrics for given conditions
// returns an average over all (trimmed) metric datapoints as float64
func VerifyMetrics(cwclient *cloudwatch.CloudWatch, params *cloudwatch.GetMetricStatisticsInput, idleCluster bool, noiseDelta float64) (float64, error) {
	resp, err := cwclient.GetMetricStatistics(params)
	if err != nil {
		return float64(0.0), fmt.Errorf("Error getting metrics of cluster: %v", err)
	}

	if resp == nil || resp.Datapoints == nil {
		return float64(0.0), fmt.Errorf("Cloudwatch get metrics failed, returned null")
	}
	metricsCount := len(resp.Datapoints)
	if metricsCount == 0 {
		return float64(0.0), fmt.Errorf("No datapoints returned")
	}

	// Samplecount is always expected to be "1" for cluster metrics
	datapoint := resp.Datapoints[metricsCount-1]
	if *datapoint.SampleCount != 1.0 {
		return float64(0.0), fmt.Errorf("Incorrect SampleCount %f, expected 1", *datapoint.SampleCount)
	}

	trimmedResponseDatapoints := trimOutliers(resp.Datapoints)
	responseAverage := getAverage(trimmedResponseDatapoints)

	if idleCluster {
		if responseAverage >= noiseDelta {
			return float64(0.0), fmt.Errorf("utilization is >= expected noise delta for idle cluster")
		}
	} else {
		if responseAverage < noiseDelta {
			return float64(0.0), fmt.Errorf("utilization is < expected noise delta for non-idle cluster")
		}
	}
	return responseAverage, nil
}

// trimOutliers smooths out an array of CloudWatch Datapoints.
// This is meant to clear outliers we encounter in the stats.
func trimOutliers(datapoints []*cloudwatch.Datapoint) []*cloudwatch.Datapoint {
	if len(datapoints) < 3 {
		// we need at least 3 datapoints to remove min/max and still
		// have something left over
		return datapoints
	}
	// find min/max indexes and slice out of datapoints array
	maxIndex := -1
	maxValue := float64(0.0) // initialized to min float
	for index, datapoint := range datapoints {
		if *datapoint.Average >= maxValue {
			maxValue = *datapoint.Average
			maxIndex = index
		}
	}
	datapoints = append(datapoints[:maxIndex], datapoints[maxIndex+1:]...)
	minIndex := -1
	minValue := math.MaxFloat64
	for index, datapoint := range datapoints {
		if *datapoint.Average <= minValue {
			minValue = *datapoint.Average
			minIndex = index
		}
	}
	datapoints = append(datapoints[:minIndex], datapoints[minIndex+1:]...)
	return datapoints
}

// finds average for all datapoints in an array of cloudwatch.Datapoint
func getAverage(datapoints []*cloudwatch.Datapoint) float64 {
	total := float64(0.0)
	for _, val := range datapoints {
		total += *val.Average
	}
	return total / float64(len(datapoints))
}

// ResolveTaskDockerID determines the Docker ID for a container within a given
// task that has been run by the Agent.
func (agent *TestAgent) ResolveTaskDockerID(task *TestTask, containerName string) (string, error) {
	var err error
	var dockerId string
	for i := 0; i < 5; i++ {
		dockerId, err = agent.resolveTaskDockerID(task, containerName)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return dockerId, err
}

func (agent *TestAgent) resolveTaskDockerID(task *TestTask, containerName string) (string, error) {
	bodyData, err := agent.callTaskIntrospectionApi(*task.TaskArn)
	if err != nil {
		return "", err
	}
	var taskResp v1.TaskResponse
	err = json.Unmarshal(*bodyData, &taskResp)
	if err != nil {
		return "", err
	}
	if len(taskResp.Containers) == 0 {
		return "", errors.New("No containers in task response")
	}
	for _, container := range taskResp.Containers {
		if container.Name == containerName {
			return container.DockerID, nil
		}
	}
	return "", errors.New("No containers matched given name")
}

func (agent *TestAgent) WaitStoppedViaIntrospection(task *TestTask) (bool, error) {
	var err error
	var isStopped bool

	for i := 0; i < 5; i++ {
		isStopped, err = agent.waitStoppedViaIntrospection(task)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return isStopped, err
}

func (agent *TestAgent) waitStoppedViaIntrospection(task *TestTask) (bool, error) {
	rawResponse, err := agent.callTaskIntrospectionApi(*task.TaskArn)
	if err != nil {
		return false, err
	}

	var taskResp v1.TaskResponse
	err = json.Unmarshal(*rawResponse, &taskResp)

	if taskResp.KnownStatus == "STOPPED" {
		return true, nil
	} else {
		return false, errors.New("Task should be STOPPED but is " + taskResp.KnownStatus)
	}
}

func (agent *TestAgent) WaitRunningViaIntrospection(task *TestTask) (bool, error) {
	var err error
	var isRunning bool

	for i := 0; i < 5; i++ {
		isRunning, err = agent.waitRunningViaIntrospection(task)
		if err == nil && isRunning {
			break
		}
		time.Sleep(10000 * time.Millisecond)
	}
	return isRunning, err
}

func (agent *TestAgent) waitRunningViaIntrospection(task *TestTask) (bool, error) {
	rawResponse, err := agent.callTaskIntrospectionApi(*task.TaskArn)
	if err != nil {
		return false, err
	}

	var taskResp v1.TaskResponse
	err = json.Unmarshal(*rawResponse, &taskResp)

	if taskResp.KnownStatus == "RUNNING" {
		return true, nil
	} else {
		return false, errors.New("Task should be RUNNING but is " + taskResp.KnownStatus)
	}
}

func (agent *TestAgent) CallTaskIntrospectionAPI(task *TestTask) (*v1.TaskResponse, error) {
	rawResponse, err := agent.callTaskIntrospectionApi(*task.TaskArn)
	if err != nil {
		return nil, err
	}

	var taskResp v1.TaskResponse
	err = json.Unmarshal(*rawResponse, &taskResp)
	return &taskResp, err
}

func (agent *TestAgent) callTaskIntrospectionApi(taskArn string) (*[]byte, error) {
	fullIntrospectionApiURL := agent.IntrospectionURL + "/v1/tasks"
	if taskArn != "" {
		fullIntrospectionApiURL += "?taskarn=" + taskArn
	}

	agentTasksResp, err := http.Get(fullIntrospectionApiURL)
	if err != nil {
		return nil, err
	}

	bodyData, err := ioutil.ReadAll(agentTasksResp.Body)
	if err != nil {
		return nil, err
	}
	return &bodyData, nil
}

func (agent *TestAgent) RequireVersion(version string) {
	if agent.Version == "UNKNOWN" {
		agent.t.Skipf("Skipping test requiring version %v; agent version unknown", version)
	}

	matches, err := utils.Version(agent.Version).Matches(version)
	if err != nil {
		agent.t.Skipf("Skipping test requiring version %v; could not compare because of error: %v", version, err)
	}
	if !matches {
		agent.t.Skipf("Skipping test requiring version %v; agent version %v", version, agent.Version)
	}
}

type TestTask struct {
	*ecs.Task
}

func (task *TestTask) Redescribe() {
	res, err := ECS.DescribeTasks(&ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []*string{task.TaskArn},
	})
	if err == nil && len(res.Failures) == 0 {
		task.Task = res.Tasks[0]
	}
}

func (task *TestTask) waitStatus(timeout time.Duration, status string) error {
	timer := time.NewTimer(timeout)
	atStatus := make(chan error, 1)

	cancelled := false
	go func() {
		if *task.LastStatus == "STOPPED" && status != "STOPPED" {
			atStatus <- errors.New("Task terminal; will never reach " + status)
			return
		}
		for *task.LastStatus != status && !cancelled {
			task.Redescribe()
			if *task.LastStatus == status {
				break
			}
			if *task.LastStatus == "STOPPED" && status != "STOPPED" {
				atStatus <- errors.New("Task terminal; will never reach " + status)
				return
			}
			time.Sleep(5 * time.Second)
		}
		atStatus <- nil
	}()

	select {
	case err := <-atStatus:
		return err
	case <-timer.C:
		cancelled = true
		return errors.Errorf("Timed out waiting for task '%s' to reach '%s': '%s' ",
			status, *task.TaskArn, task.GoString())
	}
}

func (task *TestTask) ContainerExitcode(name string) (int, bool) {
	for _, cont := range task.Containers {
		if cont != nil && cont.Name != nil && cont.ExitCode != nil {
			if *cont.Name == name {
				return int(*cont.ExitCode), true
			}
		}
	}
	return 0, false
}

func (task *TestTask) WaitRunning(timeout time.Duration) error {
	return task.waitStatus(timeout, "RUNNING")
}

func (task *TestTask) WaitStopped(timeout time.Duration) error {
	return task.waitStatus(timeout, "STOPPED")
}

func (task *TestTask) ExpectErrorType(containerName, errType string, timeout time.Duration) error {
	task.WaitStopped(timeout)

	for _, container := range task.Containers {
		if *container.Name != containerName {
			continue
		}
		if container.Reason == nil {
			return errors.New("Expected error reason")
		}
		errParts := strings.SplitN(*container.Reason, ":", 2)
		if len(errParts) != 2 {
			return errors.New("Error did not have a type: " + *container.Reason)
		}
		if errParts[0] != errType {
			return errors.New("Type did not match: " + *container.Reason)
		}
		return nil
	}
	return errors.New("Could not find container " + containerName + " in task " + *task.TaskArn)
}

func (task *TestTask) Stop() error {
	_, err := ECS.StopTask(&ecs.StopTaskInput{
		Cluster: task.ClusterArn,
		Task:    task.TaskArn,
	})
	return err
}

// GetAttachmentInfo returns the task's attachment properties, as a list of key value pairs
func (task *TestTask) GetAttachmentInfo() ([]*ecs.KeyValuePair, error) {
	if len(task.Attachments) == 0 {
		return nil, errors.New("attachments empty for task")
	}

	return task.Attachments[0].Details, nil
}

func RequireDockerVersion(t *testing.T, selector string) {
	ctx := context.TODO()
	dockerClient, err := docker.NewClientWithOpts(docker.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Could not get docker client to check version")

	version, err := dockerClient.ServerVersion(ctx)
	require.NoError(t, err, "Could not get docker version")

	dockerVersion := version.Version
	match, err := utils.Version(dockerVersion).Matches(selector)
	require.NoError(t, err, "Could not check docker version to match required")

	if !match {
		t.Skipf("Skipping test; requires %v, but version is %v", selector, dockerVersion)
	}
}

func RequireMinimumMemory(t *testing.T, minimumMemoryInMegaBytes int) {
	memInfo, err := system.ReadMemInfo()
	require.NoError(t, err, "Could not check system memory info before checking minimum memory requirement")

	totalMemory := int(memInfo.MemTotal / bytePerMegabyte)
	if totalMemory < minimumMemoryInMegaBytes {
		t.Skipf("Skipping the test since it requires %d MB of memory. Total memory on the instance: %d MB", minimumMemoryInMegaBytes, totalMemory)
	}
}

func RequireDockerAPIVersion(t *testing.T, selector string) {
	ctx := context.TODO()
	dockerClient, err := docker.NewClientWithOpts(docker.WithVersion(sdkclientfactory.GetDefaultVersion().String()))
	require.NoError(t, err, "Could not get docker client to check version")

	version, err := dockerClient.ServerVersion(ctx)
	require.NoError(t, err, "Could not get docker version")

	apiVersion := version.APIVersion
	// adding zero patch to use semver comparator
	// TODO: Implement non-semver comparator
	apiVersion += ".0"
	selector += ".0"

	match, err := utils.Version(apiVersion).Matches(selector)
	if err != nil {
		t.Fatalf("Could not check docker api version to match required: %v", err)
	}

	if !match {
		t.Skipf("Skipping test; requires %v, but api version is %v", selector, apiVersion)
	}
}

func RequireRegions(t *testing.T, supportedRegions []string, region string) {
	skipTest := true
	for _, supportedRegion := range supportedRegions {
		if region == supportedRegion {
			skipTest = false
		}
	}

	if skipTest {
		t.Skipf("Skipping the test in unsupported region: %s", *ECS.Config.Region)
	}
}

// RequireInstanceTypes skips the test if current instance type is not a supported instance type
func RequireInstanceTypes(t *testing.T, supportedTypePrefixes []string) {
	iid, _ := ec2.NewEC2MetadataClient(nil).InstanceIdentityDocument()
	instanceType := iid.InstanceType
	for _, prefix := range supportedTypePrefixes {
		if strings.HasPrefix(instanceType, prefix) {
			return
		}
	}

	t.Skipf("Skipped because the instance type %s is not a supported instance type. Supported instance type: %v",
		instanceType, supportedTypePrefixes)
}

// GetInstanceProfileName gets the instance profile name
func GetInstanceMetadata(path string) (string, error) {
	ec2MetadataClient := ec2metadata.New(session.New())
	return ec2MetadataClient.GetMetadata(path)
}

// GetInstanceIAMRole gets the iam roles attached to the instance profile
func GetInstanceIAMRole() (*iam.Role, error) {
	// This returns the name of the role
	instanceRoleName, err := GetInstanceMetadata("iam/security-credentials")
	if err != nil {
		return nil, fmt.Errorf("Error getting instance role name, err: %v", err)
	}
	if utils.ZeroOrNil(instanceRoleName) {
		return nil, fmt.Errorf("Instance Role name nil")
	}

	iamClient := iam.New(session.New())
	instanceRole, err := iamClient.GetRole(&iam.GetRoleInput{
		RoleName: aws.String(instanceRoleName),
	})
	if err != nil {
		return nil, err
	}

	return instanceRole.Role, nil
}

// SweepTask removes all the containers belong to a task
func (agent *TestAgent) SweepTask(task *TestTask) error {
	bodyData, err := agent.callTaskIntrospectionApi(*task.TaskArn)
	if err != nil {
		return err
	}

	var taskResponse v1.TaskResponse
	err = json.Unmarshal(*bodyData, &taskResponse)
	if err != nil {
		return err
	}

	for _, container := range taskResponse.Containers {
		ctx, _ := context.WithTimeout(context.Background(), 1*time.Minute)
		agent.DockerClient.ContainerRemove(ctx, container.DockerID, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   false,
			Force:         false,
		})
	}

	return nil
}

// AttributesToMap transforms a list of key, value attributes to return a map
func AttributesToMap(attributes []*ecs.Attribute) map[string]string {
	attributeMap := make(map[string]string)
	for _, attribute := range attributes {
		attributeMap[aws.StringValue(attribute.Name)] = aws.StringValue(attribute.Value)
	}
	return attributeMap
}

// GetSubnetID gets the subnet id for the instance from ec2 instance metadata
func GetSubnetID() (string, error) {
	ec2Metadata := ec2metadata.New(session.Must(session.NewSession()))
	mac, err := ec2Metadata.GetMetadata("mac")
	if err != nil {
		return "", errors.Wrapf(err, "unable to get mac from ec2 metadata")
	}
	subnet, err := ec2Metadata.GetMetadata("network/interfaces/macs/" + mac + "/subnet-id")
	if err != nil {
		return "", errors.Wrapf(err, "unable to get subnet from ec2 metadata")
	}

	return subnet, nil
}

// GetSecurityGroupIDs returns all of the security group IDs that the instance is in.
func GetSecurityGroupIDs() ([]string, error) {
	ec2Metadata := ec2metadata.New(session.Must(session.NewSession()))
	mac, err := ec2Metadata.GetMetadata("mac")
	if err != nil {
		return []string{}, errors.Wrapf(err, "unable to get mac from ec2 metadata")
	}
	sgroups, err := ec2Metadata.GetMetadata("network/interfaces/macs/" + mac + "/security-group-ids")
	if err != nil {
		return []string{}, errors.Wrapf(err, "unable to get security group ids from ec2 metadata")
	}

	return strings.Fields(sgroups), nil
}

// GetAccountID returns the aws account id from the instance metadata
func GetAccountID() (string, error) {
	ec2Metadata := ec2metadata.New(session.Must(session.NewSession()))

	instanceIdentity, err := ec2Metadata.GetInstanceIdentityDocument()
	if err != nil {
		return "", err
	}

	return instanceIdentity.AccountID, nil
}

// GetTaskID returns the task id from the task arn
func GetTaskID(taskARN string) (string, error) {
	// Parse taskARN
	parsedARN, err := arn.Parse(taskARN)
	if err != nil {
		return "", errors.Wrapf(err, "task get-id: malformed taskARN: %s", taskARN)
	}

	// Get task resource section
	resource := parsedARN.Resource

	if !strings.Contains(resource, arnResourceDelimiter) {
		return "", errors.Errorf("task get-id: malformed task resource: %s", resource)
	}

	resourceSplit := strings.SplitN(resource, arnResourceDelimiter, arnResourceSections)
	if len(resourceSplit) != arnResourceSections {
		return "", errors.Errorf("task get-id: invalid task resource split: %s, expected=%d, actual=%d", resource, arnResourceSections, len(resourceSplit))
	}

	// resourceSplit[1] can be "cluster/task-id" or "task-id", depends on whether task long arn format is turned on or not.
	if !strings.Contains(resourceSplit[1], "/") {
		return resourceSplit[1], nil
	}

	fields := strings.Split(resourceSplit[1], "/")
	return fields[1], nil
}

// WaitContainerInstanceStatus waits for a container instance to reach certain status by polling its status
func (agent *TestAgent) WaitContainerInstanceStatus(desiredStatus string, t *testing.T) error {
	timer := time.NewTimer(waitContainerInstanceStatusDuration)
	errChan := make(chan error, 1)
	containerInstanceStatus := ""

	cancelled := false
	go func() {
		for !cancelled {
			status, err := agent.getContainerInstanceStatus()
			if err != nil {
				t.Logf("Failed to get container instance status: %v", err)
				// There's eventual consistent issue in backend such that after we register a new container instance
				// and immediately describe it, that container instance might not be found. In that case, retry instead of
				// failing.
				retriable := false
				if strings.Contains(err.Error(), "MISSING") && desiredStatus == "ACTIVE" {
					retriable = true
				}

				if !retriable {
					errChan <- err
					return
				}
			}
			containerInstanceStatus = status

			if status == desiredStatus {
				break
			}
			if desiredStatus == "ACTIVE" {
				if status == "REGISTRATION_FAILED" || status == "INACTIVE" {
					errChan <- errors.Errorf("container instance ends at status %s; will never reach ACTIVE", status)
					return
				}
			}
			time.Sleep(5 * time.Second)
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-timer.C:
		cancelled = true
		return errors.Errorf("timed out waiting for container instance '%s' to reach 'ACTIVE', status is '%s'",
			agent.ContainerInstanceArn, containerInstanceStatus)
	}
}

func (agent *TestAgent) getContainerInstanceStatus() (string, error) {
	res, err := ECS.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(agent.Cluster),
		ContainerInstances: aws.StringSlice([]string{agent.ContainerInstanceArn}),
	})

	if err != nil {
		return "", err
	}

	if len(res.Failures) != 0 {
		return "", errors.Errorf("unable to describe container instance %s: %v", agent.ContainerInstanceArn, res.Failures)
	}

	return aws.StringValue(res.ContainerInstances[0].Status), nil
}

// GetNetworkInterfaceMacs returns a list of macs of all the network interfaces attached to the instance
func GetNetworkInterfaceMacs() ([]string, error) {
	macs, err := ec2.NewEC2MetadataClient(nil).AllENIMacs()
	if err != nil {
		return nil, err
	}

	return strings.Split(macs, "\n"), nil
}

// WaitNetworkInterfaceCount waits until there are certain number of ENIs attached to the instance
func WaitNetworkInterfaceCount(desiredCount int, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	errChan := make(chan error, 1)
	networkInterfaceCount := 0

	cancelled := false
	go func() {
		for !cancelled {
			macs, err := GetNetworkInterfaceMacs()
			count := len(macs)
			if err != nil {
				errChan <- err
				return
			}
			networkInterfaceCount = count

			if count == desiredCount {
				break
			}
			time.Sleep(5 * time.Second)
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-timer.C:
		cancelled = true
		return errors.Errorf("Timed out waiting for instance to have %d network interfaces attached; number of interfaces attached: %d",
			desiredCount, networkInterfaceCount)
	}
}

func IsEFSCapable() bool {
	// TODO: make this list betterer
	// Grabbed from the following page on 12/12/2019
	// https://aws.amazon.com/about-aws/global-infrastructure/regional-product-services/
	acceptableEFSRegions := []string{"us-east-1", "us-east-2", "us-west-2", "us-west-1", "ca-central-1", "sa-east-1", "us-gov-west-1", "eu-west-1", "eu-central-1", "eu-west-2", "eu-west-3", "eu-north-1", "me-south-1", "ap-southeast-1", "ap-northeast-1", "ap-southeast-2", "ap-northeast-2", "ap-south-1", "ap-east-1"}

	for _, r := range acceptableEFSRegions {
		if *ECS.Config.Region == r {
			return true
		}
	}
	return false
}
