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

package stats

//go:generate mockgen.sh github.com/aws/amazon-ecs-agent/agent/stats Engine mock/$GOFILE

import (
	"fmt"
	"sync"

	"github.com/pborman/uuid"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	ecsengine "github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/aws/amazon-ecs-agent/agent/tcs/model/ecstcs"
	"github.com/aws/aws-sdk-go/aws"
	"golang.org/x/net/context"
)

var log = logger.ForModule("stats")

// DockerContainerMetadataResolver implements ContainerMetadataResolver for
// DockerTaskEngine.
type DockerContainerMetadataResolver struct {
	dockerTaskEngine *ecsengine.DockerTaskEngine
}

// Engine defines methods to be implemented by the engine struct. It is
// defined to make testing easier.
type Engine interface {
	GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error)
}

// DockerStatsEngine is used to monitor docker container events and to report
// utlization metrics of the same.
type DockerStatsEngine struct {
	client               ecsengine.DockerClient
	cluster              string
	containerInstanceArn string
	containersLock       sync.RWMutex
	ctx                  context.Context
	dockerGraphPath      string
	events               <-chan ecsengine.DockerContainerChangeEvent
	resolver             resolver.ContainerMetadataResolver
	// tasksToContainers maps task arns to a map of container ids to CronContainer objects.
	tasksToContainers map[string]map[string]*CronContainer
	// tasksToDefinitions maps task arns to task definiton name and family metadata objects.
	tasksToDefinitions         map[string]*taskDefinition
	unsubscribeContainerEvents context.CancelFunc
}

// dockerStatsEngine is a singleton object of DockerStatsEngine.
var dockerStatsEngine *DockerStatsEngine

// ResolveTask resolves the task arn, given container id.
func (resolver *DockerContainerMetadataResolver) ResolveTask(dockerID string) (*api.Task, error) {
	if resolver.dockerTaskEngine == nil {
		return nil, fmt.Errorf("Docker task engine uninitialized")
	}
	task, found := resolver.dockerTaskEngine.State().TaskById(dockerID)
	if !found {
		return nil, fmt.Errorf("Could not map docker id to task")
	}

	return task, nil
}

// NewDockerStatsEngine creates a new instance of the DockerStatsEngine object.
// MustInit() must be called to initialize the fields of the new event listener.
func NewDockerStatsEngine(cfg *config.Config) *DockerStatsEngine {
	if dockerStatsEngine == nil {
		dockerStatsEngine = &DockerStatsEngine{
			client:             nil,
			dockerGraphPath:    cfg.DockerGraphPath,
			resolver:           nil,
			tasksToContainers:  make(map[string]map[string]*CronContainer),
			tasksToDefinitions: make(map[string]*taskDefinition),
		}
	}

	return dockerStatsEngine
}

// MustInit initializes fields of the DockerStatsEngine object.
func (engine *DockerStatsEngine) MustInit(taskEngine ecsengine.TaskEngine, cluster string, containerInstanceArn string) error {
	log.Info("Initializing stats engine")
	err := engine.initDockerClient()
	if err != nil {
		return err
	}

	engine.cluster = cluster
	engine.containerInstanceArn = containerInstanceArn

	engine.resolver, err = newDockerContainerMetadataResolver(taskEngine)
	if err != nil {
		return err
	}

	return engine.Init()
}

// Init initializes the docker client's event engine. This must be called
// to subscribe to the docker's event stream.
func (engine *DockerStatsEngine) Init() error {
	engine.ctx, engine.unsubscribeContainerEvents = context.WithCancel(context.TODO())
	err := engine.openEventStream()
	if err != nil {
		return err
	}

	go engine.listContainersAndStartEventHandler()
	return nil
}

// listContainersAndStartEventHandler adds existing containers to the watch-list
// and starts the docker event handler.
func (engine *DockerStatsEngine) listContainersAndStartEventHandler() {
	// List and add existing containers to the list of containers to watch.
	err := engine.addExistingContainers()
	if err != nil {
		log.Warn("Error listing existing containers", "err", err)
		// Cancel the context to unsubscribe container events.
		engine.unsubscribeContainerEvents()
		return
	}

	go engine.handleDockerEvents()
}

// addExistingContainers lists existing containers and adds them to the engine.
func (engine *DockerStatsEngine) addExistingContainers() error {
	listContainersResponse := engine.client.ListContainers(false)
	if listContainersResponse.Error != nil {
		return listContainersResponse.Error
	}

	for _, containerID := range listContainersResponse.DockerIds {
		engine.addContainer(containerID)
	}

	return nil
}

// GetInstanceMetrics gets all task metrics and instance metadata from stats engine.
func (engine *DockerStatsEngine) GetInstanceMetrics() (*ecstcs.MetricsMetadata, []*ecstcs.TaskMetric, error) {
	var taskMetrics []*ecstcs.TaskMetric
	idle := engine.isIdle()
	metricsMetadata := &ecstcs.MetricsMetadata{
		Cluster:           aws.String(engine.cluster),
		ContainerInstance: aws.String(engine.containerInstanceArn),
		Idle:              aws.Bool(idle),
		MessageId:         aws.String(uuid.NewRandom().String()),
	}

	if idle {
		log.Debug("Instance is idle. No task metrics to report")
		fin := true
		metricsMetadata.Fin = &fin
		return metricsMetadata, taskMetrics, nil
	}

	for taskArn := range engine.tasksToContainers {
		containerMetrics, err := engine.getContainerMetricsForTask(taskArn)
		if err != nil {
			log.Debug("Error getting container metrics for task", "err", err, "task", taskArn)
			continue
		}

		if len(containerMetrics) == 0 {
			log.Debug("Empty containerMetrics for task, ignoring", "task", taskArn)
			continue
		}

		taskDef, exists := engine.tasksToDefinitions[taskArn]
		if !exists {
			log.Debug("Could not map task to definition", "task", taskArn)
			continue
		}

		metricTaskArn := taskArn
		taskMetric := &ecstcs.TaskMetric{
			TaskArn:               &metricTaskArn,
			TaskDefinitionFamily:  &taskDef.family,
			TaskDefinitionVersion: &taskDef.version,
			ContainerMetrics:      containerMetrics,
		}
		taskMetrics = append(taskMetrics, taskMetric)
	}

	if len(taskMetrics) == 0 {
		// Not idle. Expect taskMetrics to be there.
		return nil, nil, fmt.Errorf("No task metrics to report")
	}

	// Reset current stats. Retaining older stats results in incorrect utilization stats
	// until they are removed from the queue.
	engine.resetStats()
	return metricsMetadata, taskMetrics, nil
}

func (engine *DockerStatsEngine) isIdle() bool {
	return len(engine.tasksToContainers) == 0
}

// initDockerClient initializes engine's docker client.
func (engine *DockerStatsEngine) initDockerClient() error {
	if engine.client == nil {
		client, err := ecsengine.NewDockerGoClient(nil, "", config.NewSensitiveRawMessage([]byte("")), false)
		if err != nil {
			return err
		}
		engine.client = client
	}

	return nil
}

// openEventStream initializes the channel to receive events from docker client's
// event stream.
func (engine *DockerStatsEngine) openEventStream() error {
	events, err := engine.client.ContainerEvents(engine.ctx)
	if err != nil {
		return err
	}
	engine.events = events
	return nil
}

// handleDockerEvents must be called after openEventstream; it processes each
// event that it reads from the docker event stream.
func (engine *DockerStatsEngine) handleDockerEvents() {
	for {
		select {
		case <-engine.ctx.Done():
			return
		case event, ok := <-engine.events:
			if !ok {
				log.Crit("Docker event stream closed unexpectedly")
				return
			}
			log.Debug("Handling an event: ", "container", event.DockerId, "status", event.Status.String())
			switch event.Status {
			case api.ContainerRunning:
				engine.addContainer(event.DockerId)
			case api.ContainerStopped:
				engine.removeContainer(event.DockerId)
			default:
				log.Debug("Ignoring event for container", "id", event.DockerId, "status", event.Status)
			}
		}
	}
}

// addContainer adds a container to the map of containers being watched.
// It also starts the periodic usage data collection for the container.
func (engine *DockerStatsEngine) addContainer(dockerID string) {
	engine.containersLock.Lock()
	defer engine.containersLock.Unlock()

	// Make sure that this container belongs to a task and that the task
	// is not terminal.
	task, err := engine.resolver.ResolveTask(dockerID)
	if err != nil {
		log.Debug("Could not map container to task, ignoring", "err", err, "id", dockerID)
		return
	}

	if len(task.Arn) == 0 || len(task.Family) == 0 {
		log.Debug("Task has invalid fields", "id", dockerID)
		return
	}

	if task.KnownStatus.Terminal() {
		log.Debug("Task is terminal, ignoring", "id", dockerID)
		return
	}

	if err != nil {
		log.Debug("Could not get name for container, ignoring", "err", err, "id", dockerID)
		return
	}

	// Check if this container is already being watched.
	_, taskExists := engine.tasksToContainers[task.Arn]
	if taskExists {
		// task arn exists in map.
		_, containerExists := engine.tasksToContainers[task.Arn][dockerID]
		if containerExists {
			// container arn exists in map.
			log.Debug("Container already being watched, ignoring", "id", dockerID)
			return
		}
	} else {
		// Create a map for the task arn if it doesn't exist yet.
		engine.tasksToContainers[task.Arn] = make(map[string]*CronContainer)
	}

	log.Debug("Adding container to stats watch list", "id", dockerID, "task", task.Arn)
	container := newCronContainer(dockerID, engine.dockerGraphPath)
	engine.tasksToContainers[task.Arn][dockerID] = container
	engine.tasksToDefinitions[task.Arn] = &taskDefinition{family: task.Family, version: task.Version}
	container.StartStatsCron()
}

// removeContainer deletes the container from the map of containers being watched.
// It also stops the periodic usage data collection for the container.
func (engine *DockerStatsEngine) removeContainer(dockerID string) {
	engine.containersLock.Lock()
	defer engine.containersLock.Unlock()

	// Make sure that this container belongs to a task.
	task, err := engine.resolver.ResolveTask(dockerID)
	if err != nil {
		log.Debug("Could not map container to task, ignoring", "err", err, "id", dockerID)
		return
	}

	_, taskExists := engine.tasksToContainers[task.Arn]
	if !taskExists {
		log.Debug("Container not being watched", "id", dockerID)
		return
	}

	// task arn exists in map.
	container, containerExists := engine.tasksToContainers[task.Arn][dockerID]
	if !containerExists {
		// container arn does not exist in map.
		log.Debug("Container not being watched", "id", dockerID)
		return
	}

	container.StopStatsCron()
	delete(engine.tasksToContainers[task.Arn], dockerID)
	log.Debug("Deleted container from tasks", "id", dockerID)

	if len(engine.tasksToContainers[task.Arn]) == 0 {
		// No containers in task, delete task arn from map.
		delete(engine.tasksToContainers, task.Arn)
		// No need to verify if the key exists in tasksToDefinitions.
		// Delete will do nothing if the specified key doesn't exist.
		delete(engine.tasksToDefinitions, task.Arn)
		log.Debug("Deleted task from tasks", "arn", task.Arn)
	}
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

// getContainerMetricsForTask gets all container metrics for a task arn.
func (engine *DockerStatsEngine) getContainerMetricsForTask(taskArn string) ([]*ecstcs.ContainerMetric, error) {
	engine.containersLock.Lock()
	defer engine.containersLock.Unlock()

	containerMap, taskExists := engine.tasksToContainers[taskArn]
	if !taskExists {
		return nil, fmt.Errorf("Task not found")
	}

	var containerMetrics []*ecstcs.ContainerMetric
	for _, container := range containerMap {
		// Get CPU stats set.
		cpuStatsSet, err := container.statsQueue.GetCPUStatsSet()
		if err != nil {
			log.Warn("Error getting cpu stats", "err", err, "container", container.containerMetadata)
			continue
		}

		// Get memory stats set.
		memoryStatsSet, err := container.statsQueue.GetMemoryStatsSet()
		if err != nil {
			log.Warn("Error getting memory stats", "err", err, "container", container.containerMetadata)
			continue
		}

		containerMetrics = append(containerMetrics, &ecstcs.ContainerMetric{
			CpuStatsSet:    cpuStatsSet,
			MemoryStatsSet: memoryStatsSet,
		})

	}

	return containerMetrics, nil
}

// resetStats resets stats for all watched containers.
func (engine *DockerStatsEngine) resetStats() {
	engine.containersLock.Lock()
	defer engine.containersLock.Unlock()
	for _, containerMap := range engine.tasksToContainers {
		for _, container := range containerMap {
			container.statsQueue.Reset()
		}
	}
}

// newMetricsMetadata creates the singleton metadata object.
func newMetricsMetadata(cluster *string, containerInstance *string) *ecstcs.MetricsMetadata {
	return &ecstcs.MetricsMetadata{
		Cluster:           cluster,
		ContainerInstance: containerInstance,
	}
}
