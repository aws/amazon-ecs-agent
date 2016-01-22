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

package main

import (
	"flag"
	"fmt"
	mathrand "math/rand"
	"os"
	"runtime"
	"time"

	acshandler "github.com/aws/amazon-ecs-agent/agent/acs/handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/httpclient"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/tcs/handler"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	utilatomic "github.com/aws/amazon-ecs-agent/agent/utils/atomic"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/defaults"
	log "github.com/cihub/seelog"
	"golang.org/x/net/context"
)

func init() {
	runtime.GOMAXPROCS(1)
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

	log.Infof("Starting Agent: %v", version.String())
	if *acceptInsecureCert {
		log.Warn("SSL certificate verification disabled. This is not recommended.")
	}
	log.Info("Loading configuration")
	cfg, err := config.NewConfig(ec2MetadataClient)
	// Load cfg before doing 'versionFlag' so that it has the DOCKER_HOST
	// variable loaded if needed
	if *versionFlag {
		versionableEngine := engine.NewTaskEngine(cfg, *acceptInsecureCert)
		version.PrintVersion(versionableEngine)
		return exitcodes.ExitSuccess
	}

	sighandlers.StartDebugHandler()
	ctx := context.Background()

	if err != nil {
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
		previousTaskEngine := engine.NewTaskEngine(cfg, *acceptInsecureCert)
		// previousState is used to verify that our current runtime configuration is
		// compatible with our past configuration as reflected by our state-file
		previousState, err := initializeStateManager(cfg, previousTaskEngine, &previousCluster, &previousContainerInstanceArn, &previousEc2InstanceID, acshandler.SequenceNumber)
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
				configuredCluster = config.DEFAULT_CLUSTER_NAME
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
			log.Warnf("Data mismatch; saved InstanceID '%v' does not match current InstanceID '%v'. Overwriting old datafile", previousEc2InstanceID, currentEc2InstanceID)

			// Reset taskEngine; all the other values are still default
			taskEngine = engine.NewTaskEngine(cfg, *acceptInsecureCert)
		} else {
			// Use the values we loaded if there's no issue
			containerInstanceArn = previousContainerInstanceArn
			taskEngine = previousTaskEngine
		}
	} else {
		log.Info("Checkpointing not enabled; a new container instance will be created each time the agent is run")
		taskEngine = engine.NewTaskEngine(cfg, *acceptInsecureCert)
	}

	stateManager, err := initializeStateManager(cfg, taskEngine, &cfg.Cluster, &containerInstanceArn, &currentEc2InstanceID, acshandler.SequenceNumber)
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
		log.Warnf("Error getting valid credentials (AKID %v): %v", preflightCreds.AccessKeyID, err)
	}
	client := api.NewECSClient(credentialProvider, cfg, httpclient.New(api.RoundtripTimeout, *acceptInsecureCert), ec2MetadataClient)

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
		log.Infof("Registration completed successfully. I am running as '%v' in cluster '%v'", containerInstanceArn, cfg.Cluster)
		// Save our shiny new containerInstanceArn
		stateManager.Save()
	} else {
		log.Infof("Restored from checkpoint file. I am running as '%v' in cluster '%v'", containerInstanceArn, cfg.Cluster)
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
	taskEngine.MustInit()

	go sighandlers.StartTerminationHandler(stateManager, taskEngine)

	// Agent introspection api
	go handlers.ServeHttp(&containerInstanceArn, taskEngine, cfg)

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(taskEngine, client, stateManager)

	telemetrySessionParams := tcshandler.TelemetrySessionParams{
		ContainerInstanceArn: containerInstanceArn,
		CredentialProvider:   credentialProvider,
		Cfg:                  cfg,
		AcceptInvalidCert:    *acceptInsecureCert,
		EcsClient:            client,
		TaskEngine:           taskEngine,
	}

	// Start metrics session in a go routine
	go tcshandler.StartMetricsSession(telemetrySessionParams)

	log.Info("Beginning Polling for updates")
	err = acshandler.StartSession(ctx, acshandler.StartSessionArguments{
		AcceptInvalidCert:    *acceptInsecureCert,
		Config:               cfg,
		ContainerInstanceArn: containerInstanceArn,
		CredentialProvider:   credentialProvider,
		ECSClient:            client,
		StateManager:         stateManager,
		TaskEngine:           taskEngine,
	})
	if err != nil {
		log.Criticalf("Unretriable error starting communicating with ACS: %v", err)
		return exitcodes.ExitTerminal
	}
	log.Critical("ACS Session handler should never exit")
	return exitcodes.ExitError
}

func initializeStateManager(cfg *config.Config, taskEngine engine.TaskEngine, cluster, containerInstanceArn, savedInstanceID *string, sequenceNumber *utilatomic.IncreasingInt64) (statemanager.StateManager, error) {
	if !cfg.Checkpoint {
		return statemanager.NewNoopStateManager(), nil
	}
	stateManager, err := statemanager.NewStateManager(cfg,
		statemanager.AddSaveable("TaskEngine", taskEngine),
		statemanager.AddSaveable("ContainerInstanceArn", containerInstanceArn),
		statemanager.AddSaveable("Cluster", cluster),
		statemanager.AddSaveable("EC2InstanceID", savedInstanceID),
		statemanager.AddSaveable("ACSSeqNum", sequenceNumber),
	)
	if err != nil {
		return nil, err
	}
	return stateManager, nil
}
