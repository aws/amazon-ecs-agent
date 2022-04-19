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

package stats

import (
	"context"
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/ecs_client/model/ecs"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	sdkClient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// checkPointSleep is the sleep duration in milliseconds between
	// starting/stopping containers in the test code.
	checkPointSleep              = 5 * time.Second
	testContainerHealthImageName = "amazon/amazon-ecs-containerhealthcheck:make"

	// defaultDockerTimeoutSeconds is the timeout for dialing the docker remote API.
	defaultDockerTimeoutSeconds = 10 * time.Second

	// waitForCleanupSleep is the sleep duration in milliseconds
	// for the waiting after container cleanup before checking the state of the manager.
	waitForCleanupSleep = 10 * time.Millisecond

	taskArn               = "gremlin"
	taskDefinitionFamily  = "docker-gremlin"
	taskDefinitionVersion = "1"
	containerName         = "gremlin-container"
)

var (
	cfg                      = config.DefaultConfig()
	defaultCluster           = "default"
	defaultContainerInstance = "ci"
)

func init() {
	cfg.EngineAuthData = config.NewSensitiveRawMessage([]byte{})
	cfg.ImagePullBehavior = config.ImagePullPreferCachedBehavior
}

// parseNanoTime returns the time object from a string formatted with RFC3339Nano layout.
func parseNanoTime(value string) time.Time {
	ts, _ := time.Parse(time.RFC3339Nano, value)
	return ts
}

// eventStream returns the event stream used to receive container change events
func eventStream(name string) *eventstream.EventStream {
	eventStream := eventstream.NewEventStream(name, context.Background())
	eventStream.StartListening()
	return eventStream
}

// createGremlin creates the gremlin container using the docker client.
// It is used only in the test code.
func createGremlin(client *sdkClient.Client, netMode string) (*dockercontainer.ContainerCreateCreatedBody, error) {
	containerGremlin, err := client.ContainerCreate(context.TODO(),
		&dockercontainer.Config{
			Image: testImageName,
		},
		&dockercontainer.HostConfig{
			NetworkMode: dockercontainer.NetworkMode(netMode),
		},
		&network.NetworkingConfig{},
		"")

	return &containerGremlin, err
}

func createHealthContainer(client *sdkClient.Client) (*dockercontainer.ContainerCreateCreatedBody, error) {
	container, err := client.ContainerCreate(context.TODO(),
		&dockercontainer.Config{
			Image: testContainerHealthImageName,
		},
		&dockercontainer.HostConfig{},
		&network.NetworkingConfig{},
		"")

	return &container, err
}

type IntegContainerMetadataResolver struct {
	containerIDToTask            map[string]*apitask.Task
	containerIDToDockerContainer map[string]*apicontainer.DockerContainer
}

func newIntegContainerMetadataResolver() *IntegContainerMetadataResolver {
	resolver := IntegContainerMetadataResolver{
		containerIDToTask:            make(map[string]*apitask.Task),
		containerIDToDockerContainer: make(map[string]*apicontainer.DockerContainer),
	}

	return &resolver
}

func (resolver *IntegContainerMetadataResolver) ResolveTask(containerID string) (*apitask.Task, error) {
	task, exists := resolver.containerIDToTask[containerID]
	if !exists {
		return nil, fmt.Errorf("unmapped container")
	}

	return task, nil
}

func (resolver *IntegContainerMetadataResolver) ResolveContainer(containerID string) (*apicontainer.DockerContainer, error) {
	container, exists := resolver.containerIDToDockerContainer[containerID]
	if !exists {
		return nil, fmt.Errorf("unmapped container")
	}

	return container, nil
}

func validateInstanceMetrics(t *testing.T, engine *DockerStatsEngine) {
	metadata, taskMetrics, err := engine.GetInstanceMetrics(false)
	assert.NoError(t, err, "gettting instance metrics failed")
	assert.NoError(t, validateMetricsMetadata(metadata), "validating metadata failed")
	assert.Len(t, taskMetrics, 1, "incorrect number of tasks")

	taskMetric := taskMetrics[0]
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionFamily), taskDefinitionFamily, "unexpected task definition family")
	assert.Equal(t, aws.StringValue(taskMetric.TaskDefinitionVersion), taskDefinitionVersion, "unexpected task definition version")
	assert.NoError(t, validateContainerMetrics(taskMetric.ContainerMetrics, 1), "validating container metrics failed")
}

func validateContainerMetrics(containerMetrics []*ecstcs.ContainerMetric, expected int) error {
	if len(containerMetrics) != expected {
		return fmt.Errorf("Mismatch in number of ContainerStatsSet elements. Expected: %d, Got: %d", expected, len(containerMetrics))
	}
	for _, containerMetric := range containerMetrics {
		if *containerMetric.ContainerName == "" {
			return fmt.Errorf("ContainerName is empty")
		}
		if containerMetric.CpuStatsSet == nil {
			return fmt.Errorf("CPUStatsSet is nil")
		}
		if containerMetric.MemoryStatsSet == nil {
			return fmt.Errorf("MemoryStatsSet is nil")
		}
		if containerMetric.NetworkStatsSet == nil {
			return fmt.Errorf("NetworkStatsSet is nil")
		}
		if containerMetric.StorageStatsSet == nil {
			return fmt.Errorf("StorageStatsSet is nil")
		}
	}
	return nil
}

func validateIdleContainerMetrics(t *testing.T, engine *DockerStatsEngine) {
	metadata, taskMetrics, err := engine.GetInstanceMetrics(false)
	assert.NoError(t, err, "getting instance metrics failed")
	assert.NoError(t, validateMetricsMetadata(metadata), "validating metadata failed")

	assert.True(t, aws.BoolValue(metadata.Idle), "expected idle metadata to be true")
	assert.True(t, aws.BoolValue(metadata.Fin), "fin not set to true when idle")
	assert.Len(t, taskMetrics, 0, "expected empty task metrics")
}

func validateMetricsMetadata(metadata *ecstcs.MetricsMetadata) error {
	if metadata == nil {
		return fmt.Errorf("Metadata is nil")
	}
	if *metadata.Cluster != defaultCluster {
		return fmt.Errorf("Expected cluster in metadata to be: %s, got %s",
			defaultCluster, *metadata.Cluster)
	}
	if *metadata.ContainerInstance != defaultContainerInstance {
		return fmt.Errorf("Expected container instance in metadata to be %s, got %s",
			defaultContainerInstance, *metadata.ContainerInstance)
	}
	if len(*metadata.MessageId) == 0 {
		return fmt.Errorf("Empty MessageId")
	}

	return nil
}

func validateHealthMetricsMetadata(metadata *ecstcs.HealthMetadata) error {
	if metadata == nil {
		return fmt.Errorf("metadata is nil")
	}

	if aws.StringValue(metadata.Cluster) != defaultCluster {
		return fmt.Errorf("expected cluster in metadata to be: %s, got %s",
			defaultCluster, aws.StringValue(metadata.Cluster))
	}

	if aws.StringValue(metadata.ContainerInstance) != defaultContainerInstance {
		return fmt.Errorf("expected container instance in metadata to be %s, got %s",
			defaultContainerInstance, aws.StringValue(metadata.ContainerInstance))
	}
	if len(aws.StringValue(metadata.MessageId)) == 0 {
		return fmt.Errorf("empty MessageId")
	}

	return nil
}

func validateContainerHealthMetrics(metrics []*ecstcs.ContainerHealth, expected int) error {
	if len(metrics) != expected {
		return fmt.Errorf("mismatch in number of ContainerHealth elements. Expected: %d, Got: %d",
			expected, len(metrics))
	}
	for _, health := range metrics {
		if aws.StringValue(health.ContainerName) == "" {
			return fmt.Errorf("container name is empty")
		}
		if aws.StringValue(health.HealthStatus) == "" {
			return fmt.Errorf("container health status is empty")
		}
		if aws.TimeValue(health.StatusSince).IsZero() {
			return fmt.Errorf("container health status change timestamp is empty")
		}
	}
	return nil
}

func validateTaskHealthMetrics(t *testing.T, engine *DockerStatsEngine) {
	healthMetadata, healthMetrics, err := engine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health metrics failed")
	require.Len(t, healthMetrics, 1)
	assert.NoError(t, validateHealthMetricsMetadata(healthMetadata), "validating health metedata failed")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskArn), taskArn, "task arn not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionFamily), taskDefinitionFamily, "task definition family not expected")
	assert.Equal(t, aws.StringValue(healthMetrics[0].TaskDefinitionVersion), taskDefinitionVersion, "task definition version not expected")
	assert.NoError(t, validateContainerHealthMetrics(healthMetrics[0].Containers, 1))
}

func validateEmptyTaskHealthMetrics(t *testing.T, engine *DockerStatsEngine) {
	// If the metrics is empty, no health metrics will be send, the metadata won't be used
	// no need to check the metadata here
	_, healthMetrics, err := engine.GetTaskHealthMetrics()
	assert.NoError(t, err, "getting task health failed")
	assert.Len(t, healthMetrics, 0, "no health metrics was expected")
}

func createFakeContainerStats() []*ContainerStats {
	netStats := &NetworkStats{
		RxBytes:   796,
		RxDropped: 6,
		RxErrors:  0,
		RxPackets: 10,
		TxBytes:   8192,
		TxDropped: 5,
		TxErrors:  0,
		TxPackets: 60,
	}
	return []*ContainerStats{
		{22400432, 1839104, uint64(100), uint64(200), netStats, parseNanoTime("2015-02-12T21:22:05.131117533Z")},
		{116499979, 3649536, uint64(300), uint64(400), netStats, parseNanoTime("2015-02-12T21:22:05.232291187Z")},
	}
}

type MockTaskEngine struct {
}

func (engine *MockTaskEngine) GetAdditionalAttributes() []*ecs.Attribute {
	return nil
}

func (engine *MockTaskEngine) Init(ctx context.Context) error {
	return nil
}
func (engine *MockTaskEngine) MustInit(ctx context.Context) {
}

func (engine *MockTaskEngine) StateChangeEvents() chan statechange.Event {
	return make(chan statechange.Event)
}

func (engine *MockTaskEngine) SetDataClient(data.Client) {
}

func (engine *MockTaskEngine) AddTask(*apitask.Task) {
}

func (engine *MockTaskEngine) ListTasks() ([]*apitask.Task, error) {
	return nil, nil
}

func (engine *MockTaskEngine) GetTaskByArn(arn string) (*apitask.Task, bool) {
	return nil, false
}

func (engine *MockTaskEngine) UnmarshalJSON([]byte) error {
	return nil
}

func (engine *MockTaskEngine) MarshalJSON() ([]byte, error) {
	return make([]byte, 0), nil
}

func (engine *MockTaskEngine) Version() (string, error) {
	return "", nil
}

func (engine *MockTaskEngine) LoadState() error {
	return nil
}

func (engine *MockTaskEngine) SaveState() error {
	return nil
}

func (engine *MockTaskEngine) Capabilities() []*ecs.Attribute {
	return nil
}

func (engine *MockTaskEngine) Disable() {
}

func (engine *MockTaskEngine) Info() (types.Info, error) {
	return types.Info{}, nil
}
