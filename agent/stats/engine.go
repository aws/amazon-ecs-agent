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

//go:generate mockgen -destination=mock/$GOFILE -copyright_file=../../scripts/copyright_file github.com/aws/amazon-ecs-agent/agent/stats Engine

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"

	"github.com/cihub/seelog"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/docker/api/types"
)

const (
	containerChangeHandler = "DockerStatsEngineDockerEventsHandler"
	queueResetThreshold    = 2 * dockerclient.StatsInactivityTimeout
	hostNetworkMode        = "host"
	noneNetworkMode        = "none"
)

var (
	// EmptyMetricsError indicates an error for a task when there are no container
	// metrics to report
	EmptyMetricsError = errors.New("stats engine: no task metrics to report")
	// EmptyHealthMetricsError indicates an error for a task when there are no container
	// health metrics to report
	EmptyHealthMetricsError = errors.New("stats engine: no task health metrics to report")
)

// DockerContainerMetadataResolver implements ContainerMetadataResolver for
// DockerTaskEngine.
type DockerContainerMetadataResolver struct {
	dockerTaskEngine *ecsengine.DockerTaskEngine
}

// Engine defines methods to be implemented by the engine struct. It is
// defined to make testing easier.
type Engine interface {
	GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error)
	ContainerDockerStats(taskARN string, containerID string) (*types.StatsJSON, *NetworkStatsPerSec, error)
	GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error)
}

// DockerStatsEngine is used to monitor docker container events and to report
// utilization metrics of the same.

type DockerStatsEngine struct {
	ctx                        context.Context
	stopEngine                 context.CancelFunc
	client                     dockerapi.DockerClient
	cluster                    string
	containerInstanceArn       string
	lock                       sync.RWMutex
	config                     *config.Config
	containerChangeEventStream *eventstream.EventStream
	resolver                   resolver.ContainerMetadataResolver
	// tasksToContainers maps task arns to a map of container ids to StatsContainer objects.
	tasksToContainers map[string]map[string]*StatsContainer
	// tasksToHealthCheckContainers map task arns to the containers that has health check enabled
	tasksToHealthCheckContainers map[string]map[string]*StatsContainer
	// tasksToDefinitions maps task arns to task definition name and family metadata objects.
	tasksToDefinitions        map[string]*taskDefinition
	taskToTaskStats           map[string]*StatsTask
	taskToServiceConnectStats map[string]*ServiceConnectStats
}

// ResolveTask resolves the api task object, given container id.
func (resolver *DockerContainerMetadataResolver) ResolveTask(dockerID string) (*apitask.Task, error) {
	if resolver.dockerTaskEngine == nil {
		return nil, fmt.Errorf("Docker task engine uninitialized")
	}
	task, found := resolver.dockerTaskEngine.State().TaskByID(dockerID)
	if !found {
		return nil, fmt.Errorf("Could not map docker id to task: %s", dockerID)
	}

	return task, nil
}

func (resolver *DockerContainerMetadataResolver) ResolveTaskByARN(taskArn string) (*apitask.Task, error) {
	if resolver.dockerTaskEngine == nil {
		return nil, fmt.Errorf("docker task engine uninitialized")
	}
	task, found := resolver.dockerTaskEngine.State().TaskByArn(taskArn)
	if !found {
		return nil, fmt.Errorf("could not map task arn to task: %s", taskArn)
	}
	return task, nil

}

// ResolveContainer resolves the api container object, given container id.
func (resolver *DockerContainerMetadataResolver) ResolveContainer(dockerID string) (*apicontainer.DockerContainer, error) {
	if resolver.dockerTaskEngine == nil {
		return nil, fmt.Errorf("Docker task engine uninitialized")
	}
	container, found := resolver.dockerTaskEngine.State().ContainerByID(dockerID)
	if !found {
		return nil, fmt.Errorf("Could not map docker id to container: %s", dockerID)
	}

	return container, nil
}

// NewDockerStatsEngine creates a new instance of the DockerStatsEngine object.
// MustInit() must be called to initialize the fields of the new event listener.
func NewDockerStatsEngine(cfg *config.Config, client dockerapi.DockerClient, containerChangeEventStream *eventstream.EventStream) *DockerStatsEngine {
	return &DockerStatsEngine{
		client:                       client,
		resolver:                     nil,
		config:                       cfg,
		tasksToContainers:            make(map[string]map[string]*StatsContainer),
		tasksToHealthCheckContainers: make(map[string]map[string]*StatsContainer),
		tasksToDefinitions:           make(map[string]*taskDefinition),
		taskToTaskStats:              make(map[string]*StatsTask),
		taskToServiceConnectStats:    make(map[string]*ServiceConnectStats),
		containerChangeEventStream:   containerChangeEventStream,
	}
}

// synchronizeState goes through all the containers on the instance to synchronize the state on agent start
func (engine *DockerStatsEngine) synchronizeState() error {
	listContainersResponse := engine.client.ListContainers(engine.ctx, false, dockerclient.ListContainersTimeout)
	if listContainersResponse.Error != nil {
		return listContainersResponse.Error
	}

	for _, containerID := range listContainersResponse.DockerIDs {
		engine.addAndStartStatsContainer(containerID)
	}

	return nil
}

// addAndStartStatsContainer add the container into stats engine and start collecting the container stats
func (engine *DockerStatsEngine) addAndStartStatsContainer(containerID string) {
	engine.lock.Lock()
	defer engine.lock.Unlock()
	statsContainer, statsTaskContainer, err := engine.addContainerUnsafe(containerID)
	if err != nil {
		logger.Debug("Adding container to stats watchlist failed", logger.Fields{
			field.Container: containerID,
			field.Error:     err,
		})
		return
	}

	if engine.config.DisableMetrics.Enabled() || statsContainer == nil {
		return
	}

	statsContainer.StartStatsCollection()

	task, err := engine.resolver.ResolveTask(containerID)
	if err != nil {
		return
	}

	dockerContainer, errResolveContainer := engine.resolver.ResolveContainer(containerID)
	if errResolveContainer != nil {
		logger.Debug("Could not map container ID to container", logger.Fields{
			field.Container: containerID,
			field.Error:     err,
		})
		return
	}

	if task.IsNetworkModeAWSVPC() {
		// Start stats collector only for pause container
		if statsTaskContainer != nil && dockerContainer.Container.Type == apicontainer.ContainerCNIPause {
			statsTaskContainer.StartStatsCollection()
		} else {
			logger.Debug("Stats task container is nil, cannot start task stats collection", logger.Fields{
				field.Container: containerID,
			})
		}
	}

}

// MustInit initializes fields of the DockerStatsEngine object.
func (engine *DockerStatsEngine) MustInit(ctx context.Context, taskEngine ecsengine.TaskEngine, cluster string, containerInstanceArn string) error {
	derivedCtx, cancel := context.WithCancel(ctx)
	engine.stopEngine = cancel

	engine.ctx = derivedCtx
	// TODO ensure that this is done only once per engine object
	logger.Info("Initializing stats engine")
	engine.cluster = cluster
	engine.containerInstanceArn = containerInstanceArn

	var err error
	engine.resolver, err = newDockerContainerMetadataResolver(taskEngine)
	if err != nil {
		return err
	}

	// Subscribe to the container change event stream
	err = engine.containerChangeEventStream.Subscribe(containerChangeHandler, engine.handleDockerEvents)
	if err != nil {
		return fmt.Errorf("Failed to subscribe to container change event stream, err %v", err)
	}
	err = engine.synchronizeState()
	if err != nil {
		logger.Warn("Synchronize the container state failed", logger.Fields{
			field.Error: err,
		})
	}

	go engine.waitToStop()
	return nil
}

// Shutdown cleans up the resources after the stats engine.
func (engine *DockerStatsEngine) Shutdown() {
	engine.stopEngine()
	engine.Disable()
}

// Disable prevents this engine from managing any additional tasks.
func (engine *DockerStatsEngine) Disable() {
	engine.lock.Lock()
}

// waitToStop waits for the container change event stream close ans stop collection metrics
func (engine *DockerStatsEngine) waitToStop() {
	// Waiting for the event stream to close
	<-engine.containerChangeEventStream.Context().Done()
	logger.Debug("Event stream closed, stop listening to the event stream")
	engine.containerChangeEventStream.Unsubscribe(containerChangeHandler)
	engine.removeAll()
}

// removeAll stops the periodic usage data collection for all containers
func (engine *DockerStatsEngine) removeAll() {
	engine.lock.Lock()
	defer engine.lock.Unlock()

	for task, containers := range engine.tasksToContainers {
		for _, statsContainer := range containers {
			statsContainer.StopStatsCollection()
		}
		delete(engine.tasksToContainers, task)
	}

	for task := range engine.tasksToHealthCheckContainers {
		delete(engine.tasksToContainers, task)
	}
}

func (engine *DockerStatsEngine) addToStatsTaskMapUnsafe(task *apitask.Task, dockerContainerName string,
	containerType apicontainer.ContainerType) {
	var statsTaskContainer *StatsTask
	if task.IsNetworkModeAWSVPC() && containerType == apicontainer.ContainerCNIPause {
		// Excluding the pause container
		numberOfContainers := len(task.Containers) - 1
		var taskExists bool
		statsTaskContainer, taskExists = engine.taskToTaskStats[task.Arn]
		if !taskExists {
			containerInspect, err := engine.client.InspectContainer(engine.ctx, dockerContainerName,
				dockerclient.InspectContainerTimeout)
			if err != nil {
				return
			}
			containerpid := strconv.Itoa(containerInspect.State.Pid)
			statsTaskContainer, err = newStatsTaskContainer(task.Arn, task.GetID(), containerpid, numberOfContainers,
				engine.resolver, engine.config.PollingMetricsWaitDuration, task.ENIs)
			if err != nil {
				return
			}
			engine.taskToTaskStats[task.Arn] = statsTaskContainer
		} else {
			statsTaskContainer.TaskMetadata.NumberContainers = numberOfContainers
		}
	}
}

func (engine *DockerStatsEngine) addTaskToServiceConnectStatsUnsafe(taskArn string) {
	_, taskExists := engine.taskToServiceConnectStats[taskArn]
	if !taskExists {
		serviceConnectStats, err := newServiceConnectStats()
		if err != nil {
			seelog.Errorf("Error adding task %s to the service connect stats watchlist : %v", taskArn, err)
			return
		}
		engine.taskToServiceConnectStats[taskArn] = serviceConnectStats
	}
}

// addContainerUnsafe adds a container to the map of containers being watched.
func (engine *DockerStatsEngine) addContainerUnsafe(dockerID string) (*StatsContainer, *StatsTask, error) {
	// Make sure that this container belongs to a task and that the task
	// is not terminal.
	task, err := engine.resolver.ResolveTask(dockerID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not map container to task, ignoring container: %s", dockerID)
	}

	if len(task.Arn) == 0 || len(task.Family) == 0 {
		return nil, nil, errors.Errorf("stats add container: invalid task fields, arn: %s, familiy: %s", task.Arn, task.Family)
	}

	if task.GetKnownStatus().Terminal() {
		return nil, nil, errors.Errorf("stats add container: task is terminal, ignoring container: %s, task: %s", dockerID, task.Arn)
	}

	statsContainer, err := newStatsContainer(dockerID, engine.client, engine.resolver, engine.config)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not map docker container ID to container, ignoring container: %s", dockerID)
	}

	seelog.Debugf("Adding container to stats watch list, id: %s, task: %s", dockerID, task.Arn)
	engine.tasksToDefinitions[task.Arn] = &taskDefinition{family: task.Family, version: task.Version}

	dockerContainer, errResolveContainer := engine.resolver.ResolveContainer(dockerID)
	if errResolveContainer != nil {
		seelog.Debugf("Could not map container ID to container, container: %s, err: %s", dockerID, err)
	}

	watchStatsContainer := false
	if !engine.config.DisableMetrics.Enabled() {
		// Adding container to the map for collecting stats
		watchStatsContainer = engine.addToStatsContainerMapUnsafe(task.Arn, dockerID, statsContainer,
			engine.containerMetricsMapUnsafe)
		if errResolveContainer == nil {
			engine.addToStatsTaskMapUnsafe(task, dockerContainer.DockerName, dockerContainer.Container.Type)
		}
	}

	if errResolveContainer == nil && dockerContainer.Container.HealthStatusShouldBeReported() {
		// Track the container health status
		engine.addToStatsContainerMapUnsafe(task.Arn, dockerID, statsContainer, engine.healthCheckContainerMapUnsafe)
		seelog.Debugf("Adding container to stats health check watch list, id: %s, task: %s", dockerID, task.Arn)
	}

	if errResolveContainer == nil && task.GetServiceConnectContainer() == dockerContainer.Container {
		engine.addTaskToServiceConnectStatsUnsafe(task.Arn)
	}

	if !watchStatsContainer {
		return nil, nil, nil
	}
	return statsContainer, engine.taskToTaskStats[task.Arn], nil
}

func (engine *DockerStatsEngine) containerMetricsMapUnsafe() map[string]map[string]*StatsContainer {
	return engine.tasksToContainers
}

func (engine *DockerStatsEngine) healthCheckContainerMapUnsafe() map[string]map[string]*StatsContainer {
	return engine.tasksToHealthCheckContainers
}

// addToStatsContainerMapUnsafe adds the statscontainer into stats for tracking and returns a boolean indicates
// whether this container should be tracked for collecting metrics
func (engine *DockerStatsEngine) addToStatsContainerMapUnsafe(
	taskARN, containerID string,
	statsContainer *StatsContainer,
	statsMapToUpdate func() map[string]map[string]*StatsContainer) bool {

	taskToContainerMap := statsMapToUpdate()

	// Check if this container is already being watched.
	_, taskExists := taskToContainerMap[taskARN]
	if taskExists {
		// task arn exists in map.
		_, containerExists := taskToContainerMap[taskARN][containerID]
		if containerExists {
			// container arn exists in map.
			seelog.Debugf("Container already being watched, ignoring, id: %s", containerID)
			return false
		}
	} else {
		// Create a map for the task arn if it doesn't exist yet.
		taskToContainerMap[taskARN] = make(map[string]*StatsContainer)
	}
	taskToContainerMap[taskARN][containerID] = statsContainer

	return true
}

// GetInstanceMetrics gets all task metrics and instance metadata from stats engine.
func (engine *DockerStatsEngine) GetInstanceMetrics(includeServiceConnectStats bool) (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	var taskMetrics []*ecstcs.TaskMetric
	idle := engine.isIdle()
	metricsMetadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(engine.cluster),
		ContainerInstance: aws.String(engine.containerInstanceArn),
		Idle:              aws.Bool(idle),
		MessageId:         aws.String(uuid.NewRandom().String()),
	}
	if idle {
		seelog.Debug("Instance is idle. No task metrics to report")
		fin := true
		metricsMetadata.Fin = &fin
		return metricsMetadata, taskMetrics, nil
	}

	engine.lock.Lock()
	defer engine.lock.Unlock()

	if includeServiceConnectStats {
		err := engine.getServiceConnectStats()
		if err != nil {
			seelog.Errorf("Error getting service connect metrics: %v", err)
		}
	}

	for taskArn := range engine.tasksToContainers {
		containerMetrics, err := engine.taskContainerMetricsUnsafe(taskArn)
		if err != nil {
			seelog.Debugf("Error getting container metrics for task: %s, err: %v", taskArn, err)
			continue
		}

		if len(containerMetrics) == 0 {
			seelog.Debugf("Empty containerMetrics for task, ignoring, task: %s", taskArn)
			continue
		}

		taskDef, exists := engine.tasksToDefinitions[taskArn]
		if !exists {
			seelog.Debugf("Could not map task to definition, task: %s", taskArn)
			continue
		}

		metricTaskArn := taskArn
		taskMetric := &ecstcs.TaskMetric{
			TaskArn:               &metricTaskArn,
			TaskDefinitionFamily:  &taskDef.family,
			TaskDefinitionVersion: &taskDef.version,
			ContainerMetrics:      containerMetrics,
		}

		if includeServiceConnectStats {
			serviceConnectStats, ok := engine.taskToServiceConnectStats[taskArn]
			if !ok {
				seelog.Debugf("task '%s' is not registered to collect service connect metrics", taskArn)
				continue
			}
			if !serviceConnectStats.HasStatsBeenSent() {
				taskMetric.ServiceConnectMetricsWrapper = serviceConnectStats.GetStats()
				serviceConnectStats.SetStatsSent(true)
			}

		}

		taskMetrics = append(taskMetrics, taskMetric)
	}

	if len(taskMetrics) == 0 {
		// Not idle. Expect taskMetrics to be there.
		return nil, nil, EmptyMetricsError
	}

	engine.resetStatsUnsafe()
	return metricsMetadata, taskMetrics, nil
}

// GetTaskHealthMetrics returns the container health metrics
func (engine *DockerStatsEngine) GetTaskHealthMetrics() (*ecstcs.HealthMetadata, []*ecstcs.TaskHealth, error) {
	var taskHealths []*ecstcs.TaskHealth
	metadata := &ecstcs.HealthMetadata{
		Cluster:           aws.String(engine.cluster),
		ContainerInstance: aws.String(engine.containerInstanceArn),
		MessageId:         aws.String(uuid.NewRandom().String()),
	}

	if !engine.containerHealthsToMonitor() {
		return metadata, taskHealths, nil
	}

	engine.lock.RLock()
	defer engine.lock.RUnlock()

	for taskARN := range engine.tasksToHealthCheckContainers {
		taskHealth := engine.getTaskHealthUnsafe(taskARN)
		if taskHealth == nil {
			continue
		}
		taskHealths = append(taskHealths, taskHealth)
	}

	if len(taskHealths) == 0 {
		return nil, nil, EmptyHealthMetricsError
	}

	return metadata, taskHealths, nil
}

func (engine *DockerStatsEngine) isIdle() bool {
	engine.lock.RLock()
	defer engine.lock.RUnlock()

	return len(engine.tasksToContainers) == 0
}

func (engine *DockerStatsEngine) containerHealthsToMonitor() bool {
	engine.lock.RLock()
	defer engine.lock.RUnlock()

	return len(engine.tasksToHealthCheckContainers) != 0
}

// stopTrackingContainerUnsafe removes the StatsContainer from stats engine and
// returns true if the container is stopped or no longer tracked in agent. Otherwise
// it does nothing and return false
func (engine *DockerStatsEngine) stopTrackingContainerUnsafe(container *StatsContainer, taskARN string) bool {
	terminal, err := container.terminal()
	if err != nil {
		// Error determining if the container is terminal. This means that the container
		// id could not be resolved to a container that is being tracked by the
		// docker task engine. If the docker task engine has already removed
		// the container from its state, there's no point in stats engine tracking the
		// container. So, clean-up anyway.
		logger.Warn("Error determining if the container is terminal, removing from stats", logger.Fields{
			field.Container: container.containerMetadata.DockerID,
			field.Error:     err,
		})
		engine.doRemoveContainerUnsafe(container, taskARN)
		return true
	}
	if terminal {
		// Container is in known terminal state. Stop collection metrics.
		logger.Info("Container is terminal, removing from stats", logger.Fields{
			field.Container: container.containerMetadata.DockerID,
		})
		engine.doRemoveContainerUnsafe(container, taskARN)
		return true
	}

	return false
}

func (engine *DockerStatsEngine) getTaskHealthUnsafe(taskARN string) *ecstcs.TaskHealth {
	// Acquire the task definition information
	taskDefinition, ok := engine.tasksToDefinitions[taskARN]
	if !ok {
		seelog.Debugf("Could not map task to definitions, task: %s", taskARN)
		return nil
	}
	// Check all the stats container for the task
	containers, ok := engine.tasksToHealthCheckContainers[taskARN]
	if !ok {
		seelog.Debugf("Could not map task to health containers, task: %s", taskARN)
		return nil
	}

	// Aggregate container health information for all the containers in the task
	var containerHealths []*ecstcs.ContainerHealth
	for _, container := range containers {
		// check if the container is stopped/untracked, and remove it from stats
		//engine if needed
		if engine.stopTrackingContainerUnsafe(container, taskARN) {
			continue
		}
		dockerContainer, err := engine.resolver.ResolveContainer(container.containerMetadata.DockerID)
		if err != nil {
			seelog.Debugf("Could not resolve the Docker ID in agent state: %s", container.containerMetadata.DockerID)
			continue
		}

		// Check if the container has health check enabled
		if !dockerContainer.Container.HealthStatusShouldBeReported() {
			continue
		}

		healthInfo := dockerContainer.Container.GetHealthStatus()
		if healthInfo.Since == nil {
			// container was started but the health status isn't ready
			healthInfo.Since = aws.Time(time.Now())
		}
		containerHealth := &ecstcs.ContainerHealth{
			ContainerName: aws.String(dockerContainer.Container.Name),
			HealthStatus:  aws.String(healthInfo.Status.BackendStatus()),
			StatusSince:   aws.Time(healthInfo.Since.UTC()),
		}
		containerHealths = append(containerHealths, containerHealth)
	}

	if len(containerHealths) == 0 {
		return nil
	}

	taskHealth := &ecstcs.TaskHealth{
		Containers:            containerHealths,
		TaskArn:               aws.String(taskARN),
		TaskDefinitionFamily:  aws.String(taskDefinition.family),
		TaskDefinitionVersion: aws.String(taskDefinition.version),
	}

	return taskHealth
}

// handleDockerEvents must be called after openEventstream; it processes each
// event that it reads from the docker event stream.
func (engine *DockerStatsEngine) handleDockerEvents(events ...interface{}) error {
	for _, event := range events {
		dockerContainerChangeEvent, ok := event.(dockerapi.DockerContainerChangeEvent)
		if !ok {
			return fmt.Errorf("Unexpected event received, expected docker container change event")
		}

		switch dockerContainerChangeEvent.Status {
		case apicontainerstatus.ContainerRunning:
			engine.addAndStartStatsContainer(dockerContainerChangeEvent.DockerID)
		case apicontainerstatus.ContainerStopped:
			engine.removeContainer(dockerContainerChangeEvent.DockerID)
		default:
			seelog.Debugf("Ignoring event for container, id: %s, status: %d",
				dockerContainerChangeEvent.DockerID, dockerContainerChangeEvent.Status)
		}
	}

	return nil
}

// removeContainer deletes the container from the map of containers being watched.
// It also stops the periodic usage data collection for the container.
func (engine *DockerStatsEngine) removeContainer(dockerID string) {
	engine.lock.Lock()
	defer engine.lock.Unlock()

	// Make sure that this container belongs to a task.
	task, err := engine.resolver.ResolveTask(dockerID)
	if err != nil {
		seelog.Debugf("Could not map container to task, ignoring, err: %v, id: %s", err, dockerID)
		return
	}

	_, taskExists := engine.tasksToContainers[task.Arn]
	if !taskExists {
		seelog.Debugf("Container not being watched, id: %s", dockerID)
		return
	}

	// task arn exists in map.
	container, containerExists := engine.tasksToContainers[task.Arn][dockerID]
	if !containerExists {
		// container arn does not exist in map.
		seelog.Debugf("Container not being watched, id: %s", dockerID)
		return
	}

	engine.doRemoveContainerUnsafe(container, task.Arn)
}

// newDockerContainerMetadataResolver returns a new instance of DockerContainerMetadataResolver.
func newDockerContainerMetadataResolver(taskEngine ecsengine.TaskEngine) (*DockerContainerMetadataResolver, error) {
	dockerTaskEngine, ok := taskEngine.(*ecsengine.DockerTaskEngine)
	if !ok {
		// Error type casting docker task engine.
		return nil, fmt.Errorf("Could not load docker task engine")
	}

	resolver := &DockerContainerMetadataResolver{
		dockerTaskEngine: dockerTaskEngine,
	}

	return resolver, nil
}

// taskContainerMetricsUnsafe gets all container metrics for a task arn.
func (engine *DockerStatsEngine) taskContainerMetricsUnsafe(taskArn string) ([]*ecstcs.ContainerMetric, error) {
	containerMap, taskExists := engine.tasksToContainers[taskArn]
	if !taskExists {
		return nil, fmt.Errorf("task not found")
	}

	var containerMetrics []*ecstcs.ContainerMetric
	for _, container := range containerMap {
		dockerID := container.containerMetadata.DockerID
		// Check if the container is terminal. If it is, make sure that it is
		// cleaned up properly. We might sometimes miss events from docker task
		// engine and this helps in reconciling the state. The tcs client's
		// GetInstanceMetrics probe is used as the trigger for this.
		if engine.stopTrackingContainerUnsafe(container, taskArn) {
			continue
		}

		// CPU and Memory are both critical, so skip the container if either of these fail.
		cpuStatsSet, err := container.statsQueue.GetCPUStatsSet()
		if err != nil {
			logger.Error("Error collecting cloudwatch metrics for container", logger.Fields{
				field.Container: dockerID,
				field.Error:     err,
			})
			continue
		}
		memoryStatsSet, err := container.statsQueue.GetMemoryStatsSet()
		if err != nil {
			logger.Error("Error collecting cloudwatch metrics for container", logger.Fields{
				field.Container: dockerID,
				field.Error:     err,
			})
			continue
		}

		containerMetric := &ecstcs.ContainerMetric{
			ContainerName:  &container.containerMetadata.Name,
			CpuStatsSet:    cpuStatsSet,
			MemoryStatsSet: memoryStatsSet,
		}

		storageStatsSet, err := container.statsQueue.GetStorageStatsSet()
		if err != nil {
			logger.Warn("Error getting storage stats for container", logger.Fields{
				field.Container: dockerID,
				field.Error:     err,
			})
		} else {
			containerMetric.StorageStatsSet = storageStatsSet
		}

		task, err := engine.resolver.ResolveTask(dockerID)
		if err != nil {
			logger.Warn("Task not found for container", logger.Fields{
				field.Container: dockerID,
				field.Error:     err,
			})
		} else {
			// send network stats for default/bridge/nat/awsvpc network modes
			if !task.IsNetworkModeAWSVPC() && container.containerMetadata.NetworkMode != hostNetworkMode &&
				container.containerMetadata.NetworkMode != noneNetworkMode {
				networkStatsSet, err := container.statsQueue.GetNetworkStatsSet()
				if err != nil {
					// we log the error and still continue to publish cpu, memory stats
					logger.Warn("Error getting network stats for container", logger.Fields{
						field.Container: dockerID,
						field.Error:     err,
					})
				} else {
					containerMetric.NetworkStatsSet = networkStatsSet
				}
			} else if task.IsNetworkModeAWSVPC() {
				taskStatsMap, taskExistsInTaskStats := engine.taskToTaskStats[taskArn]
				if !taskExistsInTaskStats {
					return nil, fmt.Errorf("task not found")
				}
				if dockerContainer, err := engine.resolver.ResolveContainer(dockerID); err != nil {
					seelog.Debugf("Could not map container ID to container, container: %s, err: %s", dockerID, err)
				} else {
					// do not add network stats for pause container
					if dockerContainer.Container.Type != apicontainer.ContainerCNIPause {
						networkStats, err := taskStatsMap.StatsQueue.GetNetworkStatsSet()
						if err != nil {
							logger.Warn("Error getting network stats for container", logger.Fields{
								field.TaskARN:   taskArn,
								field.Container: dockerContainer.DockerID,
								field.Error:     err,
							})
						} else {
							containerMetric.NetworkStatsSet = networkStats
						}
					}
				}
			}
		}
		containerMetrics = append(containerMetrics, containerMetric)
	}
	return containerMetrics, nil
}

func (engine *DockerStatsEngine) doRemoveContainerUnsafe(container *StatsContainer, taskArn string) {
	container.StopStatsCollection()
	dockerID := container.containerMetadata.DockerID
	delete(engine.tasksToContainers[taskArn], dockerID)
	seelog.Debugf("Deleted container from tasks, id: %s", dockerID)

	if len(engine.tasksToContainers[taskArn]) == 0 {
		// No containers in task, delete task arn from map.
		delete(engine.tasksToContainers, taskArn)
		// No need to verify if the key exists in tasksToDefinitions.
		// Delete will do nothing if the specified key doesn't exist.
		delete(engine.tasksToDefinitions, taskArn)
		seelog.Debugf("Deleted task from tasks, arn: %s", taskArn)
	}

	if _, ok := engine.taskToServiceConnectStats[taskArn]; ok {
		delete(engine.taskToServiceConnectStats, taskArn)
		seelog.Debugf("Deleted task from service connect stats watch list, arn: %s", taskArn)
	}

	// Remove the container from health container watch list
	if _, ok := engine.tasksToHealthCheckContainers[taskArn][dockerID]; !ok {
		return
	}

	delete(engine.tasksToHealthCheckContainers[taskArn], dockerID)
	if len(engine.tasksToHealthCheckContainers[taskArn]) == 0 {
		delete(engine.tasksToHealthCheckContainers, taskArn)
		seelog.Debugf("Deleted task from container health watch list, arn: %s", taskArn)
	}
}

// resetStatsUnsafe resets stats for all watched containers.
func (engine *DockerStatsEngine) resetStatsUnsafe() {
	for _, containerMap := range engine.tasksToContainers {
		for _, container := range containerMap {
			container.statsQueue.Reset()
		}
	}
}

// ContainerDockerStats returns the last stored raw docker stats object for a container
func (engine *DockerStatsEngine) ContainerDockerStats(taskARN string, containerID string) (*types.StatsJSON, *NetworkStatsPerSec, error) {
	engine.lock.RLock()
	defer engine.lock.RUnlock()

	containerIDToStatsContainer, ok := engine.tasksToContainers[taskARN]
	taskToTaskStats := engine.taskToTaskStats
	if !ok {
		return nil, nil, errors.Errorf("stats engine: task '%s' for container '%s' not found",
			taskARN, containerID)
	}

	container, ok := containerIDToStatsContainer[containerID]
	if !ok {
		return nil, nil, errors.Errorf("stats engine: container not found: %s", containerID)
	}
	containerStats := container.statsQueue.GetLastStat()
	containerNetworkRateStats := container.statsQueue.GetLastNetworkStatPerSec()

	// Insert network stats in container stats
	task, err := engine.resolver.ResolveTaskByARN(taskARN)
	if err != nil {
		return nil, nil, errors.Errorf("stats engine: task '%s' not found",
			taskARN)
	}

	if task.IsNetworkModeAWSVPC() {
		taskStats, ok := taskToTaskStats[taskARN]
		if ok {
			if taskStats.StatsQueue.GetLastStat() != nil {
				containerStats.Networks = taskStats.StatsQueue.GetLastStat().Networks
			}
			containerNetworkRateStats = taskStats.StatsQueue.GetLastNetworkStatPerSec()
		} else {
			logger.Warn("Network stats not found for container", logger.Fields{
				field.TaskID:    task.GetID(),
				field.Container: containerID,
				field.Error:     err,
			})
		}
	}

	return containerStats, containerNetworkRateStats, nil
}

// getServiceConnectStats invokes the workflow to retrieve all service connect
// related metrics for all service connect enabled tasks
func (engine *DockerStatsEngine) getServiceConnectStats() error {
	var wg sync.WaitGroup

	for taskArn := range engine.taskToServiceConnectStats {
		wg.Add(1)
		task, err := engine.resolver.ResolveTaskByARN(taskArn)
		if err != nil {
			return errors.Errorf("stats engine: task '%s' not found", taskArn)
		}
		// TODO [SC]: Check if task is service-connect enabled
		serviceConnectStats, ok := engine.taskToServiceConnectStats[taskArn]
		if !ok {
			return errors.Errorf("task '%s' is not registered to collect service connect metrics", taskArn)
		}
		go func() {
			serviceConnectStats.retrieveServiceConnectStats(task)
			wg.Done()
		}()
	}
	wg.Wait()
	return nil
}
