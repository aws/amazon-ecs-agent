// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"context"
	"fmt"

	acshandler "github.com/aws/amazon-ecs-agent/agent/acs/handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/api/ecsclient"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/credentials"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/eventstream"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	credentialshandler "github.com/aws/amazon-ecs-agent/agent/handlers/credentials"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/tcs/handler"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws"
	aws_credentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/defaults"
	log "github.com/cihub/seelog"
)

const (
	containerChangeEventStreamName             = "ContainerChange"
	deregisterContainerInstanceEventStreamName = "DeregisterContainerInstance"
	clusterMismatchErrorFormat                 = "Data mismatch; saved cluster '%v' does not match configured cluster '%v'. Perhaps you want to delete the configured checkpoint file?"
	instanceIDMismatchErrorFormat              = "Data mismatch; saved InstanceID '%s' does not match current InstanceID '%s'. Overwriting old datafile"
	instanceTypeMismatchErrorFormat            = "The current instance type does not match the registered instance type. Please revert the instance type change, or alternatively launch a new instance: %v"
)

// agent interface is used by the app runner to interact with the ecsAgent
// object. Its purpose is to mostly demonstrate how to interact with the
// ecsAgent type.
type agent interface {
	// printVersion prints the Agent version string
	printVersion() int
	// start starts the Agent execution
	start() int
}

// ecsAgent wraps all the entities needed to start the ECS Agent execution.
// after creating it via
// the newAgent() method
type ecsAgent struct {
	ctx                  context.Context
	ec2MetadataClient    ec2.EC2MetadataClient
	cfg                  *config.Config
	dockerClient         engine.DockerClient
	containerInstanceArn string
	credentialProvider   *aws_credentials.Credentials
}

// newAgent returns a new ecsAgent object
func newAgent(
	ctx context.Context,
	blackholeEC2Metadata bool,
	acceptInsecureCert *bool) (agent, error) {

	ec2MetadataClient := ec2.NewEC2MetadataClient(nil)
	if blackholeEC2Metadata {
		ec2MetadataClient = ec2.NewBlackholeEC2MetadataClient()
	}

	log.Info("Loading configuration")
	cfg, err := config.NewConfig(ec2MetadataClient)
	if err != nil {
		// All required config values can be inferred from EC2 Metadata,
		// so this error could be transient.
		log.Criticalf("Error loading config: %v", err)
		return nil, err
	}
	cfg.AcceptInsecureCert = aws.BoolValue(acceptInsecureCert)
	if cfg.AcceptInsecureCert {
		log.Warn("SSL certificate verification disabled. This is not recommended.")
	}
	log.Debugf("Loaded config: %s", cfg.String())

	dockerClient, err := engine.NewDockerGoClient(dockerclient.NewFactory(cfg.DockerEndpoint), cfg)
	if err != nil {
		// This is also non terminal in the current config
		log.Criticalf("Error creating Docker client: %v", err)
		return nil, err
	}

	return &ecsAgent{
		ctx:               ctx,
		ec2MetadataClient: ec2MetadataClient,
		cfg:               cfg,
		dockerClient:      dockerClient,
		// We instantiate our own credentialProvider for use in acs/tcs. This tries
		// to mimic roughly the way it's instantiated by the SDK for a default
		// session.
		credentialProvider: defaults.CredChain(defaults.Config(), defaults.Handlers()),
	}, nil

}

func (agent *ecsAgent) printVersion() int {
	version.PrintVersion(agent.dockerClient)
	return exitcodes.ExitSuccess
}

func (agent *ecsAgent) start() int {
	sighandlers.StartDebugHandler()

	containerChangeEventStream := eventstream.NewEventStream(containerChangeEventStreamName, agent.ctx)
	credentialsManager := credentials.NewManager()
	state := dockerstate.NewTaskEngineState()
	imageManager := engine.NewImageManager(agent.cfg, agent.dockerClient, state)

	return agent.doStart(containerChangeEventStream, credentialsManager, state, imageManager)
}

func (agent *ecsAgent) doStart(containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	imageManager engine.ImageManager) int {

	taskEngine, currentEC2InstanceID, err := agent.newTaskEngine(containerChangeEventStream,
		credentialsManager, state, imageManager)
	if err != nil {
		return exitcodes.ExitTerminal
	}

	stateManager, err := agent.newStateManager(taskEngine,
		&agent.cfg.Cluster, &agent.containerInstanceArn, &currentEC2InstanceID)
	if err != nil {
		log.Criticalf("Error creating state manager: %v", err)
		return exitcodes.ExitTerminal
	}

	client := ecsclient.NewECSClient(agent.credentialProvider, agent.cfg, agent.ec2MetadataClient)
	err = agent.registerContainerInstance(taskEngine, stateManager, client)
	if err != nil {
		if isNonTerminal(err) {
			return exitcodes.ExitError
		}
		return exitcodes.ExitTerminal
	}

	// Begin listening to the docker daemon and saving changes
	taskEngine.SetSaver(stateManager)
	imageManager.SetSaver(stateManager)
	taskEngine.MustInit()

	deregisterInstanceEventStream := eventstream.NewEventStream(
		deregisterContainerInstanceEventStreamName, agent.ctx)
	deregisterInstanceEventStream.StartListening()

	taskHandler := eventhandler.NewTaskHandler()

	agent.startAsyncRoutines(containerChangeEventStream, credentialsManager, imageManager,
		taskEngine, stateManager, deregisterInstanceEventStream, client, taskHandler)
	return agent.startACSSession(credentialsManager, taskEngine, stateManager,
		deregisterInstanceEventStream, client, taskHandler)
}

func (agent *ecsAgent) newTaskEngine(containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	state dockerstate.TaskEngineState,
	imageManager engine.ImageManager) (engine.TaskEngine, string, error) {

	containerChangeEventStream.StartListening()

	if !agent.cfg.Checkpoint {
		log.Info("Checkpointing not enabled; a new container instance will be created each time the agent is run")
		return engine.NewTaskEngine(agent.cfg, agent.dockerClient, credentialsManager,
			containerChangeEventStream, imageManager, state), "", nil
	}

	// We try to set these values by loading the existing state file first
	var previousCluster, previousEC2InstanceID, previousContainerInstanceArn string
	previousTaskEngine := engine.NewTaskEngine(agent.cfg, agent.dockerClient, credentialsManager,
		containerChangeEventStream, imageManager, state)

	// previousState is used to verify that our current runtime configuration is
	// compatible with our past configuration as reflected by our state-file
	previousState, err := agent.newStateManager(previousTaskEngine, &previousCluster,
		&previousContainerInstanceArn, &previousEC2InstanceID)
	if err != nil {
		log.Criticalf("Error creating state manager: %v", err)
		return nil, "", err
	}

	err = previousState.Load()
	if err != nil {
		log.Criticalf("Error loading previously saved state: %v", err)
		return nil, "", err
	}

	if previousCluster != "" {
		if err := agent.setClusterInConfig(previousCluster); err != nil {
			return nil, "", err
		}
	}

	currentEC2InstanceID := agent.getEC2InstanceID()
	if previousEC2InstanceID != "" && previousEC2InstanceID != currentEC2InstanceID {
		log.Warnf(instanceIDMismatchErrorFormat,
			previousEC2InstanceID, currentEC2InstanceID)

		// Reset taskEngine; all the other values are still default
		return engine.NewTaskEngine(agent.cfg, agent.dockerClient, credentialsManager,
			containerChangeEventStream, imageManager, state), currentEC2InstanceID, nil
	}
	// Use the values we loaded if there's no issue
	agent.containerInstanceArn = previousContainerInstanceArn
	return previousTaskEngine, currentEC2InstanceID, nil
}

func (agent *ecsAgent) setClusterInConfig(previousCluster string) error {
	// TODO Handle default cluster in a sane and unified way across the codebase
	configuredCluster := agent.cfg.Cluster
	if configuredCluster == "" {
		log.Debug("Setting cluster to default; none configured")
		configuredCluster = config.DefaultClusterName
	}
	if previousCluster != configuredCluster {
		clusterMismatchError := fmt.Errorf(clusterMismatchErrorFormat,
			previousCluster, configuredCluster)
		log.Criticalf("%v", clusterMismatchError)
		return clusterMismatchError
	}
	agent.cfg.Cluster = previousCluster
	log.Infof("Restored cluster '%s'", agent.cfg.Cluster)

	return nil
}

func (agent *ecsAgent) getEC2InstanceID() string {
	instanceIdentityDoc, err := agent.ec2MetadataClient.InstanceIdentityDocument()
	if err != nil {
		log.Criticalf("Unable to access EC2 Metadata service to determine EC2 ID: %v", err)
		return ""
	}
	return instanceIdentityDoc.InstanceId
}

// expect all of these to be backfilled on Load
func (agent *ecsAgent) newStateManager(
	taskEngine engine.TaskEngine,
	cluster *string,
	containerInstanceArn *string,
	savedInstanceID *string) (statemanager.StateManager, error) {

	if !agent.cfg.Checkpoint {
		return statemanager.NewNoopStateManager(), nil
	}
	return statemanager.NewStateManager(agent.cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", containerInstanceArn),
		statemanager.AddSaveable("Cluster", cluster),
		statemanager.AddSaveable("EC2InstanceID", savedInstanceID),
		// The ACSSeqNum field is retained for compatibility with statemanager.EcsDataVersion 4 and
		// can be removed in the future with a version bump.
		statemanager.AddSaveable("ACSSeqNum", 1),
	)
}

func (agent *ecsAgent) registerContainerInstance(
	taskEngine engine.TaskEngine,
	stateManager statemanager.StateManager,
	client api.ECSClient) error {
	// Preflight request to make sure they're good
	if preflightCreds, err := agent.credentialProvider.Get(); err != nil || preflightCreds.AccessKeyID == "" {
		log.Warnf("Error getting valid credentials (AKID %s): %v", preflightCreds.AccessKeyID, err)
	}
	capabilities := taskEngine.Capabilities()

	if agent.containerInstanceArn != "" {
		log.Infof("Restored from checkpoint file. I am running as '%s' in cluster '%s'", agent.containerInstanceArn, agent.cfg.Cluster)
		return agent.reregisterContainerInstance(client, capabilities)
	}

	log.Info("Registering Instance with ECS")
	containerInstanceArn, err := client.RegisterContainerInstance("", capabilities)
	if err != nil {
		log.Errorf("Error registering: %v", err)
		if retriable, ok := err.(utils.Retriable); ok && !retriable.Retry() {
			return err
		}
		if _, ok := err.(utils.AttributeError); ok {
			log.Criticalf("Instance registration attempt with an invalid attribute")
			return err
		}
		return nonTerminalError{err}
	}
	log.Infof("Registration completed successfully. I am running as '%s' in cluster '%s'", containerInstanceArn, agent.cfg.Cluster)
	agent.containerInstanceArn = containerInstanceArn
	// Save our shiny new containerInstanceArn
	stateManager.Save()
	return nil

}

func (agent *ecsAgent) reregisterContainerInstance(client api.ECSClient, capabilities []string) error {
	_, err := client.RegisterContainerInstance(agent.containerInstanceArn, capabilities)
	if err == nil {
		return nil
	}
	log.Errorf("Error re-registering: %v", err)
	if api.IsInstanceTypeChangedError(err) {
		log.Criticalf(instanceTypeMismatchErrorFormat, err)
		return err
	}
	if _, ok := err.(utils.AttributeError); ok {
		log.Criticalf("Instance re-registration attempt with an invalid attribute")
		return err
	}
	return nonTerminalError{err}
}

func (agent *ecsAgent) startAsyncRoutines(
	containerChangeEventStream *eventstream.EventStream,
	credentialsManager credentials.Manager,
	imageManager engine.ImageManager,
	taskEngine engine.TaskEngine,
	stateManager statemanager.StateManager,
	deregisterInstanceEventStream *eventstream.EventStream,
	client api.ECSClient,
	taskHandler *eventhandler.TaskHandler) {

	// Start of the periodic image cleanup process
	if !agent.cfg.ImageCleanupDisabled {
		go imageManager.StartImageCleanupProcess(agent.ctx)
	}

	go sighandlers.StartTerminationHandler(stateManager, taskEngine)

	// Agent introspection api
	go handlers.ServeHttp(&agent.containerInstanceArn, taskEngine, agent.cfg)

	// Start serving the endpoint to fetch IAM Role credentials
	go credentialshandler.ServeHTTP(credentialsManager, agent.containerInstanceArn, agent.cfg)

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(taskEngine, client, stateManager, taskHandler)

	telemetrySessionParams := tcshandler.TelemetrySessionParams{
		CredentialProvider:            agent.credentialProvider,
		Cfg:                           agent.cfg,
		ContainerInstanceArn:          agent.containerInstanceArn,
		DeregisterInstanceEventStream: deregisterInstanceEventStream,
		ContainerChangeEventStream:    containerChangeEventStream,
		DockerClient:                  agent.dockerClient,
		ECSClient:                     client,
		TaskEngine:                    taskEngine,
	}

	// Start metrics session in a go routine
	go tcshandler.StartMetricsSession(telemetrySessionParams)
}

func (agent *ecsAgent) startACSSession(
	credentialsManager credentials.Manager,
	taskEngine engine.TaskEngine,
	stateManager statemanager.StateManager,
	deregisterInstanceEventStream *eventstream.EventStream,
	client api.ECSClient,
	taskHandler *eventhandler.TaskHandler) int {

	acsSession := acshandler.NewSession(
		agent.ctx,
		agent.cfg,
		deregisterInstanceEventStream,
		agent.containerInstanceArn,
		agent.credentialProvider,
		client,
		stateManager,
		taskEngine,
		credentialsManager,
		taskHandler,
	)
	log.Info("Beginning Polling for updates")
	err := acsSession.Start()
	if err != nil {
		log.Criticalf("Unretriable error starting communicating with ACS: %v", err)
		return exitcodes.ExitTerminal
	}
	log.Critical("ACS Session handler should never exit")
	return exitcodes.ExitError
}
