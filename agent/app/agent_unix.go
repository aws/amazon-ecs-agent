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

package app

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"

	asmfactory "github.com/aws/amazon-ecs-agent/agent/asm/factory"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerapi"
	ebs "github.com/aws/amazon-ecs-agent/agent/ebs"
	"github.com/aws/amazon-ecs-agent/agent/ecscni"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eni/watcher"
	"github.com/aws/amazon-ecs-agent/agent/gpu"
	s3factory "github.com/aws/amazon-ecs-agent/agent/s3/factory"
	ssmfactory "github.com/aws/amazon-ecs-agent/agent/ssm/factory"
	"github.com/aws/amazon-ecs-agent/ecs-agent/api/ecs/model/ecs"
	"github.com/aws/amazon-ecs-agent/ecs-agent/credentials"

	"github.com/aws/amazon-ecs-agent/agent/statechange"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	cgroup "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup/control"
	"github.com/aws/amazon-ecs-agent/agent/utils/ioutilwrapper"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"
)

// initPID defines the process identifier for the init process
const initPID = 1

// awsVPCCNIPlugins is a list of CNI plugins required by the ECS Agent
// to configure the ENI for a task
var awsVPCCNIPlugins = []string{
	ecscni.VPCENIPluginName,
	ecscni.ECSBridgePluginName,
	ecscni.ECSIPAMPluginName,
	ecscni.ECSAppMeshPluginName,
	ecscni.ECSBranchENIPluginName,
}

// startWindowsService is not supported on Linux
func (agent *ecsAgent) startWindowsService() int {
	seelog.Error("Windows Services are not supported on Linux")
	return 1
}

var getPid = os.Getpid

// initializeTaskENIDependencies initializes all of the dependencies required by
// the Agent to support the 'awsvpc' networking mode. A non nil error is returned
// if an error is encountered during this process. An additional boolean flag to
// indicate if this error is considered terminal is also returned
func (agent *ecsAgent) initializeTaskENIDependencies(state dockerstate.TaskEngineState, taskEngine engine.TaskEngine) (error, bool) {
	// Check if the Agent process's pid  == 1, which means it's running without an init system
	if getPid() == initPID {
		// This is a terminal error. Bad things happen with invoking the
		// the ENI plugin when there's no init process in the pid namespace.
		// Specifically, the DHClient processes that are started as children
		// of the Agent will not be reaped leading to the ENI device
		// disappearing until the Agent is killed.
		return errors.New("agent is not started with an init system"), true
	}

	// Set VPC and Subnet IDs for the instance
	if err, ok := agent.setVPCSubnet(); err != nil {
		return err, ok
	}

	// Validate that the CNI plugins exist in the expected path and that
	// they possess the right capabilities
	if err := agent.verifyCNIPluginsCapabilities(); err != nil {
		// An error here is terminal as it means that the plugins
		// do not support the ENI capability
		return err, true
	}

	if err := agent.startENIWatcher(state, taskEngine.StateChangeEvents()); err != nil {
		// If udev watcher was not initialized in this run because of the udev socket
		// file not being available etc, the Agent might be able to retry and succeed
		// on the next run. Hence, returning a false here for terminal bool
		return err, false
	}

	return nil, false
}

// verifyCNIPluginsCapabilities returns an error if there's an error querying
// capabilities or if the required capability is absent from the capabilities
// of the following plugins:
// a. vpc-eni
// b. ecs-bridge
// c. ecs-ipam
// d. aws-appmesh
// e. vpc-branch-eni
func (agent *ecsAgent) verifyCNIPluginsCapabilities() error {
	// Check if we can get capabilities from each plugin
	for _, plugin := range awsVPCCNIPlugins {
		// skip verifying branch cni plugin if eni trunking is not enabled
		if plugin == ecscni.ECSBranchENIPluginName && agent.cfg != nil && !agent.cfg.ENITrunkingEnabled.Enabled() {
			continue
		}

		capabilities, err := agent.cniClient.Capabilities(plugin)
		if err != nil {
			return err
		}
		// appmesh plugin is not needed for awsvpc networking capability
		if plugin == ecscni.ECSAppMeshPluginName {
			continue
		}
		if !contains(capabilities, ecscni.CapabilityAWSVPCNetworkingMode) {
			return errors.Errorf("plugin '%s' doesn't support the capability: %s",
				plugin, ecscni.CapabilityAWSVPCNetworkingMode)
		}
	}

	return nil
}

// startENIWatcher starts the udev monitor and the watcher for receiving
// notifications from the monitor
func (agent *ecsAgent) startENIWatcher(state dockerstate.TaskEngineState, stateChangeEvents chan<- statechange.Event) error {
	seelog.Debug("Setting up ENI Watcher")
	if agent.eniWatcher == nil {
		eniWatcher, err := watcher.New(agent.ctx, agent.mac, state, stateChangeEvents)
		if err != nil {
			return errors.Wrapf(err, "unable to create ENI watcher")
		}
		agent.eniWatcher = eniWatcher

		if err := agent.eniWatcher.Init(); err != nil {
			return errors.Wrapf(err, "unable to initialize eni watcher")
		}
		go agent.eniWatcher.Start()
	}
	return nil
}

func (agent *ecsAgent) startEBSWatcher(
	state dockerstate.TaskEngineState,
	taskEngine engine.TaskEngine,
	dockerClient dockerapi.DockerClient,
) {
	if agent.ebsWatcher == nil {
		seelog.Debug("Creating new EBS watcher...")
		agent.ebsWatcher = ebs.NewWatcher(agent.ctx, state, taskEngine, dockerClient)
		go agent.ebsWatcher.Start()
	}
}

// initializeResourceFields exists mainly for testing doStart() to use mock Control
// object
func (agent *ecsAgent) initializeResourceFields(credentialsManager credentials.Manager) {
	agent.resourceFields = &taskresource.ResourceFields{
		Control: cgroup.New(),
		ResourceFieldsCommon: &taskresource.ResourceFieldsCommon{
			IOUtil:             ioutilwrapper.NewIOUtil(),
			ASMClientCreator:   asmfactory.NewClientCreator(),
			SSMClientCreator:   ssmfactory.NewSSMClientCreator(),
			S3ClientCreator:    s3factory.NewS3ClientCreator(),
			CredentialsManager: credentialsManager,
			EC2InstanceID:      agent.getEC2InstanceID(),
		},
		Ctx:              agent.ctx,
		DockerClient:     agent.dockerClient,
		NvidiaGPUManager: gpu.NewNvidiaGPUManager(),
	}
}

func (agent *ecsAgent) cgroupInit() error {
	err := agent.resourceFields.Control.Init()
	// When task CPU and memory limits are enabled, all tasks are placed
	// under the '/ecs' cgroup root.
	if err == nil {
		return nil
	}
	if agent.cfg.TaskCPUMemLimit.Value == config.ExplicitlyEnabled {
		return errors.Wrapf(err, "unable to setup '/ecs' cgroup")
	}
	seelog.Warnf("Disabling TaskCPUMemLimit because agent is unable to setup '/ecs' cgroup: %v", err)
	agent.cfg.TaskCPUMemLimit.Value = config.ExplicitlyDisabled
	return nil
}

func (agent *ecsAgent) initializeGPUManager() error {
	if agent.resourceFields != nil && agent.resourceFields.NvidiaGPUManager != nil {
		return agent.resourceFields.NvidiaGPUManager.Initialize()
	}
	return nil
}

func (agent *ecsAgent) getPlatformDevices() []*ecs.PlatformDevice {
	if agent.cfg.GPUSupportEnabled {
		if agent.resourceFields != nil && agent.resourceFields.NvidiaGPUManager != nil {
			return agent.resourceFields.NvidiaGPUManager.GetDevices()
		}
	}
	return nil
}

type Object struct {
	ID   string `json:"ID"`
	Type string `json:"Type"`
}

func (agent *ecsAgent) getDevicePlugins() []*ecs.PlatformDevice {
	var out []*ecs.PlatformDevice
	// Define a struct that matches the JSON structure
	var files []string

	// Walk through the directory to find all json files
	err := filepath.Walk("/var/lib/ecs/device-plugins/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			seelog.Error(err)
			return err
		}
		// Check if the file is not a directory and ends with ".json"
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			seelog.Info("Found json file: " + path)
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		seelog.Error(err)
		return nil
	}

	for _, file := range files {
		// Read the file
		file, err := ioutil.ReadFile(file)
		if err != nil {
			seelog.Error(err)
			return nil
		}

		// Parse the JSON into a slice of `Object` structs
		var objects []Object
		err = json.Unmarshal(file, &objects)
		if err != nil {
			seelog.Error(err)
			return nil
		}

		for _, obj := range objects {
			out = append(out, &ecs.PlatformDevice{
				Type: aws.String(obj.Type),
				Id:   aws.String(obj.ID),
			})
		}
	}

	return out
}

func (agent *ecsAgent) loadPauseContainer() error {
	// Load the pause container's image from the 'disk'
	_, err := agent.pauseLoader.LoadImage(agent.ctx, agent.cfg, agent.dockerClient)

	return err
}
