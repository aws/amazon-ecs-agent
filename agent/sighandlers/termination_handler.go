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
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	"github.com/aws/amazon-ecs-agent/agent/statemanager"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var log = logger.ForModule("TerminationHandler")

func StartTerminationHandler(saver statemanager.Saver, taskEngine engine.TaskEngine) {
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	sig := <-signalChannel
	log.Debug("Received termination signal", "signal", sig.String())

	err := FinalSave(saver, taskEngine)
	if err != nil {
		log.Crit("Error saving state before final shutdown", "err", err)
		// Terminal because it's a sigterm; the user doesn't want it to restart
		os.Exit(exitcodes.ExitTerminal)
	}
	os.Exit(exitcodes.ExitSuccess)
}

const engineDisableTimeout = 5 * time.Second
const finalSaveTimeout = 3 * time.Second

// FinalSave should be called immediately before exiting, and only before
// exiting, in order to flush tasks to disk. It waits a short timeout for state
// to settle if necessary. If unable to reach a steady-state and save within
// this short timeout, it returns an error
func FinalSave(saver statemanager.Saver, taskEngine engine.TaskEngine) error {
	engineDisabled := make(chan error)

	disableTimer := time.AfterFunc(engineDisableTimeout, func() {
		engineDisabled <- errors.New("Timed out waiting for TaskEngine to settle")
	})

	go func() {
		log.Debug("Shutting down task engine")
		taskEngine.Disable()
		disableTimer.Stop()
		engineDisabled <- nil
	}()

	disableErr := <-engineDisabled

	stateSaved := make(chan error)
	saveTimer := time.AfterFunc(finalSaveTimeout, func() {
		stateSaved <- errors.New("Timed out trying to save to disk")
	})
	go func() {
		log.Debug("Saving state before shutting down")
		stateSaved <- saver.ForceSave()
		saveTimer.Stop()
	}()

	saveErr := <-stateSaved

	if disableErr != nil || saveErr != nil {
		return utils.NewMultiError(disableErr, saveErr)
	}
	return nil
}
