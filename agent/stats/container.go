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
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	"github.com/aws/amazon-ecs-agent/agent/stats/resolver"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	loggerfield "github.com/aws/amazon-ecs-agent/ecs-agent/logger/field"
	"github.com/aws/amazon-ecs-agent/ecs-agent/utils/retry"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

func newStatsContainer(dockerID string, client dockerapi.DockerClient, resolver resolver.ContainerMetadataResolver, cfg *config.Config, dataClient data.Client) (*StatsContainer, error) {
	dockerContainer, err := resolver.ResolveContainer(dockerID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &StatsContainer{
		containerMetadata: &ContainerMetadata{
			DockerID:    dockerID,
			Name:        dockerContainer.Container.Name,
			NetworkMode: dockerContainer.Container.GetNetworkMode(),
			StartedAt:   dockerContainer.Container.GetStartedAt(),
		},
		ctx:                    ctx,
		cancel:                 cancel,
		client:                 client,
		resolver:               resolver,
		config:                 cfg,
		restartAggregationData: dockerContainer.Container.GetRestartAggregationDataForStats(),
		dataClient:             dataClient,
	}, nil
}

func (container *StatsContainer) StartStatsCollection() {
	// queue will be sized to hold enough stats for 4 publishing intervals.
	var queueSize int
	if container.config != nil && container.config.PollMetrics.Enabled() {
		pollingInterval := container.config.PollingMetricsWaitDuration.Seconds()
		queueSize = int(config.DefaultContainerMetricsPublishInterval.Seconds() / pollingInterval * 4)
	} else {
		// for streaming stats we assume 1 stat every second
		queueSize = int(config.DefaultContainerMetricsPublishInterval.Seconds() * 4)
	}
	container.statsQueue = NewQueue(queueSize)
	go container.collect()
}

func (container *StatsContainer) StopStatsCollection() {
	container.cancel()
}

func (container *StatsContainer) collect() {
	dockerID := container.containerMetadata.DockerID
	backoff := retry.NewExponentialBackoff(time.Second*1, time.Second*10, 0.5, 2)
	for {
		err := container.processStatsStream()

		select {
		case <-container.ctx.Done():
			logger.Info("Stopping container stats collection", logger.Fields{"runtimeID": dockerID})
			return
		default:
			if err != nil {
				d := backoff.Duration()
				logger.Debug(fmt.Sprintf(
					"Error processing stats stream of container, backing off %s before reopening", d), logger.Fields{
					loggerfield.DockerId: dockerID,
					loggerfield.Error:    err,
				})
				time.Sleep(d)
			}
			// We were disconnected from the stats stream.
			// Check if the container is terminal. If it is, stop collecting metrics.
			// We might sometimes miss events from docker task  engine and this helps
			// in reconciling the state.
			terminal, err := container.terminal()
			if err != nil {
				// Error determining if the container is terminal. This means that the container
				// id could not be resolved to a container that is being tracked by the
				// docker task engine. If the docker task engine has already removed
				// the container from its state, there's no point in stats engine tracking the
				// container. So, clean-up anyway.
				seelog.Warnf("Container [%s]: Error determining if the container is terminal, stopping stats collection: %v", dockerID, err)
				container.StopStatsCollection()
				return
			} else if terminal {
				logger.Info("Container is terminal, stopping stats collection", logger.Fields{"runtimeID": dockerID})
				container.StopStatsCollection()
				return
			}
		}
	}
}

func (container *StatsContainer) getApiContainer(dockerID string) (*apicontainer.Container, error) {
	if container.resolver == nil {
		return nil, fmt.Errorf("Container ID=%s has no stats metadata resolver", dockerID)
	}
	dockerContainer, err := container.resolver.ResolveContainer(dockerID)
	if err != nil {
		return nil, err
	}
	return dockerContainer.Container, nil
}

func getNonDockerContainerStats(apiContainer *apicontainer.Container) NonDockerContainerStats {
	nonDockerStats := NonDockerContainerStats{}
	if apiContainer == nil {
		return nonDockerStats
	}
	if apiContainer.RestartPolicyEnabled() {
		restartCount := int64(apiContainer.RestartTracker.GetRestartCount())
		nonDockerStats.restartCount = &restartCount
	}
	return nonDockerStats
}

func (container *StatsContainer) processStatsStream() error {
	dockerID := container.containerMetadata.DockerID
	seelog.Debugf("Collecting stats for container %s", dockerID)
	if container.client == nil {
		return errors.New("container processStatsStream: Client is not set.")
	}
	dockerStats, errC := container.client.Stats(container.ctx, dockerID, dockerclient.StatsInactivityTimeout)

	apiContainer, err := container.getApiContainer(dockerID)
	if apiContainer == nil && err != nil {
		logger.Error("apiContainer is nil - will not be able to get restart stats set or determine "+
			"if container has restart policy enabled", logger.Fields{
			loggerfield.DockerId: dockerID,
			loggerfield.Error:    err,
		})
	}

	returnError := false
	for {
		select {
		case <-container.ctx.Done():
			return nil
		case err := <-errC:
			select {
			case <-container.ctx.Done():
				// ignore error when container.ctx.Done()
			default:
				seelog.Warnf("Error encountered processing metrics stream from docker, this may affect cloudwatch metric accuracy: %s", err)
				returnError = true
			}
		case rawStat, ok := <-dockerStats:
			if !ok {
				if returnError {
					return fmt.Errorf("error encountered processing metrics stream from docker")
				}
				return nil
			}
			containerEnabledRestartPolicy := false // default to false if apiContainer is nil
			if apiContainer != nil {
				containerEnabledRestartPolicy = apiContainer.RestartPolicyEnabled()
			}
			err := validateDockerStats(rawStat, containerEnabledRestartPolicy)
			if err != nil {
				return err
			}

			isFirstStatAfterContainerRestart := apiContainer != nil && apiContainer.RestartPolicyEnabled() &&
				container.syncContainerRestartAggregationData()
			err = container.statsQueue.AddContainerStat(rawStat, getNonDockerContainerStats(apiContainer),
				&container.restartAggregationData.LastStatBeforeLastRestart, container.hasRestartedBefore())
			if err != nil {
				seelog.Warnf("Container [%s]: error converting stats for container: %v", dockerID, err)
				continue
			}
			if isFirstStatAfterContainerRestart {
				err = container.saveRestartAggregationData(apiContainer)
				if err != nil {
					logger.Error("Failed to update container's stats restart aggregation data in database",
						logger.Fields{
							loggerfield.DockerId: dockerID,
							loggerfield.Error:    err,
						})
				}
			}
		}
	}
}

func (container *StatsContainer) terminal() (bool, error) {
	dockerContainer, err := container.resolver.ResolveContainer(container.containerMetadata.DockerID)
	if err != nil {
		return false, err
	}
	return dockerContainer.Container.KnownTerminal(), nil
}

// syncContainerRestartAggregationData updates a container's restart aggregation data if a container restart has been
// detected. This method returns true if a restart was detected and returns false otherwise.
func (container *StatsContainer) syncContainerRestartAggregationData() bool {
	_, dockerContainerData := container.client.DescribeContainer(container.ctx, container.containerMetadata.DockerID)
	restartDetected := dockerContainerData.StartedAt.Sub(container.containerMetadata.StartedAt) > 0
	if container.hasRestartedBefore() {
		restartDetected = dockerContainerData.StartedAt.Sub(container.restartAggregationData.LastRestartDetectedAt) > 0
	}

	if restartDetected {
		container.restartAggregationData.LastRestartDetectedAt = time.Now()

		logger.Debug("Received first stat for container after a container restart", logger.Fields{
			loggerfield.DockerId: container.containerMetadata.DockerID,
		})

		lastStat := container.statsQueue.GetLastStat()
		if lastStat != nil {
			container.restartAggregationData.LastStatBeforeLastRestart = *lastStat
		}
	}

	return restartDetected
}

// saveRestartAggregationData is used to save a container's restart aggregation data in Agent's local state database.
func (container *StatsContainer) saveRestartAggregationData(apiContainer *apicontainer.Container) error {
	apiContainer.SetRestartAggregationDataForStats(container.restartAggregationData)
	err := container.dataClient.SaveContainer(apiContainer)
	if err != nil {
		return errors.Wrap(err, "failed to save data for container")
	}

	return nil
}

// hasRestartedBefore determines if a container has ever restarted before.
func (container *StatsContainer) hasRestartedBefore() bool {
	return !container.restartAggregationData.LastRestartDetectedAt.IsZero()
}
