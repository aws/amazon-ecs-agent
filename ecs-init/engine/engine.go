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
	"math"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-init/apparmor"
	"github.com/aws/amazon-ecs-agent/ecs-init/backoff"
	"github.com/aws/amazon-ecs-agent/ecs-init/cache"
	"github.com/aws/amazon-ecs-agent/ecs-init/chc"
	"github.com/aws/amazon-ecs-agent/ecs-init/config"
	"github.com/aws/amazon-ecs-agent/ecs-init/docker"
	"github.com/aws/amazon-ecs-agent/ecs-init/exec"
	"github.com/aws/amazon-ecs-agent/ecs-init/exec/iptables"
	"github.com/aws/amazon-ecs-agent/ecs-init/exec/sysctl"
	"github.com/aws/amazon-ecs-agent/ecs-init/gpu"

	log "github.com/cihub/seelog"
	ctrdapparmor "github.com/containerd/containerd/pkg/apparmor"
)

const (
	terminalSuccessAgentExitCode  = 0
	containerFailureAgentExitCode = 2
	TerminalFailureAgentExitCode  = 5
	DefaultInitErrorExitCode      = -1
	upgradeAgentExitCode          = 42
	serviceStartMinRetryTime      = time.Millisecond * 500
	serviceStartMaxRetryTime      = time.Second * 15
	serviceStartRetryJitter       = 0.10
	serviceStartRetryMultiplier   = 2.0
	serviceStartMaxRetries        = math.MaxInt64 // essentially retry forever
	failedContainerLogWindowSize  = "200"         // as string for log config
	mountFilePermission           = 0755
)

// Injection point for testing purposes
var (
	getDockerClient = func() (dockerClient, error) {
		return docker.Client()
	}
	hostSupports       = ctrdapparmor.HostSupports
	loadDefaultProfile = apparmor.LoadDefaultProfile
)

func dockerError(err error) error {
	return engineError("could not create docker client", err)
}

// Engine contains methods invoked when ecs-init is run
type Engine struct {
	downloader               downloader
	loopbackRouting          loopbackRouting
	credentialsProxyRoute    credentialsProxyRoute
	ipv6RouterAdvertisements ipv6RouterAdvertisements
	nvidiaGPUManager         gpu.GPUManager
}

type TerminalError struct {
	err      string
	exitCode int
}

func (e *TerminalError) Error() string {
	return fmt.Sprintf("%s: %d", e.err, e.exitCode)
}

// New creates an instance of Engine
func New() (*Engine, error) {
	downloader, err := cache.NewDownloader()
	if err != nil {
		return nil, err
	}

	cmdExec := exec.NewExec()
	loopbackRouting, err := sysctl.NewIpv4RouteLocalNet(cmdExec)
	if err != nil {
		return nil, err
	}
	ipv6RouterAdvertisements, err := sysctl.NewIpv6RouterAdvertisements(cmdExec)
	if err != nil {
		return nil, err
	}
	credentialsProxyRoute, err := iptables.NewNetfilterRoute(cmdExec)
	if err != nil {
		return nil, err
	}
	return &Engine{
		downloader:               downloader,
		loopbackRouting:          loopbackRouting,
		credentialsProxyRoute:    credentialsProxyRoute,
		ipv6RouterAdvertisements: ipv6RouterAdvertisements,
		nvidiaGPUManager:         gpu.NewNvidiaGPUManager(),
	}, nil
}

// PreStart prepares the ECS Agent for starting. It also configures the instance
// to handle credentials requests from containers by rerouting these requests to
// to the ECS Agent's credentials endpoint
func (e *Engine) PreStart() error {
	// setup gpu if necessary
	err := e.PreStartGPU()
	if err != nil {
		return err
	}
	// setup AppArmor if necessary
	err = e.PreStartAppArmor()
	if err != nil {
		return err
	}
	// Enable use of loopback addresses for local routing purposes
	log.Info("pre-start: enabling loopback routing")
	err = e.loopbackRouting.Enable()
	if err != nil {
		return engineError("could not enable loopback routing", err)
	}
	// Disable ipv6 router advertisements
	log.Info("pre-start: disabling ipv6 router advertisements")
	err = e.ipv6RouterAdvertisements.Disable()
	if err != nil {
		return engineError("could not disable ipv6 router advertisements", err)
	}
	// Add the rerouting netfilter rule for credentials endpoint
	log.Info("pre-start: creating credentials proxy route")
	err = e.credentialsProxyRoute.Create()
	if err != nil {
		return engineError("could not create route to the credentials proxy", err)
	}
	// Add the EBS Task Attach host mount point
	err = os.MkdirAll(config.MountDirectoryEBS(), mountFilePermission)
	if err != nil {
		return engineError("could not create EBS mount directory", err)
	}

	docker, err := getDockerClient()
	if err != nil {
		return dockerError(err)
	}
	log.Info("pre-start: checking ecs agent container image loaded presence")
	imageLoaded, err := docker.IsAgentImageLoaded()
	if err != nil {
		return engineError("could not check Docker for Agent image presence", err)
	}
	log.Infof("pre-start: ecs agent container image loaded presence: %t", imageLoaded)

	switch e.downloader.AgentCacheStatus() {
	// Uncached, go get the Agent.
	case cache.StatusUncached:
		log.Info("pre-start: downloading agent")
		return e.downloadAndLoadCache(docker)

	// The Agent is cached, and mandates a reload regardless of the
	// already loaded image.
	case cache.StatusReloadNeeded:
		log.Info("pre-start: reloading agent")
		return e.load(docker, e.downloader.LoadCachedAgent)

	// Agent is cached, respect the already loaded Agent.
	case cache.StatusCached:
		if imageLoaded {
			return nil
		}
		log.Info("pre-start: loading cached agent")
		return e.load(docker, e.downloader.LoadCachedAgent)

	// There shouldn't be unhandled cache states.
	default:
		return errors.New("could not handle cache state")
	}
}

// PreStartGPU sets up the nvidia gpu manager if it's enabled.
func (e *Engine) PreStartGPU() error {
	docker, err := getDockerClient()
	if err != nil {
		return dockerError(err)
	}
	envVariables := docker.LoadEnvVars()
	if val, ok := envVariables[config.GPUSupportEnvVar]; ok {
		if val == "true" {
			log.Info("pre-start: setting up GPUs")
			defer log.Info("pre-start: done setting up GPUs")
			err := e.nvidiaGPUManager.Setup()
			if err != nil {
				log.Errorf("Nvidia GPU Manager: %v", err)
				return engineError("Nvidia GPU Manager", err)
			}
		}
	}
	return nil
}

// PreStartAppArmor sets up the ecs-agent-default AppArmor profile if we're running
// on an AppArmor-enabled system.
func (e *Engine) PreStartAppArmor() error {
	if hostSupports() {
		log.Infof("pre-start: setting up %s AppArmor profile", apparmor.ECSAgentDefaultProfileName)
		return loadDefaultProfile(apparmor.ECSAgentDefaultProfileName)
	}
	return nil
}

// ReloadCache reloads the cached image of the ECS Agent into Docker
func (e *Engine) ReloadCache() error {
	docker, err := getDockerClient()
	if err != nil {
		return dockerError(err)
	}
	cached := e.downloader.IsAgentCached()
	if !cached {
		return e.downloadAndLoadCache(docker)
	}
	return e.load(docker, e.downloader.LoadCachedAgent)
}

func (e *Engine) downloadAndLoadCache(docker dockerClient) error {
	err := e.downloadAgent()
	if err != nil {
		return err
	}

	log.Info("Loading Amazon Elastic Container Service Agent into Docker")
	return e.load(docker, e.downloader.LoadCachedAgent)
}

func (e *Engine) downloadAgent() error {
	log.Info("Downloading Amazon Elastic Container Service Agent")
	err := e.downloader.DownloadAgent()
	if err != nil {
		return engineError("could not download Amazon Elastic Container Service Agent", err)
	}
	return nil
}

func (e *Engine) load(docker dockerClient, agentLoader func() (io.ReadCloser, error)) error {
	image, err := agentLoader()
	if err != nil {
		return engineError("could not load Amazon Elastic Container Service Agent from cache", err)
	}
	defer image.Close()
	err = docker.LoadImage(image)
	if err != nil {
		return engineError("could not load Amazon Elastic Container Service Agent into Docker", err)
	}
	return e.downloader.RecordCachedAgent()
}

// StartSupervised starts the ECS Agent and ensures it stays running, except for terminal errors (indicated by an agent exit code of 5)
func (e *Engine) StartSupervised() error {
	docker, err := getDockerClient()
	if err != nil {
		return dockerError(err)
	}
	agentExitCode := -1
	retryBackoff := backoff.NewBackoff(serviceStartMinRetryTime, serviceStartMaxRetryTime,
		serviceStartRetryJitter, serviceStartRetryMultiplier, serviceStartMaxRetries)
	for {
		err := docker.RemoveExistingAgentContainer()
		if err != nil {
			return engineError("could not remove existing Agent container", err)
		}

		// start the custom healthcheck server,
		// this will receive request from the ECS agent to run custom health check commands on the container instance
		go chc.StartCustomHealthCheckServer()

		log.Info("Starting Amazon Elastic Container Service Agent")
		agentExitCode, err = docker.StartAgent()
		if err != nil {
			return engineError("could not start Agent", err)
		}
		log.Infof("Agent exited with code %d", agentExitCode)

		switch agentExitCode {
		case upgradeAgentExitCode:
			err = e.upgradeAgent(docker)
			if err != nil {
				log.Error("could not upgrade agent", err)
			} else {
				// continuing here because a successful upgrade doesn't need to backoff retries
				continue
			}
		case containerFailureAgentExitCode:
			// capture the tail of the failed agent container
			log.Infof("Captured the last %s lines of the agent container logs====>\n", failedContainerLogWindowSize)
			log.Info(docker.GetContainerLogTail(failedContainerLogWindowSize))
			log.Infof("<====end %s lines of the failed agent container logs\n", failedContainerLogWindowSize)
		case TerminalFailureAgentExitCode:
			return &TerminalError{
				err:      "agent exited with terminal exit code",
				exitCode: TerminalFailureAgentExitCode,
			}
		case terminalSuccessAgentExitCode:
			return nil
		}
		d := retryBackoff.Duration()
		log.Warnf("ECS Agent failed to start, retrying in %s", d)
		time.Sleep(d)
	}
}

func (e *Engine) upgradeAgent(docker dockerClient) error {
	log.Info("Loading new desired Amazon Elastic Container Service Agent into Docker")
	return e.load(docker, e.downloader.LoadDesiredAgent)
}

// PreStop sends commands to Docker to stop the ECS Agent
func (e *Engine) PreStop() error {
	docker, err := getDockerClient()
	if err != nil {
		return dockerError(err)
	}
	log.Info("Stopping Amazon Elastic Container Service Agent")
	err = docker.StopAgent()
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
	// added in the first place
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
