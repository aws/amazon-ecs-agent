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

	"github.com/aws/amazon-ecs-agent/agent/acs"
	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/auth"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/eventhandler"
	"github.com/aws/amazon-ecs-agent/agent/handlers"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

func init() {
	mathrand.Seed(time.Now().UnixNano())
}

func main() {
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
		os.Exit(1)
	}

	containerInstanceArn := ""
	taskEngine := engine.NewTaskEngine()

	// Load any state from disk *before* talking to the docker daemon or any
	// network services such that the information we get from them is applied to
	// the correct state
	var state_manager statemanager.StateManager
	if !cfg.Checkpoint {
		state_manager = statemanager.NewNoopStateManager()
	} else {
		state_manager, err = statemanager.NewStateManager(cfg, statemanager.AddSaveable("TaskEngine", taskEngine), statemanager.AddSaveable("ContainerInstanceArn", &containerInstanceArn))
		if err != nil {
			log.Crit("Error creating state manager", "err", err)
			os.Exit(1)
		}
	}
	err = state_manager.Load()
	if err != nil {
		log.Crit("Error loading initial state", "err", err)
		os.Exit(1)
	}

	// Begin listening to the docker daemon and saving changes
	taskEngine.SetSaver(state_manager)
	taskEngine.MustInit()

	credentialProvider := auth.NewBasicAWSCredentialProvider()
	client := api.NewECSClient(credentialProvider, cfg, *acceptInsecureCert)

	if containerInstanceArn == "" {
		log.Info("Registering Instance with ECS")
		containerInstanceArn, err = client.RegisterContainerInstance()
		if err != nil {
			log.Error("Error registering", "err", err)
			os.Exit(1)
		}
		log.Info("Registration completed successfully", "containerInstance", containerInstanceArn, "cluster", cfg.ClusterArn)
		// Save our shiny new containerInstanceArn
		state_manager.Save()
	} else {
		log.Info("Restored state", "containerInstance", containerInstanceArn, "cluster", cfg.ClusterArn)
	}

	sighandlers.StartTerminationHandler(state_manager)

	// Agent introspection api
	go handlers.ServeHttp(&containerInstanceArn, taskEngine, cfg)

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(taskEngine, client, state_manager)

	log.Info("Beginning Polling for updates")
	// Todo, split into separate package
	for {
		backoff := utils.NewSimpleBackoff(time.Second, 1*time.Minute, 0.2, 2)
		utils.RetryWithBackoff(backoff, func() error {
			acsEndpoint, err := client.DiscoverPollEndpoint(containerInstanceArn)
			if err != nil {
				log.Error("Could not discover poll endpoint", "err", err)
				return err
			}
			log.Info("Discovered poll endpoint", "endpoint", acsEndpoint)
			acsObj := acs.NewAgentCommunicationClient(acsEndpoint, cfg, credentialProvider, containerInstanceArn)

			state_changes, errc, err := acsObj.Poll(*acceptInsecureCert)
			if err != nil {
				log.Error("Error polling; retrying", "err", err)
				return err
			}

			var err_ok bool
			for state_changes != nil {
				select {
				case state, state_ok := <-state_changes:
					if state_ok {
						backoff.Reset()

						go func(payload *acs.Payload) {
							for _, task := range payload.Tasks {
								taskEngine.AddTask(task)
							}

							err = state_manager.Save()
							if err != nil {
								log.Error("Error saving state", "err", err)
							}
							acsObj.Ack(payload)
						}(state)
					} else {
						// Break out of the loop to reconnect
						state_changes = nil
					}
				case err, err_ok = <-errc:
					if !err_ok {
						log.Error("Error channel unexpectedly closed")

						state_changes = nil // break out
					}
					log.Error("Error in state", "err", err)
				}
			}
			if err != nil {
				log.Warn("Error polling. Waiting and retrying")
				return err
			}
			// Shouldn't happen
			return nil
		})
	}
}
