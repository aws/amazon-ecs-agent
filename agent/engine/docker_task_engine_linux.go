//go:build linux
// +build linux

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
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/logger/field"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	dockercontainer "github.com/docker/docker/api/types/container"
)

const (
	// Constants for CNI timeout during setup and cleanup.
	cniSetupTimeout   = 1 * time.Minute
	cniCleanupTimeout = 30 * time.Second

	defaultKerberosTicketBindPath = "/var/credentials-fetcher/krbdir"
	readOnly                      = ":ro"
)

// updateTaskENIDependencies updates the task's dependencies for awsvpc networking mode.
// This method is used only on Windows platform.
func (engine *DockerTaskEngine) updateTaskENIDependencies(task *apitask.Task) {
}

// invokePluginForContainer is used to invoke the CNI plugin for the given container
// On non-windows platform, we will not invoke CNI plugins for non-pause containers
func (engine *DockerTaskEngine) invokePluginsForContainer(task *apitask.Task, container *apicontainer.Container) error {
	return nil
}

func (engine *DockerTaskEngine) watchAppNetImage(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error(fmt.Sprintf("failed to initialize fsnotify NewWatcher, error: %v", err))
	}
	appnetContainerTarballDir := engine.serviceconnectManager.GetAppnetContainerTarballDir()
	err = watcher.Add(appnetContainerTarballDir)
	if err != nil {
		logger.Error(fmt.Sprintf("error adding %s to fsnotify watcher, error: %v", appnetContainerTarballDir, err))
	}
	defer watcher.Close()

	// Start listening for events.
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				logger.Warn("fsnotify event watcher channel is closed")
				return
			}
			// check if the event file operation is write or create
			const writeOrCreateMask = fsnotify.Write | fsnotify.Create
			if event.Op&writeOrCreateMask != 0 {
				logger.Debug(fmt.Sprintf("new fsnotify watcher event: %s", event.Name))
				// reload the updated Appnet Agent image
				if err := engine.reloadAppNetImage(); err == nil {
					// restart the internal instance relay task with
					// updated Appnet Agent image
					engine.restartInstanceTask()
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				logger.Warn("fsnotify event watcher channel is closed")
				return
			}
			logger.Error(fmt.Sprintf("fsnotify watcher error: %v", err))
		case <-ctx.Done():
			return
		}
	}
}

func (engine *DockerTaskEngine) reloadAppNetImage() error {
	_, err := engine.serviceconnectManager.LoadImage(engine.ctx, engine.cfg, engine.client)
	if err != nil {
		logger.Error(fmt.Sprintf("engine: Failed to reload appnet Agent container, error: %v", err))
		return err
	}
	return nil
}

func (engine *DockerTaskEngine) restartInstanceTask() {
	if engine.serviceconnectRelay != nil {
		serviceconnectRelayTask, err := engine.serviceconnectManager.CreateInstanceTask(engine.cfg)
		if err != nil {
			logger.Error(fmt.Sprintf("Unable to start relay for task in the engine: %v", err))
			return
		}
		// clean up instance relay task
		for _, container := range engine.serviceconnectRelay.Containers {
			if container.Type == apicontainer.ContainerServiceConnectRelay {
				engine.stopContainer(engine.serviceconnectRelay, container)
			}
		}
		engine.serviceconnectRelay.SetDesiredStatus(apitaskstatus.TaskStopped)
		engine.sweepTask(engine.serviceconnectRelay)
		engine.deleteTask(engine.serviceconnectRelay)

		engine.serviceconnectRelay = serviceconnectRelayTask
		engine.AddTask(engine.serviceconnectRelay)
		logger.Info("engine: Restarted AppNet Relay task")
	}
}

// updateCredentialSpecMapping is used to map the bind location of kerberos ticket to the target location on the application container
func (engine *DockerTaskEngine) updateCredentialSpecMapping(taskID string, containerName string, desiredCredSpecInjection string, hostConfig *dockercontainer.HostConfig) {
	// Inject containers' hostConfig.Bind with the kerberos ticket bind
	logger.Info("Injecting container with kerberos ticket bind", logger.Fields{
		field.TaskID:           taskID,
		field.Container:        containerName,
		"kerberos ticket path": desiredCredSpecInjection,
	})

	// Inject containers' hostConfig.BindMount with the kerberos ticket location
	bindMountKerberosTickets := desiredCredSpecInjection + ":" + defaultKerberosTicketBindPath + readOnly
	if len(hostConfig.Binds) == 0 {
		hostConfig.Binds = []string{bindMountKerberosTickets}
	} else {
		hostConfig.Binds = append(hostConfig.Binds, bindMountKerberosTickets)
	}

	if len(hostConfig.SecurityOpt) != 0 {
		for idx, opt := range hostConfig.SecurityOpt {
			// credentialspec security opt is not supported by docker on linux
			if strings.HasPrefix(opt, "credentialspec:") {
				hostConfig.SecurityOpt = utils.Remove(hostConfig.SecurityOpt, idx)
			}
		}
	}
}
