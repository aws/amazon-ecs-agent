//go:build sudo || integration
// +build sudo integration

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

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	engineserviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	ec2testutil "github.com/aws/amazon-ecs-agent/agent/utils/test/ec2util"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/ec2"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/userparser"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	log "github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	sdkClientFactory        sdkclientfactory.Factory
	mockTaskMetadataHandler = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte(`{"TaskARN": "arn:aws:ecs:region:account-id:task/task-id"}`))
	})
)

const (
	testECSRegion         = "us-east-1"
	testLogGroupName      = "test-fluentbit"
	testLogGroupPrefix    = "firelens-fluentbit-"
	validTaskArnPrefix    = "arn:aws:ecs:region:account-id:task/"
	testAppContainerImage = "public.ecr.aws/docker/library/busybox:latest"
	TestCluster           = "testCluster"
)

func init() {
	sdkClientFactory = sdkclientfactory.NewFactory(context.TODO(), dockerEndpoint)
}

func DefaultTestConfigIntegTest() *config.Config {
	cfg, _ := config.NewConfig(ec2testutil.FakeEC2MetadataClient{})
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.ImagePullBehavior = config.ImagePullPreferCachedBehavior
	return cfg
}

func CreateTestTask(arn string) *apitask.Task {
	return &apitask.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{CreateTestContainer()},
	}
}

func setupIntegTestLogs(t *testing.T) string {
	// Create a directory for storing test logs.
	testLogDir := t.TempDir()

	logger, err := log.LoggerFromConfigAsString(loggerConfigIntegrationTest(testLogDir))
	assert.NoError(t, err, "initialisation failed")

	err = log.ReplaceLogger(logger)
	assert.NoError(t, err, "unable to replace logger")

	return testLogDir
}

func setupGMSALinux(cfg *config.Config, state dockerstate.TaskEngineState, t *testing.T) (TaskEngine, func(), credentials.Manager) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	skipIntegTestIfApplicable(t)

	sdkClientFactory := sdkclientfactory.NewFactory(ctx, dockerEndpoint)
	dockerClient, err := dockerapi.NewDockerGoClient(sdkClientFactory, cfg, context.Background())
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	if state == nil {
		state = dockerstate.NewTaskEngineState()
	}
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetDataClient(data.NewNoopClient())
	metadataManager := containermetadata.NewManager(dockerClient, cfg)

	resourceFields := &taskresource.ResourceFields{
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			SSMClientCreator: ssmfactory.NewSSMClientCreator(),
			S3ClientCreator:  s3factory.NewS3ClientCreator(),
		},
		DockerClient: dockerClient,
	}
	hostResources := getTestHostResources()
	hostResourceManager := NewHostResourceManager(hostResources)
	daemonManagers := getTestDaemonManagers()

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, &hostResourceManager, state, metadataManager,
		resourceFields, execcmd.NewManager(), engineserviceconnect.NewManager(), daemonManagers)
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, credentialsManager
}

func loggerConfigIntegrationTest(logfile string) string {
	config := fmt.Sprintf(`
	<seelog type="asyncloop" minlevel="debug">
		<outputs formatid="main">
			<console />
			<rollingfile filename="%s/ecs-agent-log.log" type="date"
			 datepattern="2006-01-02-15" archivetype="none" maxrolls="24" />
		</outputs>
		<formats>
			<format id="main" format="%%UTCDate(2006-01-02T15:04:05Z07:00) [%%LEVEL] %%Msg%%n" />
			<format id="windows" format="%%Msg" />
		</formats>
	</seelog>`, logfile)

	return config
}

func VerifyContainerManifestPulledStateChange(t *testing.T, taskEngine TaskEngine) {
	// Skip assertions on Windows because we don't run a local registry,
	// so images are always cached and never pulled.
	if runtime.GOOS == "windows" {
		t.Log("Not expecting image manifest pulls on Windows")
		return
	}
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, apicontainerstatus.ContainerManifestPulled, event.(api.ContainerStateChange).Status,
		"Expected container to be at MANIFEST_PULLED state")
}

func VerifyTaskManifestPulledStateChange(t *testing.T, taskEngine TaskEngine) {
	// Skip assertions on Windows because we don't run a local registry,
	// so images are always cached and never pulled.
	if runtime.GOOS == "windows" {
		t.Log("Not expecting image manifest pulls on Windows")
		return
	}
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, apitaskstatus.TaskManifestPulled, event.(api.TaskStateChange).Status,
		"Expected task to reach MANIFEST_PULLED state")
}

func VerifyContainerRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, apicontainerstatus.ContainerRunning, event.(api.ContainerStateChange).Status,
		"Expected container to be RUNNING")
}

func VerifyTaskRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, apitaskstatus.TaskRunning, event.(api.TaskStateChange).Status,
		"Expected task to be RUNNING")
}

func verifyContainerRunningStateChangeWithRuntimeID(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, apicontainerstatus.ContainerRunning, event.(api.ContainerStateChange).Status,
		"Expected container to be RUNNING")
	assert.NotEqual(t, "", event.(api.ContainerStateChange).RuntimeID,
		"Expected container runtimeID should not empty")
}

func verifyExecAgentStateChange(t *testing.T, taskEngine TaskEngine,
	expectedStatus apicontainerstatus.ManagedAgentStatus, waitDone chan<- struct{}) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	for event := range stateChangeEvents {
		if managedAgentEvent, ok := event.(api.ManagedAgentStateChange); ok {
			if managedAgentEvent.Status == expectedStatus {
				close(waitDone)
				return
			}

		}
	}
}

func VerifyContainerStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	sc := event.(api.ContainerStateChange)
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped,
		"Expected container %s from task %s to be STOPPED", sc.RuntimeID, sc.TaskArn)
}

func verifyContainerStoppedStateChangeWithReason(t *testing.T, taskEngine TaskEngine, reason string) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped,
		"Expected container to be STOPPED")
	assert.Equal(t, reason, event.(api.ContainerStateChange).Reason)
}

func verifyContainerStoppedStateChangeWithRuntimeID(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped,
		"Expected container to be STOPPED")
	assert.NotEqual(t, "", event.(api.ContainerStateChange).RuntimeID,
		"Expected container runtimeID should not empty")
}

// verifySpecificContainerStateChange verifies that a specific container (identified by the containerName parameter),
// has a specific status (identified by the containerStatus parameter)
func verifySpecificContainerStateChange(t *testing.T, taskEngine TaskEngine, containerName string,
	containerStatus apicontainerstatus.ContainerStatus) {
	// Skip assertions on Windows because we don't run a local registry,
	// so images are always cached and never pulled.
	if runtime.GOOS == "windows" && containerStatus == apicontainerstatus.ContainerManifestPulled {
		t.Log("Not expecting image manifest pulls on Windows")
		return
	}
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).ContainerName, containerName)
	assert.Equal(t, event.(api.ContainerStateChange).Status, containerStatus)
}

func VerifyTaskStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped,
		"Expected task to be STOPPED")
}

func SetupIntegTestTaskEngine(cfg *config.Config, state dockerstate.TaskEngineState, t *testing.T) (TaskEngine, func(), dockerapi.DockerClient, credentials.Manager) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	skipIntegTestIfApplicable(t)

	sdkClientFactory := sdkclientfactory.NewFactory(ctx, dockerEndpoint)
	dockerClient, err := dockerapi.NewDockerGoClient(sdkClientFactory, cfg, context.Background())
	if err != nil {
		t.Fatalf("Error creating Docker client: %v", err)
	}
	credentialsManager := credentials.NewManager()
	if state == nil {
		state = dockerstate.NewTaskEngineState()
	}
	imageManager := NewImageManager(cfg, dockerClient, state)
	imageManager.SetDataClient(data.NewNoopClient())
	metadataManager := containermetadata.NewManager(dockerClient, cfg)
	hostResources := getTestHostResources()
	hostResourceManager := NewHostResourceManager(hostResources)
	daemonManagers := getTestDaemonManagers()

	taskEngine := NewDockerTaskEngine(cfg, dockerClient, credentialsManager,
		eventstream.NewEventStream("ENGINEINTEGTEST", context.Background()), imageManager, &hostResourceManager, state, metadataManager,
		nil, execcmd.NewManager(), engineserviceconnect.NewManager(), daemonManagers)
	taskEngine.MustInit(context.TODO())
	return taskEngine, func() {
		taskEngine.Shutdown()
	}, dockerClient, credentialsManager
}

func skipIntegTestIfApplicable(t *testing.T) {
	if os.Getenv("ECS_SKIP_ENGINE_INTEG_TEST") != "" {
		t.Skip("ECS_SKIP_ENGINE_INTEG_TEST")
	}
	if !isDockerRunning() {
		t.Skip("Docker not running")
	}
}

// Values in host resources from getTestHostResources() should be looked at and CPU/Memory assigned
// accordingly
func createTestContainerWithImageAndName(image string, name string) *apicontainer.Container {
	return &apicontainer.Container{
		Name:                name,
		Image:               image,
		Command:             []string{},
		Essential:           true,
		DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
		CPU:                 256,
		Memory:              128,
	}
}

func waitForTaskCleanup(t *testing.T, taskEngine TaskEngine, taskArn string, seconds int) {
	for i := 0; i < seconds; i++ {
		_, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(taskArn)
		if !ok {
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("timed out waiting for task to be clean up, task: %s", taskArn)
}

// A map that stores statusChangeEvents for both Tasks and Containers
// Organized first by EventType (Task or Container),
// then by StatusType (i.e. RUNNING, STOPPED, etc)
// then by Task/Container identifying string (TaskARN or ContainerName)
//
//	                 EventType
//	                /         \
//	        TaskEvent         ContainerEvent
//	      /          \           /        \
//	  RUNNING      STOPPED   RUNNING      STOPPED
//	  /    \        /    \      |             |
//	ARN1  ARN2    ARN3  ARN4  ARN:Cont1    ARN:Cont2
type EventSet map[statechange.EventType]statusToName

// Type definition for mapping a Status to a TaskARN/ContainerName
type statusToName map[string]nameSet

// Type definition for a generic set implemented as a map
type nameSet map[string]bool

// Holds the Events Map described above with a RW mutex
type TestEvents struct {
	RecordedEvents    EventSet
	StateChangeEvents <-chan statechange.Event
}

// Initializes the TestEvents using the TaskEngine. Abstracts the overhead required to set up
// collecting TaskEngine stateChangeEvents.
// We must use the Golang assert library and NOT the require library to ensure the Go routine is
// stopped at the end of our tests
func InitTestEventCollection(taskEngine TaskEngine) *TestEvents {
	stateChangeEvents := taskEngine.StateChangeEvents()
	recordedEvents := make(EventSet)
	testEvents := &TestEvents{
		RecordedEvents:    recordedEvents,
		StateChangeEvents: stateChangeEvents,
	}
	return testEvents
}

// This method queries the TestEvents struct to check a Task Status.
// This method will block if there are no more stateChangeEvents from the DockerTaskEngine but is expected
func VerifyTaskStatus(status apitaskstatus.TaskStatus, taskARN string, testEvents *TestEvents, t *testing.T) {
	// Skip assertions on Windows because we don't run a local registry,
	// so images are always cached and never pulled.
	if runtime.GOOS == "windows" && status == apitaskstatus.TaskManifestPulled {
		t.Log("Not expecting image manifest pulls on Windows")
		return
	}
	for {
		if _, found := testEvents.RecordedEvents[statechange.TaskEvent][status.String()][taskARN]; found {
			return
		}
		event := <-testEvents.StateChangeEvents
		RecordTestEvent(testEvents, event)
	}
}

// This method queries the TestEvents struct to check a Task Status.
// This method will block if there are no more stateChangeEvents from the DockerTaskEngine but is expected
func VerifyContainerStatus(status apicontainerstatus.ContainerStatus, ARNcontName string, testEvents *TestEvents, t *testing.T) {
	// Skip assertions on Windows because we don't run a local registry,
	// so images are always cached and never pulled.
	if runtime.GOOS == "windows" && status == apicontainerstatus.ContainerManifestPulled {
		t.Log("Not expecting image manifest pulls on Windows")
		return
	}
	for {
		if _, found := testEvents.RecordedEvents[statechange.ContainerEvent][status.String()][ARNcontName]; found {
			return
		}
		event := <-testEvents.StateChangeEvents
		RecordTestEvent(testEvents, event)
	}
}

// Will record the event that was just collected into the TestEvents struct's RecordedEvents map
func RecordTestEvent(testEvents *TestEvents, event statechange.Event) {
	switch event.GetEventType() {
	case statechange.TaskEvent:
		taskEvent := event.(api.TaskStateChange)
		if _, exists := testEvents.RecordedEvents[statechange.TaskEvent]; !exists {
			testEvents.RecordedEvents[statechange.TaskEvent] = make(statusToName)
		}
		if _, exists := testEvents.RecordedEvents[statechange.TaskEvent][taskEvent.Status.String()]; !exists {
			testEvents.RecordedEvents[statechange.TaskEvent][taskEvent.Status.String()] = make(map[string]bool)
		}
		testEvents.RecordedEvents[statechange.TaskEvent][taskEvent.Status.String()][taskEvent.TaskARN] = true
	case statechange.ContainerEvent:
		containerEvent := event.(api.ContainerStateChange)
		if _, exists := testEvents.RecordedEvents[statechange.ContainerEvent]; !exists {
			testEvents.RecordedEvents[statechange.ContainerEvent] = make(statusToName)
		}
		if _, exists := testEvents.RecordedEvents[statechange.ContainerEvent][containerEvent.Status.String()]; !exists {
			testEvents.RecordedEvents[statechange.ContainerEvent][containerEvent.Status.String()] = make(map[string]bool)
		}
		testEvents.RecordedEvents[statechange.ContainerEvent][containerEvent.Status.String()][containerEvent.TaskArn+":"+containerEvent.ContainerName] = true
	}
}

// setupTaskMetadataServer sets up a mock TMDS that the Firelens container needs access to.
func setupTaskMetadataServer(t *testing.T) {
	// Note that the listener has to be overwritten here, because the default one from httptest.NewServer
	// only listens to localhost and isn't reachable from container running in bridge network mode.
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	server := httptest.NewUnstartedServer(mockTaskMetadataHandler)
	server.Listener = l
	server.Start()
	defer server.Close()
	port := getPortFromAddr(t, server.URL)
	ec2MetadataClient, err := ec2.NewEC2MetadataClient(nil)
	require.NoError(t, err)
	serverURL := fmt.Sprintf("http://%s:%s", getHostPrivateIP(t, ec2MetadataClient), port)
	defer setV3MetadataURLFormat(serverURL + "/v3/%s")()
}

// getPortFromAddr returns the port part of an address in format "http://<addr>:<port>".
func getPortFromAddr(t *testing.T, addr string) string {
	u, err := url.Parse(addr)
	require.NoErrorf(t, err, "unable to parse address: %s", addr)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoErrorf(t, err, "unable to get port from address: %s", addr)
	return port
}

// getHostPrivateIP returns the host's private IP.
func getHostPrivateIP(t *testing.T, ec2MetadataClient ec2.EC2MetadataClient) string {
	ip, err := ec2MetadataClient.PrivateIPv4Address()
	require.NoError(t, err)
	return ip
}

// setV3MetadataURLFormat sets the container metadata URI format and returns a function to set it back.
func setV3MetadataURLFormat(fmt string) func() {
	backup := apicontainer.MetadataURIFormat
	apicontainer.MetadataURIFormat = fmt
	return func() {
		apicontainer.MetadataURIFormat = backup
	}
}

// createFirelensTask creates an ECS task with 2 containers – 1) Log-sender 2) Firelens container
func createFirelensTask(t *testing.T, firelensContainerImage, firelensContainerUser string) *apitask.Task {
	rawHostConfigInputForLogSender := dockercontainer.HostConfig{
		LogConfig: dockercontainer.LogConfig{
			Type: logDriverTypeFirelens,
			Config: map[string]string{
				"Name":              "cloudwatch_logs",
				"exclude-pattern":   "exclude",
				"include-pattern":   "include",
				"log_group_name":    testLogGroupName,
				"log_stream_prefix": testLogGroupPrefix,
				"region":            testECSRegion,
				"auto_create_group": "true",
			},
		},
	}
	rawHostConfigForLogSender, err := json.Marshal(&rawHostConfigInputForLogSender)
	require.NoError(t, err)

	rawConfigInputForLogRouter := dockercontainer.Config{
		User: firelensContainerUser,
	}
	rawConfigForLogRouter, err := json.Marshal(&rawConfigInputForLogRouter)
	require.NoError(t, err)

	testTask := CreateTestTask(validTaskArnPrefix + uuid.New())
	testTask.Containers = []*apicontainer.Container{
		{
			Name:      "app",
			Image:     testAppContainerImage,
			Essential: true,
			Command:   []string{"echo exclude; echo include"},
			DockerConfig: apicontainer.DockerConfig{
				HostConfig: func() *string {
					s := string(rawHostConfigForLogSender)
					return &s
				}(),
			},
		},
		{
			Name:      "log-router",
			Image:     firelensContainerImage,
			Essential: true,
			FirelensConfig: &apicontainer.FirelensConfig{
				Type: firelens.FirelensConfigTypeFluentbit,
				Options: map[string]string{
					"enable-ecs-log-metadata": "true",
				},
			},
			DockerConfig: apicontainer.DockerConfig{
				Config: func() *string {
					s := string(rawConfigForLogRouter)
					return &s
				}(),
			},
			Environment: map[string]string{
				"AWS_EXECUTION_ENV": "AWS_ECS_EC2",
				"FLB_LOG_LEVEL":     "debug",
			},
			TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
		},
	}
	testTask.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	return testTask
}

// verifyTaskStatusBasedOnAction verifies the task and container statuses
func verifyTaskStatusBasedOnAction(t *testing.T, taskEngine TaskEngine, action string) {
	switch action {
	case "start":
		// Verify 'log-router' container runs first, followed by the 'app' container
		verifySpecificContainerStateChange(t, taskEngine, "log-router", apicontainerstatus.ContainerRunning)
		verifySpecificContainerStateChange(t, taskEngine, "app", apicontainerstatus.ContainerRunning)
		// Verify task is in running state
		VerifyTaskRunningStateChange(t, taskEngine)
	case "stop":
		// Verify 'app' container stops first, followed by the 'log-router' container
		verifySpecificContainerStateChange(t, taskEngine, "app", apicontainerstatus.ContainerStopped)
		verifySpecificContainerStateChange(t, taskEngine, "log-router", apicontainerstatus.ContainerStopped)
		// Verify the task has stopped
		VerifyTaskStoppedStateChange(t, taskEngine)
	default:
		t.Errorf("Unexpected action: %s", action)
	}
}

// verifyFirelensDataDir verifies the configuration of the data directory, that is created for the Firelens task
func verifyFirelensDataDir(t *testing.T, firelensDataDir, firelensContainerUser string) {
	// Verify Firelens data directory exists
	_, err := os.Stat(firelensDataDir)
	require.NoError(t, err)

	// Validate subdirectories - config and socket
	firelensConfigDir := filepath.Join(firelensDataDir, "config")
	firelensSocketDir := filepath.Join(firelensDataDir, "socket")
	// Verify config directory exists
	configDirInfo, err := os.Stat(firelensConfigDir)
	require.NoError(t, err)
	// Verify socket directory exists
	socketDirInfo, err := os.Stat(firelensSocketDir)
	require.NoError(t, err)

	// Verify config directory ownership is set as per the ECS agent's user config
	sys := configDirInfo.Sys().(*syscall.Stat_t)
	assert.Equal(t, os.Getuid, sys.Uid,
		"Config directory owner UID doesn't match expected ECS agent UID")
	assert.Equal(t, os.Getgid, sys.Gid,
		"Config directory owner GID doesn't match expected ECS agent GID")

	// Verify socket directory ownership is set as per the Firelens container's user config
	userPart, groupPart, err := userparser.ParseUser(firelensContainerUser)
	require.NoError(t, err)
	sys = socketDirInfo.Sys().(*syscall.Stat_t)
	assert.Equal(t, userPart, sys.Uid,
		"Socket directory owner UID doesn't match expected Firelens container UID")
	assert.Equal(t, groupPart, sys.Gid,
		"Socket directory owner GID doesn't match expected Firelens container GID")
}

// verifyFirelensLogs verifies that the Firelens container sent app logs to CloudWatch as expected
func verifyFirelensLogs(t *testing.T, testTask *apitask.Task) {
	taskID := testTask.GetID()
	// Declare a cloudwatch client
	cwlClient := cloudwatchlogs.New(session.New(), aws.NewConfig().WithRegion(testECSRegion))
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(testLogGroupName),
		LogStreamName: aws.String(fmt.Sprintf("firelens-fluentbit-logsender-firelens-%s", taskID)),
	}
	// Wait for logs to appear in CloudWatch Logs
	resp, err := waitCloudwatchLogs(cwlClient, params)
	require.NoError(t, err)
	// There should only be one event as we are echoing only one thing that part of the include-filter
	assert.Equal(t, 1, len(resp.Events))
	message := aws.StringValue(resp.Events[0].Message)
	jsonBlob := make(map[string]string)
	err = json.Unmarshal([]byte(message), &jsonBlob)
	require.NoError(t, err)
	assert.Equal(t, "stdout", jsonBlob["source"])
	assert.Equal(t, "include", jsonBlob["log"])
	assert.Contains(t, jsonBlob, "container_id")
	assert.Contains(t, jsonBlob["container_name"], "logsender")
	assert.Equal(t, TestCluster, jsonBlob["ecs_cluster"])
	assert.Equal(t, testTask.Arn, jsonBlob["ecs_task_arn"])
}

func waitCloudwatchLogs(client *cloudwatchlogs.CloudWatchLogs, params *cloudwatchlogs.GetLogEventsInput) (*cloudwatchlogs.GetLogEventsOutput, error) {
	// The test could fail for timing issue, so retry for 60 seconds to make this test more stable
	for i := 0; i < 60; i++ {
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
	return nil, fmt.Errorf("timeout waiting for the logs to be sent to cloud watch logs")
}

// cleanupFirelensTask helps cleanup the Firelens task resources created during the test execution
func stopAndCleanupFirelensTask(t *testing.T, cfg *config.Config, testTask *apitask.Task, taskEngine TaskEngine,
	firelensDataDir string) {
	// Stop the Firelens task and cleanup
	testTask.SetSentStatus(apitaskstatus.TaskStopped)
	// Sleep for 3 times the task cleanup wait duration
	time.Sleep(3 * cfg.TaskCleanupWaitDuration)
	// Wait for Firelens task to be removed from the task engine
	for i := 0; i < 60; i++ {
		_, ok := taskEngine.(*DockerTaskEngine).State().TaskByArn(testTask.Arn)
		if !ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	// Make sure all the resource is cleaned up
	_, err := os.ReadDir(firelensDataDir)
	assert.Error(t, err)
}
