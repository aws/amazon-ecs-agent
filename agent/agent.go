// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package main

import (
	"flag"
	"fmt"
	mathrand "math/rand"
	"os"
	"time"

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
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/tcs/handler"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/defaults"
	log "github.com/cihub/seelog"
	"golang.org/x/net/context"
)

const (
	ContainerChangeEventStream             = "ContainerChange"
	DeregisterContainerInstanceEventStream = "DeregisterContainerInstance"
)

func init() {
	mathrand.Seed(time.Now().UnixNano())
}

func main() {
	// Use a returning main instead of os.Exiting main to allow defers to run
	// before exit
	os.Exit(_main())
}
func _main() int {
	defer log.Flush()
	flagset := flag.NewFlagSet("Amazon ECS Agent", flag.ContinueOnError)
	versionFlag := flagset.Bool("version", false, "Print the agent version information and exit")
	logLevel := flagset.String("loglevel", "", "Loglevel: [<crit>|<error>|<warn>|<info>|<debug>]")
	acceptInsecureCert := flagset.Bool("k", false, "Disable SSL certificate verification. We do not recommend setting this option.")
	licenseFlag := flagset.Bool("license", false, "Print the LICENSE and NOTICE files and exit")
	blackholeEc2Metadata := flagset.Bool("blackhole-ec2-metadata", false, "Blackhole the EC2 Metadata requests. Setting this option can cause the ECS Agent to fail to work properly.  We do not recommend setting this option")
	err := flagset.Parse(os.Args[1:])
	if err != nil {
		return exitcodes.ExitTerminal
	}

	if *licenseFlag {
		license := utils.NewLicenseProvider()
		text, err := license.GetText()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitcodes.ExitError
		}
		fmt.Println(text)
		return exitcodes.ExitSuccess
	}

	logger.SetLevel(*logLevel)
	ec2MetadataClient := ec2.DefaultClient
	if *blackholeEc2Metadata {
		ec2MetadataClient = ec2.NewBlackholeEC2MetadataClient()
	}

	log.Infof("Starting Agent: %s", version.String())
	if *acceptInsecureCert {
		log.Warn("SSL certificate verification disabled. This is not recommended.")
	}
	log.Info("Loading configuration")
	cfg, cfgErr := config.NewConfig(ec2MetadataClient)
	// Load cfg and create Docker client before doing 'versionFlag' so that it has the DOCKER_HOST variable loaded if needed
	clientFactory := dockerclient.NewFactory(cfg.DockerEndpoint)
	dockerClient, err := engine.NewDockerGoClient(clientFactory, *acceptInsecureCert, cfg)
	if err != nil {
		log.Criticalf("Error creating Docker client: %v", err)
		return exitcodes.ExitError
	}

	ctx := context.Background()
	// Create the DockerContainerChange event stream for tcs
	containerChangeEventStream := eventstream.NewEventStream(ContainerChangeEventStream, ctx)
	containerChangeEventStream.StartListening()

	// Create credentials manager. This will be used by the task engine and
	// the credentials handler
	credentialsManager := credentials.NewManager()
	// Create image manager. This will be used by the task engine for saving image states
	state := dockerstate.NewDockerTaskEngineState()
	imageManager := engine.NewImageManager(cfg, dockerClient, state)
	if *versionFlag {
		versionableEngine := engine.NewTaskEngine(cfg, dockerClient, credentialsManager, containerChangeEventStream, imageManager, state)
		version.PrintVersion(versionableEngine)
		return exitcodes.ExitSuccess
	}

	sighandlers.StartDebugHandler()

	if cfgErr != nil {
		log.Criticalf("Error loading config: %v", err)
		// All required config values can be inferred from EC2 Metadata, so this error could be transient.
		return exitcodes.ExitError
	}
	log.Debug("Loaded config: " + cfg.String())

	var currentEc2InstanceID, containerInstanceArn string
	var taskEngine engine.TaskEngine

	if cfg.Checkpoint {
		log.Info("Checkpointing is enabled. Attempting to load state")
		var previousCluster, previousEc2InstanceID, previousContainerInstanceArn string
		previousTaskEngine := engine.NewTaskEngine(cfg, dockerClient, credentialsManager, containerChangeEventStream, imageManager, state)
		// previousState is used to verify that our current runtime configuration is
		// compatible with our past configuration as reflected by our state-file
		previousState, err := initializeStateManager(cfg, previousTaskEngine, &previousCluster, &previousContainerInstanceArn, &previousEc2InstanceID)
		if err != nil {
			log.Criticalf("Error creating state manager: %v", err)
			return exitcodes.ExitTerminal
		}

		err = previousState.Load()
		if err != nil {
			log.Criticalf("Error loading previously saved state: %v", err)
			return exitcodes.ExitTerminal
		}

		if previousCluster != "" {
			// TODO Handle default cluster in a sane and unified way across the codebase
			configuredCluster := cfg.Cluster
			if configuredCluster == "" {
				log.Debug("Setting cluster to default; none configured")
				configuredCluster = config.DefaultClusterName
			}
			if previousCluster != configuredCluster {
				log.Criticalf("Data mismatch; saved cluster '%v' does not match configured cluster '%v'. Perhaps you want to delete the configured checkpoint file?", previousCluster, configuredCluster)
				return exitcodes.ExitTerminal
			}
			cfg.Cluster = previousCluster
			log.Infof("Restored cluster '%v'", cfg.Cluster)
		}

		if instanceIdentityDoc, err := ec2MetadataClient.InstanceIdentityDocument(); err == nil {
			currentEc2InstanceID = instanceIdentityDoc.InstanceId
		} else {
			log.Criticalf("Unable to access EC2 Metadata service to determine EC2 ID: %v", err)
		}

		if previousEc2InstanceID != "" && previousEc2InstanceID != currentEc2InstanceID {
			log.Warnf("Data mismatch; saved InstanceID '%s' does not match current InstanceID '%s'. Overwriting old datafile", previousEc2InstanceID, currentEc2InstanceID)

			// Reset taskEngine; all the other values are still default
			taskEngine = engine.NewTaskEngine(cfg, dockerClient, credentialsManager, containerChangeEventStream, imageManager, state)
		} else {
			// Use the values we loaded if there's no issue
			containerInstanceArn = previousContainerInstanceArn
			taskEngine = previousTaskEngine
		}
	} else {
		log.Info("Checkpointing not enabled; a new container instance will be created each time the agent is run")
		taskEngine = engine.NewTaskEngine(cfg, dockerClient, credentialsManager, containerChangeEventStream, imageManager, state)
	}

	stateManager, err := initializeStateManager(cfg, taskEngine, &cfg.Cluster, &containerInstanceArn, &currentEc2InstanceID)
	if err != nil {
		log.Criticalf("Error creating state manager: %v", err)
		return exitcodes.ExitTerminal
	}

	capabilities := taskEngine.Capabilities()

	// We instantiate our own credentialProvider for use in acs/tcs. This tries
	// to mimic roughly the way it's instantiated by the SDK for a default
	// session.
	credentialProvider := defaults.CredChain(defaults.Config(), defaults.Handlers())
	// Preflight request to make sure they're good
	if preflightCreds, err := credentialProvider.Get(); err != nil || preflightCreds.AccessKeyID == "" {
		log.Warnf("Error getting valid credentials (AKID %s): %v", preflightCreds.AccessKeyID, err)
	}
	client := ecsclient.NewECSClient(credentialProvider, cfg,
		httpclient.New(ecsclient.RoundtripTimeout, *acceptInsecureCert), ec2MetadataClient)

	if containerInstanceArn == "" {
		log.Info("Registering Instance with ECS")
		containerInstanceArn, err = client.RegisterContainerInstance("", capabilities)
		if err != nil {
			log.Errorf("Error registering: %v", err)
			if retriable, ok := err.(utils.Retriable); ok && !retriable.Retry() {
				return exitcodes.ExitTerminal
			}
			return exitcodes.ExitError
		}
		log.Infof("Registration completed successfully. I am running as '%s' in cluster '%s'", containerInstanceArn, cfg.Cluster)
		// Save our shiny new containerInstanceArn
		stateManager.Save()
	} else {
		log.Infof("Restored from checkpoint file. I am running as '%s' in cluster '%s'", containerInstanceArn, cfg.Cluster)
		_, err = client.RegisterContainerInstance(containerInstanceArn, capabilities)
		if err != nil {
			log.Errorf("Error re-registering: %v", err)
			if awserr, ok := err.(awserr.Error); ok && api.IsInstanceTypeChangedError(awserr) {
				log.Criticalf("The current instance type does not match the registered instance type. Please revert the instance type change, or alternatively launch a new instance. Error: %v", err)
				return exitcodes.ExitTerminal
			}
			return exitcodes.ExitError
		}
	}

	// Begin listening to the docker daemon and saving changes
	taskEngine.SetSaver(stateManager)
	imageManager.SetSaver(stateManager)
	taskEngine.MustInit()

	// start of the periodic image cleanup process
	if !cfg.ImageCleanupDisabled {
		go imageManager.StartImageCleanupProcess(ctx)
	}

	go sighandlers.StartTerminationHandler(stateManager, taskEngine)

	// Agent introspection api
	go handlers.ServeHttp(&containerInstanceArn, taskEngine, cfg)

	// Start serving the endpoint to fetch IAM Role credentials
	go credentialshandler.ServeHTTP(credentialsManager, containerInstanceArn, cfg)

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(taskEngine, client, stateManager)

	deregisterInstanceEventStream := eventstream.NewEventStream(DeregisterContainerInstanceEventStream, ctx)
	deregisterInstanceEventStream.StartListening()

	telemetrySessionParams := tcshandler.TelemetrySessionParams{
		ContainerInstanceArn: containerInstanceArn,
		CredentialProvider:   credentialProvider,
		Cfg:                  cfg,
		DeregisterInstanceEventStream: deregisterInstanceEventStream,
		ContainerChangeEventStream:    containerChangeEventStream,
		DockerClient:                  dockerClient,
		AcceptInvalidCert:             *acceptInsecureCert,
		ECSClient:                     client,
		TaskEngine:                    taskEngine,
	}

	// Start metrics session in a go routine
	go tcshandler.StartMetricsSession(telemetrySessionParams)

	log.Info("Beginning Polling for updates")
	err = acshandler.StartSession(ctx, acshandler.StartSessionArguments{
		AcceptInvalidCert: *acceptInsecureCert,
		Config:            cfg,
		DeregisterInstanceEventStream: deregisterInstanceEventStream,
		ContainerInstanceArn:          containerInstanceArn,
		CredentialProvider:            credentialProvider,
		ECSClient:                     client,
		StateManager:                  stateManager,
		TaskEngine:                    taskEngine,
		CredentialsManager:            credentialsManager,
	})
	if err != nil {
		log.Criticalf("Unretriable error starting communicating with ACS: %v", err)
		return exitcodes.ExitTerminal
	}
	log.Critical("ACS Session handler should never exit")
	return exitcodes.ExitError
}

func initializeStateManager(cfg *config.Config, taskEngine engine.TaskEngine, cluster, containerInstanceArn, savedInstanceID *string) (statemanager.StateManager, error) {
	if !cfg.Checkpoint {
		return statemanager.NewNoopStateManager(), nil
	}
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", containerInstanceArn),
		statemanager.AddSaveable("Cluster", cluster),
		statemanager.AddSaveable("EC2InstanceID", savedInstanceID),
		//The ACSSeqNum field is retained for compatibility with statemanager.EcsDataVersion 4 and
		//can be removed in the future with a version bump.
		statemanager.AddSaveable("ACSSeqNum", 1),
	)
	if err != nil {
		return nil, err
	}
	return stateManager, nil
}
