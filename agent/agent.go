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
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

func init() {
	mathrand.Seed(time.Now().UnixNano())
}

func main() {
	insecureCert := flag.Bool("k", false, "Do not verify ssl certs")
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

	log.Info("Connecting to docker daemon")
	taskEngine := engine.MustTaskEngine()
	log.Info("Connected to docker daemon")

	credentialProvider := auth.NewBasicAWSCredentialProvider()
	client := api.NewECSClient(credentialProvider, cfg, *insecureCert)

	var containerInstanceArn string

	log.Info("Registering Instance with ECS")
	containerInstanceArn, err = client.RegisterContainerInstance()
	if err != nil {
		log.Error("Error registering", "err", err)
		os.Exit(1)
	}
	log.Info("Registration Completed Successfully. Running as: " + containerInstanceArn)

	sighandlers.StartTerminationHandler(containerInstanceArn, client)

	// Agent introspection api
	go handlers.ServeHttp(&containerInstanceArn, taskEngine, cfg)

	// Start sending events to the backend
	go eventhandler.HandleEngineEvents(taskEngine, client)

	// TODO load known state from disk
	// taskEngine.LoadState()

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

			state_changes, errc, err := acsObj.Poll(*insecureCert)
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
							// TODO, write state to disk here
							acsObj.Ack(payload)
							for _, task := range payload.Tasks {
								taskEngine.AddTask(task)
							}
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
