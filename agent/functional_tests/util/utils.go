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

package util

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	docker "github.com/fsouza/go-dockerclient"
)

var ECS *ecs.ECS
var Cluster string

func init() {
	var ecsconfig aws.Config
	if region := os.Getenv("AWS_REGION"); region != "" {
		ecsconfig.Region = &region
	}
	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		ecsconfig.Region = &region
	}
	if ecsconfig.Region == nil {
		if iid, err := ec2.GetInstanceIdentityDocument(); err == nil {
			ecsconfig.Region = &iid.Region
		}
	}
	if envEndpoint := os.Getenv("ECS_BACKEND_HOST"); envEndpoint != "" {
		ecsconfig.Endpoint = &envEndpoint
	}

	ECS = ecs.New(session.New(&ecsconfig))
	Cluster = "ecs-functional-tests"
	if envCluster := os.Getenv("ECS_CLUSTER"); envCluster != "" {
		Cluster = envCluster
	}
	ECS.CreateCluster(&ecs.CreateClusterInput{
		ClusterName: aws.String(Cluster),
	})
}

// GetTaskDefinition is a helper that provies the family:revision for the named
// task definition where the name matches the folder in which the task
// definition is present. In order to avoid re-registering a task definition
// when it has already been regestered in the past, this registers a task
// definition of the pattern 'family-md5sum' with md5sum being the input task
// definition json's md5. This special family name is checked for existence
// before a new one is registered and it is assumed that if it exists, the task
// definition currently represented by the file was registered as such already.
func GetTaskDefinition(name string) (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	tdData, err := ioutil.ReadFile(filepath.Join(path.Dir(filename), "..", "testdata", "taskdefinitions", name, "task-definition.json"))
	if err != nil {
		return "", err
	}

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

	DockerClient *docker.Client
	t            *testing.T
}

type AgentOptions struct {
	ExtraEnvironment map[string]string
	ContainerLinks   []string
}

// RunAgent launches the agent and returns an object which may be used to reference it.
// It will wait until the agent is correctly registered before returning.
// 'version' may be a docker image (e.g. amazon/amazon-ecs-agent:v1.0.0) with
// tag that may be used to run the agent. It defaults to
// 'amazon/amazon-ecs-agent:make', the version created locally by running
// 'make'
func RunAgent(t *testing.T, options *AgentOptions) *TestAgent {
	agent := &TestAgent{t: t}
	agentImage := "amazon/amazon-ecs-agent:make"
	if envImage := os.Getenv("ECS_AGENT_IMAGE"); envImage != "" {
		agentImage = envImage
	}
	agent.Image = agentImage

	dockerClient, err := docker.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	agent.DockerClient = dockerClient

	_, err = dockerClient.InspectImage(agentImage)
	if err != nil {
		err = dockerClient.PullImage(docker.PullImageOptions{Repository: agentImage}, docker.AuthConfiguration{})
		if err != nil {
			t.Fatal("Could not launch agent", err)
		}
	}

	tmpdirOverride := os.Getenv("ECS_FTEST_TMP")

	agentTempdir, err := ioutil.TempDir(tmpdirOverride, "ecs_integ_testdata")
	if err != nil {
		t.Fatal("Could not create temp dir for test")
	}
	logdir := filepath.Join(agentTempdir, "logs")
	datadir := filepath.Join(agentTempdir, "data")
	os.Mkdir(logdir, 0755)
	os.Mkdir(datadir, 0755)
	agent.TestDir = agentTempdir
	agent.Options = options
	if options == nil {
		agent.Options = &AgentOptions{}
	}
	t.Logf("Created directory %s to store test data in", agentTempdir)
	err = agent.StartAgent()
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func (agent *TestAgent) StopAgent() error {
	return agent.DockerClient.StopContainer(agent.DockerID, 10)
}

func (agent *TestAgent) StartAgent() error {
	agent.t.Logf("Launching agent with image: %s\n", agent.Image)
	logdir := filepath.Join(agent.TestDir, "logs")
	datadir := filepath.Join(agent.TestDir, "data")
	agent.Logdir = logdir

	dockerConfig := &docker.Config{
		Image: agent.Image,
		ExposedPorts: map[docker.Port]struct{}{
			"51678/tcp": struct{}{},
		},
		Env: []string{
			"ECS_CLUSTER=" + Cluster,
			"ECS_DATADIR=/data",
			"ECS_LOGLEVEL=debug",
			"ECS_LOGFILE=/logs/integ_agent.log",
			"ECS_BACKEND_HOST=" + os.Getenv("ECS_BACKEND_HOST"),
			"AWS_ACCESS_KEY_ID=" + os.Getenv("AWS_ACCESS_KEY_ID"),
			"AWS_DEFAULT_REGION=" + *ECS.Config.Region,
			"AWS_SECRET_ACCESS_KEY=" + os.Getenv("AWS_SECRET_ACCESS_KEY"),
			"ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION=" + os.Getenv("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION"),
		},
		Cmd: strings.Split(os.Getenv("ECS_FTEST_AGENT_ARGS"), " "),
	}

	hostConfig := &docker.HostConfig{
		Binds: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
			logdir + ":/logs",
			datadir + ":/data",
		},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"51678/tcp": []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0"}},
		},
		Links: agent.Options.ContainerLinks,
	}

	if agent.Options != nil {
		for key, value := range agent.Options.ExtraEnvironment {
			dockerConfig.Env = append(dockerConfig.Env, key+"="+value)
		}
	}

	agentContainer, err := agent.DockerClient.CreateContainer(docker.CreateContainerOptions{
		Config:     dockerConfig,
		HostConfig: hostConfig,
	})
	if err != nil {
		agent.t.Fatal("Could not create agent container", err)
	}
	agent.DockerID = agentContainer.ID
	agent.t.Logf("Agent started as docker container: %s\n", agentContainer.ID)

	err = agent.DockerClient.StartContainer(agentContainer.ID, nil)
	if err != nil {
		return errors.New("Could not start agent container " + err.Error())
	}

	containerMetadata, err := agent.DockerClient.InspectContainer(agentContainer.ID)
	if err != nil {
		return errors.New("Could not inspect agent container: " + err.Error())
	}
	agent.IntrospectionURL = "http://localhost:" + containerMetadata.NetworkSettings.Ports["51678/tcp"][0].HostPort

	// Wait up to 10s for it to register
	var localMetadata handlers.MetadataResponse
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
	if localMetadata.ContainerInstanceArn == nil {
		agent.DockerClient.StopContainer(agent.DockerID, 1)
		return errors.New("Could not get agent metadata after launching it")
	}

	agent.ContainerInstanceArn = *localMetadata.ContainerInstanceArn
	agent.Cluster = localMetadata.Cluster
	if localMetadata.Version != "" {
		versionNumberRegex := regexp.MustCompile(` v(\d+\.\d+\.\d+) `)
		versionNumberStr := versionNumberRegex.FindStringSubmatch(localMetadata.Version)
		if len(versionNumberStr) == 2 {
			agent.Version = string(versionNumberStr[1])
		}
	}
	if agent.Version == "" {
		agent.Version = "UNKNOWN"
	}
	agent.t.Logf("Found agent metadata: %+v", localMetadata)
	return nil
}

func (agent *TestAgent) Cleanup() {
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

func (agent *TestAgent) StartMultipleTasks(t *testing.T, task string, num int) ([]*TestTask, error) {
	td, err := GetTaskDefinition(task)
	if err != nil {
		return nil, err
	}
	t.Logf("Task definition: %s", td)

	cis := make([]*string, num)
	for i := 0; i < num; i++ {
		cis[i] = &agent.ContainerInstanceArn
	}

	resp, err := ECS.StartTask(&ecs.StartTaskInput{
		Cluster:            &agent.Cluster,
		ContainerInstances: cis,
		TaskDefinition:     &td,
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
	tasks, err := agent.StartMultipleTasks(t, task, 1)
	if err != nil {
		return nil, err
	}
	return tasks[0], nil
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
	agentTaskResp, err := http.Get(agent.IntrospectionURL + "/v1/tasks?taskarn=" + *task.TaskArn)
	if err != nil {
		return "", err
	}
	bodyData, err := ioutil.ReadAll(agentTaskResp.Body)
	if err != nil {
		return "", err
	}
	var taskResp handlers.TaskResponse
	err = json.Unmarshal(bodyData, &taskResp)
	if err != nil {
		return "", err
	}
	if len(taskResp.Containers) == 0 {
		return "", errors.New("No containers in task response")
	}
	for _, container := range taskResp.Containers {
		if container.Name == containerName {
			return container.DockerId, nil
		}
	}
	return "", errors.New("No containers matched given name")
}

func (agent *TestAgent) RequireVersion(version string) {
	if agent.Version == "UNKNOWN" {
		agent.t.Skipf("Skipping test requiring version %v; agent version unknown", version)
	}

	matches, err := Version(agent.Version).Matches(version)
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
		return errors.New("Timed out waiting for task to reach " + status + ": " + *task.TaskDefinitionArn + ", " + *task.TaskArn)
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

func RequireDockerVersion(t *testing.T, selector string) {
	dockerClient, err := docker.NewClientFromEnv()
	if err != nil {
		t.Fatalf("Could not get docker client to check version: %v", err)
	}
	dockerVersion, err := dockerClient.Version()
	if err != nil {
		t.Fatalf("Could not get docker version: %v", err)
	}

	version := dockerVersion.Get("Version")

	match, err := Version(version).Matches(selector)
	if err != nil {
		t.Fatalf("Could not check docker version to match required: %v", err)
	}

	if !match {
		t.Skipf("Skipping test; requires %v, but version is %v", selector, version)
	}
}
