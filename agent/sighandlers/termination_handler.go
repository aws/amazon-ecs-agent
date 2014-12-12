package sighandlers

// This is a temporary handler to deregister which will be obsoleted by
// correctly saving and restoring state when possible. TODO :)

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/logger"
)

func StartTerminationHandler(containerInstanceArn string, client api.ECSClient) {
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go handleSignal(signalChannel, containerInstanceArn, client)
}

func handleSignal(signalChannel chan os.Signal, containerInstanceArn string, client api.ECSClient) {
	log := logger.ForModule("TerminationHandler")

	sig := <-signalChannel
	log.Info("Received termination signal", "signal", sig.String())
	err := client.DeregisterContainerInstance(containerInstanceArn)
	if err != nil {
		log.Error("Failed to DeregisterContainerInstance", "containerInstanceArn", containerInstanceArn, "error", err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}
