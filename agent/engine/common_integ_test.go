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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/containermetadata"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclientfactory"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	engineserviceconnect "github.com/aws/amazon-ecs-agent/agent/engine/serviceconnect"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"
	"github.com/aws/amazon-ecs-agent/ecs-agent/eventstream"
	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

var (
	sdkClientFactory sdkclientfactory.Factory
)

func init() {
	sdkClientFactory = sdkclientfactory.NewFactory(context.TODO(), dockerEndpoint)
}

func defaultTestConfigIntegTest() *config.Config {
	cfg, _ := config.NewConfig(ec2.NewBlackholeEC2MetadataClient())
	cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	cfg.ImagePullBehavior = config.ImagePullPreferCachedBehavior
	return cfg
}

func createTestTask(arn string) *apitask.Task {
	return &apitask.Task{
		Arn:                 arn,
		Family:              "family",
		Version:             "1",
		DesiredStatusUnsafe: apitaskstatus.TaskRunning,
		Containers:          []*apicontainer.Container{createTestContainer()},
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

func verifyContainerRunningStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning,
		"Expected container to be RUNNING")
}

func verifyContainerRunningStateChangeWithRuntimeID(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning,
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

func verifyContainerStoppedStateChange(t *testing.T, taskEngine TaskEngine) {
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerStopped,
		"Expected container to be STOPPED")
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
	stateChangeEvents := taskEngine.StateChangeEvents()
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).ContainerName, containerName)
	assert.Equal(t, event.(api.ContainerStateChange).Status, containerStatus)
}

func setup(cfg *config.Config, state dockerstate.TaskEngineState, t *testing.T) (TaskEngine, func(), credentials.Manager) {
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
	}, credentialsManager
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
func InitEventCollection(taskEngine TaskEngine) *TestEvents {
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
func VerifyTaskStatus(status apitaskstatus.TaskStatus, taskARN string, testEvents *TestEvents, t *testing.T) error {
	for {
		if _, found := testEvents.RecordedEvents[statechange.TaskEvent][status.String()][taskARN]; found {
			return nil
		}
		event := <-testEvents.StateChangeEvents
		RecordEvent(testEvents, event)
	}
}

// This method queries the TestEvents struct to check a Task Status.
// This method will block if there are no more stateChangeEvents from the DockerTaskEngine but is expected
func VerifyContainerStatus(status apicontainerstatus.ContainerStatus, ARNcontName string, testEvents *TestEvents, t *testing.T) error {
	for {
		if _, found := testEvents.RecordedEvents[statechange.ContainerEvent][status.String()][ARNcontName]; found {
			return nil
		}
		event := <-testEvents.StateChangeEvents
		RecordEvent(testEvents, event)
	}
}

// Will record the event that was just collected into the TestEvents struct's RecordedEvents map
func RecordEvent(testEvents *TestEvents, event statechange.Event) {
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
