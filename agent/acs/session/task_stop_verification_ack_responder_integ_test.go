//go:build integration
// +build integration

package session_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/acs/session"
	"github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/envFiles"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/model/ecsacs"
	acssession "github.com/aws/amazon-ecs-agent/ecs-agent/acs/session"
	"github.com/aws/amazon-ecs-agent/ecs-agent/acs/session/testconst"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	apitaskstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/require"
)

// Tests that a task, its containers, and its resources are all stopped when a task stop verification ACK message is received.
func TestTaskStopVerificationACKResponder_StopsTaskContainersAndResources(t *testing.T) {
	taskEngine, done, dockerClient, _ := engine.SetupIntegTestTaskEngine(engine.DefaultTestConfigIntegTest(), nil, t)
	defer done()

	task := engine.CreateTestTask("test_task")
	createEnvironmentFileResources(task, 3)
	createLongRunningContainers(task, 3)
	go taskEngine.AddTask(task)

	for i := 0; i < len(task.Containers); i++ {
		engine.VerifyContainerManifestPulledStateChange(t, taskEngine)
	}
	engine.VerifyTaskManifestPulledStateChange(t, taskEngine)
	for i := 0; i < len(task.Containers); i++ {
		engine.VerifyContainerRunningStateChange(t, taskEngine)
	}
	engine.VerifyTaskRunningStateChange(t, taskEngine)

	manifestMessageIDAccessor := session.NewManifestMessageIDAccessor()
	require.NoError(t, manifestMessageIDAccessor.SetMessageID("manifest_message_id"))

	taskStopper := session.NewTaskStopper(taskEngine, data.NewNoopClient())
	responder := acssession.NewTaskStopVerificationACKResponder(taskStopper, manifestMessageIDAccessor, metrics.NewNopEntryFactory())

	handler := responder.HandlerFunc().(func(*ecsacs.TaskStopVerificationAck))
	handler(&ecsacs.TaskStopVerificationAck{
		GeneratedAt: aws.Int64(testconst.DummyInt),
		MessageId:   aws.String(manifestMessageIDAccessor.GetMessageID()),
		StopTasks:   []*ecsacs.TaskIdentifier{{TaskArn: aws.String(task.Arn)}},
	})

	// Wait for all state changes before verifying container, resource, and task statuses.
	for i := 0; i < len(task.Containers); i++ {
		engine.VerifyContainerStoppedStateChange(t, taskEngine)
	}
	engine.VerifyTaskStoppedStateChange(t, taskEngine)

	// Verify that all the task's containers have stopped.
	for _, container := range task.Containers {
		status, _ := dockerClient.DescribeContainer(context.Background(), container.RuntimeID)
		require.Equal(t, apicontainerstatus.ContainerStopped, status)
	}
	// Verify that all the tasks's resources have been removed.
	for _, resource := range task.GetResources() {
		require.Equal(t, resourcestatus.ResourceRemoved, resource.GetKnownStatus())
	}
	// Verify that the task has stopped.
	require.Equal(t, apitaskstatus.TaskStopped, task.GetKnownStatus())
}

// Tests that only the tasks specified in the task stop verification ACK message are stopped.
func TestTaskStopVerificationACKResponder_StopsSpecificTasks(t *testing.T) {
	taskEngine, done, dockerClient, _ := engine.SetupIntegTestTaskEngine(engine.DefaultTestConfigIntegTest(), nil, t)
	defer done()

	testEvents := engine.InitTestEventCollection(taskEngine)

	var tasks []*apitask.Task
	for i := 0; i < 3; i++ {
		task := engine.CreateTestTask(fmt.Sprintf("test_task_%d", i))
		createLongRunningContainers(task, 1)
		go taskEngine.AddTask(task)

		containerName := task.Arn + ":" + task.Containers[0].Name
		engine.VerifyContainerStatus(apicontainerstatus.ContainerManifestPulled, containerName, testEvents, t)
		engine.VerifyTaskStatus(apitaskstatus.TaskManifestPulled, task.Arn, testEvents, t)
		engine.VerifyContainerStatus(apicontainerstatus.ContainerRunning, containerName, testEvents, t)
		engine.VerifyTaskStatus(apitaskstatus.TaskRunning, task.Arn, testEvents, t)
		tasks = append(tasks, task)
	}

	manifestMessageIDAccessor := session.NewManifestMessageIDAccessor()
	require.NoError(t, manifestMessageIDAccessor.SetMessageID("manifest_message_id"))

	taskStopper := session.NewTaskStopper(taskEngine, data.NewNoopClient())
	responder := acssession.NewTaskStopVerificationACKResponder(taskStopper, manifestMessageIDAccessor, metrics.NewNopEntryFactory())

	// Stop the last 2 tasks.
	handler := responder.HandlerFunc().(func(*ecsacs.TaskStopVerificationAck))
	handler(&ecsacs.TaskStopVerificationAck{
		GeneratedAt: aws.Int64(testconst.DummyInt),
		MessageId:   aws.String(manifestMessageIDAccessor.GetMessageID()),
		StopTasks: []*ecsacs.TaskIdentifier{
			{TaskArn: aws.String(tasks[1].Arn)},
			{TaskArn: aws.String(tasks[2].Arn)},
		},
	})

	// Wait for all state changes before verifying container and task statuses.
	for _, task := range tasks[1:] {
		containerName := task.Arn + ":" + task.Containers[0].Name
		engine.VerifyContainerStatus(apicontainerstatus.ContainerStopped, containerName, testEvents, t)
		engine.VerifyTaskStatus(apitaskstatus.TaskStopped, task.Arn, testEvents, t)
	}

	// Verify that the last 2 tasks and their containers have stopped.
	for _, task := range tasks[1:] {
		status, _ := dockerClient.DescribeContainer(context.Background(), task.Containers[0].RuntimeID)
		require.Equal(t, apicontainerstatus.ContainerStopped, status)
		require.Equal(t, apitaskstatus.TaskStopped, task.GetKnownStatus())
	}

	// Verify that the first task and its container are still running.
	status, _ := dockerClient.DescribeContainer(context.Background(), tasks[0].Containers[0].RuntimeID)
	require.Equal(t, apicontainerstatus.ContainerRunning, status)
	require.Equal(t, apitaskstatus.TaskRunning, tasks[0].GetKnownStatus())
}

// Tests simple test cases, such as the happy path for 1 task with 1 container and edge cases where no tasks are stopped.
func TestTaskStopVerificationACKResponder(t *testing.T) {
	testCases := []struct {
		description string
		messageID   string
		taskArn     string
		stopTaskArn string
		shouldStop  bool
	}{
		{
			description: "stops a task",
			messageID:   "manifest_message_id",
			taskArn:     "test_task",
			stopTaskArn: "test_task",
			shouldStop:  true,
		},
		{
			description: "task not found",
			messageID:   "manifest_message_id",
			taskArn:     "test_task",
			stopTaskArn: "not_found_task",
		},
		{
			description: "invalid message id",
			taskArn:     "test_task",
			stopTaskArn: "test_task",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			taskEngine, done, dockerClient, _ := engine.SetupIntegTestTaskEngine(engine.DefaultTestConfigIntegTest(), nil, t)
			defer done()

			task := engine.CreateTestTask(tc.taskArn)
			createLongRunningContainers(task, 1)
			go taskEngine.AddTask(task)

			engine.VerifyContainerManifestPulledStateChange(t, taskEngine)
			engine.VerifyTaskManifestPulledStateChange(t, taskEngine)
			engine.VerifyContainerRunningStateChange(t, taskEngine)
			engine.VerifyTaskRunningStateChange(t, taskEngine)

			manifestMessageIDAccessor := session.NewManifestMessageIDAccessor()
			manifestMessageIDAccessor.SetMessageID(tc.messageID)

			taskStopper := session.NewTaskStopper(taskEngine, data.NewNoopClient())
			responder := acssession.NewTaskStopVerificationACKResponder(taskStopper, manifestMessageIDAccessor, metrics.NewNopEntryFactory())

			handler := responder.HandlerFunc().(func(*ecsacs.TaskStopVerificationAck))
			handler(&ecsacs.TaskStopVerificationAck{
				GeneratedAt: aws.Int64(testconst.DummyInt),
				MessageId:   aws.String(manifestMessageIDAccessor.GetMessageID()),
				StopTasks: []*ecsacs.TaskIdentifier{
					{TaskArn: aws.String(tc.stopTaskArn)},
				},
			})

			if tc.shouldStop {
				engine.VerifyContainerStoppedStateChange(t, taskEngine)
				engine.VerifyTaskStoppedStateChange(t, taskEngine)

				status, _ := dockerClient.DescribeContainer(context.Background(), task.Containers[0].RuntimeID)
				require.Equal(t, apicontainerstatus.ContainerStopped, status)
				require.Equal(t, apitaskstatus.TaskStopped, task.GetKnownStatus())
			} else {
				status, _ := dockerClient.DescribeContainer(context.Background(), task.Containers[0].RuntimeID)
				require.Equal(t, apicontainerstatus.ContainerRunning, status)
				require.Equal(t, apitaskstatus.TaskRunning, task.GetKnownStatus())
			}
		})
	}
}

func createEnvironmentFileResources(task *apitask.Task, n int) {
	task.ResourcesMapUnsafe = make(map[string][]taskresource.TaskResource)
	for i := 0; i < n; i++ {
		envFile := &envFiles.EnvironmentFileResource{}
		// Set known status to ResourceCreated to avoid downloading files from S3.
		envFile.SetKnownStatus(resourcestatus.ResourceCreated)
		task.AddResource(envFiles.ResourceName, envFile)
	}
}

func createLongRunningContainers(task *apitask.Task, n int) {
	var containers []*container.Container
	for i := 0; i < n; i++ {
		container := engine.CreateTestContainer()
		container.Command = engine.GetLongRunningCommand()
		container.Name = fmt.Sprintf("%s-%d", container.Name, i)
		containers = append(containers, container)
	}
	task.Containers = containers
}
