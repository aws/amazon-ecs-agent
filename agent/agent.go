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
	mathrand "math/rand"
	"os"
	"time"

	acshandler "github.com/aws/amazon-ecs-agent/agent/acs/handler"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/auth"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	utilatomic "github.com/aws/amazon-ecs-agent/agent/utils/atomic"
	"github.com/aws/amazon-ecs-agent/agent/version"
	"github.com/cihub/seelog"
)

func init() {
	mathrand.Seed(time.Now().UnixNano())
}

func main() {
	defer seelog.Flush()
	versionFlag := flag.Bool("version", false, "Print the agent version information and exit")
	acceptInsecureCert := flag.Bool("k", false, "Do not verify ssl certs")
	logLevel := flag.String("loglevel", "", "Loglevel: [<crit>|<error>|<warn>|<info>|<debug>]")
	flag.Parse()

	logger.SetLevel(*logLevel)
	log := logger.ForModule("main")

	log.Info("Starting Agent")

	log.Info("Loading configuration")
	cfg, err := config.NewConfig()
	if err != nil {
		log.Error("Error loading config", "err", err)
		os.Exit(exitcodes.ExitTerminal)
	}

	if *versionFlag {
		versionableEngine := engine.NewTaskEngine(cfg)
		version.PrintVersion(versionableEngine)
		os.Exit(exitcodes.ExitSuccess)
	}

	var currentEc2InstanceID, containerInstanceArn string
	var taskEngine engine.TaskEngine

	if cfg.Checkpoint {
		var previousCluster, previousEc2InstanceID, previousContainerInstanceArn string
		previousTaskEngine := engine.NewTaskEngine(cfg)
		// previousState is used to verify that our current runtime configuration is
		// compatible with our past configuration as reflected by our state-file
		previousState, err := initializeStateManager(cfg, previousTaskEngine, &previousCluster, &previousContainerInstanceArn, &previousEc2InstanceID, acshandler.SequenceNumber)
		if err != nil {
			log.Crit("Error creating state manager", "err", err)
			os.Exit(exitcodes.ExitTerminal)
		}

		err = previousState.Load()
		if err != nil {
			log.Crit("Error loading previously saved state", "err", err)
			os.Exit(exitcodes.ExitTerminal)
		}

		if previousCluster != "" {
			// TODO Handle default cluster in a sane and unified way across the codebase
			configuredCluster := cfg.Cluster
			if configuredCluster == "" {
				configuredCluster = config.DEFAULT_CLUSTER_NAME
			}
			if previousCluster != configuredCluster {
				log.Crit("Data mismatch; saved cluster does not match configured cluster. Perhaps you want to delete the configured checkpoint file?", "saved", previousCluster, "configured", configuredCluster)
				os.Exit(exitcodes.ExitTerminal)
			}
			cfg.Cluster = previousCluster
			log.Info("Restored cluster", "cluster", cfg.Cluster)
		}

		if instanceIdentityDoc, err := ec2.GetInstanceIdentityDocument(); err == nil {
			currentEc2InstanceID = instanceIdentityDoc.InstanceId
		} else {
			log.Crit("Unable to access EC2 Metadata service to determine EC2 ID", "err", err)
		}

		if previousEc2InstanceID != "" && previousEc2InstanceID != currentEc2InstanceID {
			log.Crit("Data mismatch; saved InstanceID does not match current InstanceID. Overwriting old datafile", "current", currentEc2InstanceID, "saved", previousEc2InstanceID)

			// Reset taskEngine; all the other values are still default
			taskEngine = engine.NewTaskEngine(cfg)
		} else {
			// Use the values we loaded if there's no issue
			containerInstanceArn = previousContainerInstanceArn
			taskEngine = previousTaskEngine
		}
	} else {
		log.Info("Checkpointing disabled")
		taskEngine = engine.NewTaskEngine(cfg)
	}

	stateManager, err := initializeStateManager(cfg, taskEngine, &cfg.Cluster, &containerInstanceArn, &currentEc2InstanceID, acshandler.SequenceNumber)
	if err != nil {
		log.Crit("Error creating state manager", "err", err)
		os.Exit(exitcodes.ExitTerminal)
	}

	credentialProvider := auth.NewBasicAWSCredentialProvider()
	awsCreds := auth.ToSDK(credentialProvider)
	client := api.NewECSClient(awsCreds, cfg, *acceptInsecureCert)

	if containerInstanceArn == "" {
		log.Info("Registering Instance with ECS")
		containerInstanceArn, err = client.RegisterContainerInstance()
		if err != nil {
			log.Error("Error registering", "err", err)
			if retriable, ok := err.(utils.Retriable); ok && !retriable.Retry() {
				os.Exit(exitcodes.ExitTerminal)
			}
			os.Exit(exitcodes.ExitError)
		}
		log.Info("Registration completed successfully", "containerInstance", containerInstanceArn, "cluster", cfg.Cluster)
		// Save our shiny new containerInstanceArn
		stateManager.Save()
	} else {
		log.Info("Restored state", "containerInstance", containerInstanceArn, "cluster", cfg.Cluster)
	}

	// Begin listening to the docker daemon and saving changes
	taskEngine.SetSaver(stateManager)
	taskEngine.MustInit()

	go sighandlers.StartTerminationHandler(stateManager, taskEngine)

	// Agent introspection api
	go handlers.ServeHttp(&containerInstanceArn, taskEngine, cfg)

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(taskEngine, client, stateManager)

	log.Info("Beginning Polling for updates")
	err = acshandler.StartSession(containerInstanceArn, credentialProvider, cfg, taskEngine, client, stateManager, *acceptInsecureCert)
	if err != nil {
		log.Crit("Unretriable error starting communicating with ACS", "err", err)
		os.Exit(exitcodes.ExitTerminal)
	}
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
		statemanager.AddSaveable("SeqNum", sequenceNumber),
	)
	if err != nil {
		return nil, err
	}
	return stateManager, nil
}
