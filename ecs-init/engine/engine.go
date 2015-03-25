// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"fmt"
	"github.com/aws/amazon-ecs-init/ecs-init/cache"
	"github.com/aws/amazon-ecs-init/ecs-init/docker"

	log "github.com/cihub/seelog"
)

// Engine contains methods invoked when ecs-init is run
type Engine struct {
	downloader downloader
	docker     dockerClient
}

// New creates an instance of Engine
func New() (*Engine, error) {
	downloader := cache.NewDownloader()
	docker, err := docker.NewClient()
	if err != nil {
		return nil, err
	}
	return &Engine{
		downloader: downloader,
		docker:     docker,
	}, nil
}

// PreStart prepares the ECS Agent for starting
func (e *Engine) PreStart() error {
	loaded, err := e.docker.IsAgentImageLoaded()
	if err != nil {
		return engineError("Could not check if Agent is loaded", err)
	}

	if loaded {
		return nil
	}

	cached := e.downloader.IsAgentCached()
	if !cached {
		err = e.mustDownloadAgent()
		if err != nil {
			return err
		}
	}

	log.Info("Loading Amazon EC2 Container Service Agent into Docker")
	cachedAgent, err := e.downloader.LoadCachedAgent()
	if err != nil {
		return engineError("Could not load Amazon EC2 Container Service Agent from cache", err)
	}
	defer cachedAgent.Close()
	err = e.docker.LoadImage(cachedAgent)
	if err != nil {
		return engineError("Could not load Amazon EC2 Container Service Agent into Docker", err)
	}
	return nil
}

// Start starts the ECS Agent and waits for it to stop
func (e *Engine) Start() error {
	err := e.docker.RemoveExistingAgentContainer()
	if err != nil {
		return engineError("Could not remove existing Agent container", err)
	}

	log.Info("Starting Amazon EC2 Container Service Agent")
	retval, err := e.docker.StartAgent()
	if err != nil {
		return engineError("Could not start Agent", err)
	}
	log.Infof("Agent exited with code %d", retval)
	return nil
}

// PreStop sends commands to Docker to stop the ECS Agent
func (e *Engine) PreStop() error {
	log.Info("Stopping Amazon EC2 Container Service Agent")
	err := e.docker.StopAgent()
	if err != nil {
		return engineError("Could not stop Amazon EC2 Container Service Agent", err)
	}
	return nil
}

// UpdateCache updates teh cached image of the ECS Agent
func (e *Engine) UpdateCache() error {
	latest := e.downloader.IsAgentLatest()
	if !latest {
		log.Info("Cached Amazon EC2 Container Service Agent does not match latest published")
		return e.mustDownloadAgent()
	}
	return nil
}

func (e *Engine) mustDownloadAgent() error {
	log.Info("Downloading Amazon EC2 Container Service Agent")
	err := e.downloader.DownloadAgent()
	if err != nil {
		return engineError("Could not download Amazon EC2 Container Serivce Agent", err)
	}
	return nil
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
