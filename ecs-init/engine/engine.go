// Copyright 2015-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"errors"
	"fmt"
	"io"

	"github.com/aws/amazon-ecs-init/ecs-init/cache"
	"github.com/aws/amazon-ecs-init/ecs-init/docker"
	"github.com/aws/amazon-ecs-init/ecs-init/exec"
	"github.com/aws/amazon-ecs-init/ecs-init/exec/iptables"
	"github.com/aws/amazon-ecs-init/ecs-init/exec/sysctl"
	"github.com/aws/amazon-ecs-init/ecs-init/gpu"

	log "github.com/cihub/seelog"
)

const (
	terminalSuccessAgentExitCode = 0
	terminalFailureAgentExitCode = 5
	upgradeAgentExitCode         = 42
)

// Engine contains methods invoked when ecs-init is run
type Engine struct {
	downloader            downloader
	docker                dockerClient
	loopbackRouting       loopbackRouting
	credentialsProxyRoute credentialsProxyRoute
	nvidiaGPUManager      gpu.GPUManager
}

// New creates an instance of Engine
func New() (*Engine, error) {
	downloader, err := cache.NewDownloader()
	if err != nil {
		return nil, err
	}
	docker, err := docker.NewClient()
	if err != nil {
		return nil, err
	}
	cmdExec := exec.NewExec()
	loopbackRouting, err := sysctl.NewIpv4RouteLocalNet(cmdExec)
	if err != nil {
		return nil, err
	}
	credentialsProxyRoute, err := iptables.NewNetfilterRoute(cmdExec)
	if err != nil {
		return nil, err
	}
	return &Engine{
		downloader:            downloader,
		docker:                docker,
		loopbackRouting:       loopbackRouting,
		credentialsProxyRoute: credentialsProxyRoute,
		nvidiaGPUManager:      gpu.NewNvidiaGPUManager(),
	}, nil
}

// PreStart prepares the ECS Agent for starting. It also configures the instance
// to handle credentials requests from containers by rerouting these requests to
// to the ECS Agent's credentials endpoint
func (e *Engine) PreStart() error {
	err := e.nvidiaGPUManager.Setup()
	if err != nil {
		return engineError("Nvidia GPU Manager", err)
	}
	// Enable use of loopback addresses for local routing purposes
	err = e.loopbackRouting.Enable()
	if err != nil {
		return engineError("could not enable loopback routing", err)
	}
	// Add the rerouting netfilter rule for credentials endpoint
	err = e.credentialsProxyRoute.Create()
	if err != nil {
		return engineError("could not create route to the credentials proxy", err)
	}

	imageLoaded, err := e.docker.IsAgentImageLoaded()
	if err != nil {
		return engineError("could not check Docker for Agent image presence", err)
	}

	switch e.downloader.AgentCacheStatus() {
	// Uncached, go get the Agent.
	case cache.StatusUncached:
		return e.downloadAndLoadCache()

	// The Agent is cached, and mandates a reload regardless of the
	// already loaded image.
	case cache.StatusReloadNeeded:
		return e.load(e.downloader.LoadCachedAgent())

	// Agent is cached, respect the already loaded Agent.
	case cache.StatusCached:
		if imageLoaded {
			return nil
		}
		return e.load(e.downloader.LoadCachedAgent())

	// There shouldn't be unhandled cache states.
	default:
		return errors.New("could not handle cache state")
	}
}

// ReloadCache reloads the cached image of the ECS Agent into Docker
func (e *Engine) ReloadCache() error {
	cached := e.downloader.IsAgentCached()
	if !cached {
		return e.downloadAndLoadCache()
	}
	return e.load(e.downloader.LoadCachedAgent())
}

func (e *Engine) downloadAndLoadCache() error {
	err := e.downloadAgent()
	if err != nil {
		return err
	}

	log.Info("Loading Amazon Elastic Container Service Agent into Docker")
	return e.load(e.downloader.LoadCachedAgent())
}

func (e *Engine) downloadAgent() error {
	log.Info("Downloading Amazon Elastic Container Service Agent")
	err := e.downloader.DownloadAgent()
	if err != nil {
		return engineError("could not download Amazon Elastic Container Service Agent", err)
	}
	return nil
}

func (e *Engine) load(image io.ReadCloser, err error) error {
	if err != nil {
		return engineError("could not load Amazon Elastic Container Service Agent from cache", err)
	}
	defer image.Close()
	err = e.docker.LoadImage(image)
	if err != nil {
		return engineError("could not load Amazon Elastic Container Service Agent into Docker", err)
	}
	return e.downloader.RecordCachedAgent()
}

// StartSupervised starts the ECS Agent and ensures it stays running, except for terminal errors (indicated by an agent exit code of 5)
func (e *Engine) StartSupervised() error {
	agentExitCode := -1
	for agentExitCode != terminalSuccessAgentExitCode && agentExitCode != terminalFailureAgentExitCode {
		err := e.docker.RemoveExistingAgentContainer()
		if err != nil {
			return engineError("could not remove existing Agent container", err)
		}

		log.Info("Starting Amazon Elastic Container Service Agent")
		agentExitCode, err = e.docker.StartAgent()
		if err != nil {
			return engineError("could not start Agent", err)
		}
		log.Infof("Agent exited with code %d", agentExitCode)
		if agentExitCode == upgradeAgentExitCode {
			err = e.upgradeAgent()
			if err != nil {
				log.Error("could not upgrade agent", err)
			}
		}
	}
	if agentExitCode == terminalFailureAgentExitCode {
		return errors.New("agent exited with terminal exit code")
	}
	return nil
}

func (e *Engine) upgradeAgent() error {
	log.Info("Loading new desired Amazon Elastic Container Service Agent into Docker")
	return e.load(e.downloader.LoadDesiredAgent())
}

// PreStop sends commands to Docker to stop the ECS Agent
func (e *Engine) PreStop() error {
	log.Info("Stopping Amazon Elastic Container Service Agent")
	err := e.docker.StopAgent()
	if err != nil {
		return engineError("could not stop Amazon Elastic Container Service Agent", err)
	}
	return nil
}

// PostStop cleans up the credentials endpoint setup by disabling loopback
// routing and removing the rerouting rule from the netfilter table
func (e *Engine) PostStop() error {
	log.Info("Cleaning up the credentials endpoint setup for Amazon Elastic Container Service Agent")
	err := e.loopbackRouting.RestoreDefault()

	// Ignore error from Remove() as the netfilter might never have been
	// addred in the first place
	e.credentialsProxyRoute.Remove()
	return err
}

type _engineError struct {
	err     error
	message string
}

func (e _engineError) Error() string {
	return fmt.Sprintf("%s: %s", e.message, e.err.Error())
}

func engineError(message string, err error) _engineError {
	return _engineError{
		message: message,
		err:     err,
	}
}
