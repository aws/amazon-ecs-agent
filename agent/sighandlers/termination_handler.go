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

// sighandlers handle signals and behave appropriately. Currently, the only
// supported signal is SIGTERM which causes state to be flushed to disk before
// exiting.
package sighandlers

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
)

var log = logger.ForModule("TerminationHandler")

func StartTerminationHandler(saver statemanager.Saver) {
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signalChannel
		log.Debug("Received termination signal", "signal", sig.String())
		var err error
		if forceSaver, ok := saver.(statemanager.ForceSaver); ok {
			err = forceSaver.ForceSave()
		} else {
			err = saver.Save()
		}
		if err != nil {
			log.Crit("Error saving state before final shutdown", "err", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
}
