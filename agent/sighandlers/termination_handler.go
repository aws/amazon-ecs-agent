// Copyright 2014 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
