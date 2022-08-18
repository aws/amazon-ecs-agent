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

// Package engine contains the core logic for managing tasks
package engine

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	mock_dockerapi "github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi/mocks"
	"github.com/aws/amazon-ecs-agent/agent/engine/execcmd"
	mock_engine "github.com/aws/amazon-ecs-agent/agent/engine/mocks"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	mock_ttime "github.com/aws/amazon-ecs-agent/agent/utils/ttime/mocks"
	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
)

const (
	containerID                 = "containerID"
	waitTaskStateChangeDuration = 2 * time.Minute
)

var (
	defaultDockerClientAPIVersion = dockerclient.Version_1_17
	// some unassigned ports to use for tests
	// see https://www.speedguide.net/port.php?port=24685
	unassignedPort int32 = 24685
)

// getUnassignedPort returns a NEW unassigned port each time it's called.
func getUnassignedPort() uint16 {
	return uint16(atomic.AddInt32(&unassignedPort, 1))
}

func discardEvents(from interface{}) func() {
	done := make(chan bool)

	go func() {
		for {
			ndx, _, _ := reflect.Select([]reflect.SelectCase{
				{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(from),
				},
				{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(done),
				},
			})
			if ndx == 1 {
				break
			}
		}
	}()
	return func() {
		done <- true
	}
}

// TODO: Move integ tests away from relying on the statechange channel for
// determining if a task is running/stopped or not
func verifyTaskIsRunning(stateChangeEvents <-chan statechange.Event, task *apitask.Task) error {
	for {
		event := <-stateChangeEvents
		if event.GetEventType() != statechange.TaskEvent {
			continue
		}

		taskEvent := event.(api.TaskStateChange)
		if taskEvent.TaskARN != task.Arn {
			continue
		}
		if taskEvent.Status == apitaskstatus.TaskRunning {
			return nil
		}
		if taskEvent.Status > apitaskstatus.TaskRunning {
			return fmt.Errorf("Task went straight to %s without running, task: %s", taskEvent.Status.String(), task.Arn)
		}
	}
}

func verifyTaskIsStopped(stateChangeEvents <-chan statechange.Event, task *apitask.Task) {
	for {
		event := <-stateChangeEvents
		if event.GetEventType() != statechange.TaskEvent {
			continue
		}
		taskEvent := event.(api.TaskStateChange)
		if taskEvent.TaskARN == task.Arn && taskEvent.Status >= apitaskstatus.TaskStopped {
			return
		}
	}
}

// waitForTaskStoppedByCheckStatus verify the task is in stopped status by checking the KnownStatusUnsafe field of the task
func waitForTaskStoppedByCheckStatus(task *apitask.Task) {
	for {
		if task.GetKnownStatus() == apitaskstatus.TaskStopped {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// validateContainerRunWorkflow validates the container create and start workflow
// for a test task without any resources (such as ENIs).
//
// The createdContainerName channel is used to emit the container name from the
// create operation. It can be used to validate that the name of the container
// removed matches with the generated container name during cleanup operation in the
// test.
func validateContainerRunWorkflow(t *testing.T,
	container *apicontainer.Container,
	task *apitask.Task,
	imageManager *mock_engine.MockImageManager,
	client *mock_dockerapi.MockDockerClient,
	roleCredentials *credentials.TaskIAMRoleCredentials,
	containerEventsWG *sync.WaitGroup,
	eventStream chan dockerapi.DockerContainerChangeEvent,
	createdContainerName chan<- string,
	assertions func(),
) {
	imageManager.EXPECT().AddAllImageStates(gomock.Any()).AnyTimes()
	client.EXPECT().PullImage(gomock.Any(), container.Image, nil, gomock.Any()).Return(dockerapi.DockerContainerMetadata{})
	imageManager.EXPECT().RecordContainerReference(container).Return(nil)
	imageManager.EXPECT().GetImageStateFromImageName(gomock.Any()).Return(nil, false)
	client.EXPECT().APIVersion().Return(defaultDockerClientAPIVersion, nil)
	dockerConfig, err := task.DockerConfig(container, defaultDockerClientAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	if roleCredentials != nil {
		// Container config should get updated with this during PostUnmarshalTask
		credentialsEndpointEnvValue := roleCredentials.IAMRoleCredentials.GenerateCredentialsEndpointRelativeURI()
		dockerConfig.Env = append(dockerConfig.Env, "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI="+credentialsEndpointEnvValue)
	}

	v3EndpointID := container.GetV3EndpointID()
	if v3EndpointID == "" {
		// if container's v3 endpoint id is not specified, set it here so it's not randomly generated
		// in execution; and then we can check whether the endpoint's value is expected
		v3EndpointID = uuid.New()
		container.SetV3EndpointID(v3EndpointID)
		metadataEndpointEnvValue := fmt.Sprintf(apicontainer.MetadataURIFormat, v3EndpointID)
		dockerConfig.Env = append(dockerConfig.Env, "ECS_CONTAINER_METADATA_URI="+metadataEndpointEnvValue)
		metadataEndpointEnvValueV4 := fmt.Sprintf(apicontainer.MetadataURIFormatV4, v3EndpointID)
		dockerConfig.Env = append(dockerConfig.Env, "ECS_CONTAINER_METADATA_URI_V4="+metadataEndpointEnvValueV4)
		agentAPIEndpointEnvValue := fmt.Sprintf(apicontainer.AgentAPIURIFormatV1, v3EndpointID)
		dockerConfig.Env = append(dockerConfig.Env, "ECS_AGENT_API_URI_V1="+agentAPIEndpointEnvValue)
	}
	// Container config should get updated with this during CreateContainer
	dockerConfig.Labels["com.amazonaws.ecs.task-arn"] = task.Arn
	dockerConfig.Labels["com.amazonaws.ecs.container-name"] = container.Name
	dockerConfig.Labels["com.amazonaws.ecs.task-definition-family"] = task.Family
	dockerConfig.Labels["com.amazonaws.ecs.task-definition-version"] = task.Version
	dockerConfig.Labels["com.amazonaws.ecs.cluster"] = ""
	client.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(ctx interface{}, config *dockercontainer.Config, y interface{}, containerName string, z time.Duration) {
			checkDockerConfigsExceptEnv(t, dockerConfig, config)
			checkDockerConfigsEnv(t, dockerConfig, config)
			// sleep5 task contains only one container. Just assign
			// the containerName to createdContainerName
			createdContainerName <- containerName
			containerEventsWG.Add(1)
			go func() {
				eventStream <- createDockerEvent(apicontainerstatus.ContainerCreated)
				containerEventsWG.Done()
			}()
		}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID})
	defaultConfig := config.DefaultConfig()
	client.EXPECT().StartContainer(gomock.Any(), containerID, defaultConfig.ContainerStartTimeout).Do(
		func(ctx interface{}, id string, timeout time.Duration) {
			containerEventsWG.Add(1)
			go func() {
				eventStream <- createDockerEvent(apicontainerstatus.ContainerRunning)
				containerEventsWG.Done()
			}()
		}).Return(dockerapi.DockerContainerMetadata{DockerID: containerID})
	assertions()
}

// checkDockerConfigsExceptEnv checks whether the contents in the docker config are expected
// except for the Env field. Checking for Env field is seperated because when agent converts
// its container config to docker config, it iterates over the container's env map and
// append them into docker config's env slice. So the sequence for the env slice is undetermined,
// and it needs other logic to check equality.
func checkDockerConfigsExceptEnv(t *testing.T, expectedConfig *dockercontainer.Config, config *dockercontainer.Config) {
	expectedConfigEnvList := expectedConfig.Env
	configEnvList := config.Env
	expectedConfig.Env = nil
	config.Env = nil

	assert.True(t, reflect.DeepEqual(expectedConfig, config),
		"Mismatch in container config; expected: %v, got: %v", expectedConfig, config)

	expectedConfig.Env = expectedConfigEnvList
	config.Env = configEnvList
}

// checkDockerConfigsEnv checks whether two docker configs have same list of environment
// variables and each has same value, ignoring the order.
func checkDockerConfigsEnv(t *testing.T, expectedConfig *dockercontainer.Config, config *dockercontainer.Config) {
	expectedConfigEnvList := expectedConfig.Env
	configEnvList := config.Env

	assert.ElementsMatchf(t, expectedConfigEnvList, configEnvList,
		"Mismatch in container config env; expected: %v, got: %v", expectedConfigEnvList, configEnvList)
}

// addTaskToEngine adds a test task to the engine. It waits for a task to reach the
// steady state before returning. Hence, this should not be used for tests, which
// expect container stops to be invoked before a task reaches its steady state
func addTaskToEngine(t *testing.T,
	ctx context.Context,
	taskEngine TaskEngine,
	sleepTask *apitask.Task,
	mockTime *mock_ttime.MockTime,
	createStartEventsReported *sync.WaitGroup) {
	// steadyStateCheckWait is used to force the test to wait until the steady-state check
	// has been invoked at least once
	mockTime.EXPECT().Now().Return(time.Now()).AnyTimes()

	err := taskEngine.Init(ctx)
	assert.NoError(t, err)

	taskEngine.AddTask(sleepTask)
	waitForRunningEvents(t, taskEngine.StateChangeEvents())
	// Wait for all events to be consumed prior to moving it towards stopped; we
	// don't want to race the below with these or we'll end up with the "going
	// backwards in state" stop and we haven't 'expect'd for that

	// Wait for container create and start events to be processed
	createStartEventsReported.Wait()
}

func createDockerEvent(status apicontainerstatus.ContainerStatus) dockerapi.DockerContainerChangeEvent {
	meta := dockerapi.DockerContainerMetadata{
		DockerID: containerID,
	}
	return dockerapi.DockerContainerChangeEvent{Status: status, DockerContainerMetadata: meta}
}

// waitForRunningEvents waits for a task to emit 'RUNNING' events for a container
// and the task
func waitForRunningEvents(t *testing.T, stateChangeEvents <-chan statechange.Event) {
	event := <-stateChangeEvents
	assert.Equal(t, event.(api.ContainerStateChange).Status, apicontainerstatus.ContainerRunning,
		"Expected container to be RUNNING")

	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskRunning,
		"Expected task to be RUNNING")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}
}

// waitForStopEvents waits for a task to emit 'STOPPED' events for a container
// and the task
func waitForStopEvents(t *testing.T, stateChangeEvents <-chan statechange.Event, verifyExitCode, execEnabled bool) {
	if execEnabled {
		event := <-stateChangeEvents
		if masc := event.(api.ManagedAgentStateChange); masc.Status != apicontainerstatus.ManagedAgentStopped {
			t.Fatal("Expected managed agent to stop first")
		}
	}

	event := <-stateChangeEvents
	if cont := event.(api.ContainerStateChange); cont.Status != apicontainerstatus.ContainerStopped {
		t.Fatal("Expected container to stop first")
		if verifyExitCode {
			assert.Equal(t, *cont.ExitCode, 1, "Exit code should be present")
		}
	}
	event = <-stateChangeEvents
	assert.Equal(t, event.(api.TaskStateChange).Status, apitaskstatus.TaskStopped, "Expected task to be STOPPED")

	select {
	case <-stateChangeEvents:
		t.Fatal("Should be out of events")
	default:
	}
}

func waitForContainerHealthStatus(t *testing.T, testTask *apitask.Task) {
	ctx, cancel := context.WithTimeout(context.TODO(), waitTaskStateChangeDuration)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Error("Timed out waiting for container health status")
		default:
			healthStatus := testTask.Containers[0].GetHealthStatus()
			if healthStatus.Status.BackendStatus() == "UNKNOWN" {
				time.Sleep(time.Second)
				continue
			}
			return
		}
	}
}

// sorts through stateChangeEvents to locate and assert that the ManagedAgent event matchess the expectedManagedAgent event.
// expectContainerEvent field is a boolean to allow us to ignore an expected empty channel
func checkManagedAgentEvents(t *testing.T, expectContainerEvent bool, stateChangeEvents <-chan statechange.Event,
	expectedManagedAgent apicontainer.ManagedAgent, waitDone chan<- struct{}) {
	if expectContainerEvent {
		for event := range stateChangeEvents {
			if managedAgentEvent, ok := event.(api.ManagedAgentStateChange); ok {
				// there is currently only ever a single managed agent
				assert.Equal(t, expectedManagedAgent.Status, managedAgentEvent.Status,
					"expected managedAgent container state change event did not match actual event")
				assert.Equal(t, expectedManagedAgent.Reason, managedAgentEvent.Reason,
					"expected managedAgent container state change event reports the wrong reason")
				close(waitDone)
				return
			}
			seelog.Debugf("processed errant event: %v", event)
		}
	} else {
		assert.Empty(t, stateChangeEvents, "expected empty stateChangeEvents channel, but found an event")
		close(waitDone)
	}
}

func enableExecCommandAgentForContainer(container *apicontainer.Container, state apicontainer.ManagedAgentState) {
	container.ManagedAgentsUnsafe = []apicontainer.ManagedAgent{
		{
			Name:              execcmd.ExecuteCommandAgentName,
			Properties:        make(map[string]string),
			ManagedAgentState: state,
		},
	}
}
