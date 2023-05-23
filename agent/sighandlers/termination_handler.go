// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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

// Package sighandlers handle signals and behave appropriately.
// SIGTERM:
//
//	Flush state to disk and exit
//
// SIGUSR1:
//
//	Print a dump of goroutines to the logger and DON'T exit
package sighandlers

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/data"
	"github.com/aws/amazon-ecs-agent/agent/engine"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerstate"
	"github.com/aws/amazon-ecs-agent/agent/sighandlers/exitcodes"
	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"

	"github.com/cihub/seelog"
	bolt "go.etcd.io/bbolt"
)

const (
	engineDisableTimeout = 5 * time.Second
	finalSaveTimeout     = 3 * time.Second
)

// TerminationHandler defines a handler used for terminating the agent
type TerminationHandler func(state dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc)

// StartDefaultTerminationHandler defines a default termination handler suitable for running in a process
func StartDefaultTerminationHandler(state dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine, cancel context.CancelFunc) {
	// when we receive a termination signal, first save the state, then
	// cancel the agent's context so other goroutines can exit cleanly.
	signalC := make(chan os.Signal, 2)
	signal.Notify(signalC, os.Interrupt, syscall.SIGTERM)

	sig := <-signalC
	seelog.Infof("Agent received termination signal: %s", sig.String())

	err := FinalSave(state, dataClient, taskEngine)
	if err != nil {
		seelog.Criticalf("Error saving state before final shutdown: %v", err)
		// Terminal because it's a sigterm; the user doesn't want it to restart
		os.Exit(exitcodes.ExitTerminal)
	}
	cancel()
}

// FinalSave should be called immediately before exiting, and only before
// exiting, in order to flush tasks to disk. It waits a short timeout for state
// to settle if necessary. If unable to reach a steady-state and save within
// this short timeout, it returns an error
func FinalSave(state dockerstate.TaskEngineState, dataClient data.Client, taskEngine engine.TaskEngine) error {
	engineDisabled := make(chan error)

	disableTimer := time.AfterFunc(engineDisableTimeout, func() {
		engineDisabled <- errors.New("final save: timed out waiting for TaskEngine to settle")
	})

	go func() {
		seelog.Debug("Shutting down task engine for final save")
		taskEngine.Disable()
		disableTimer.Stop()
		engineDisabled <- nil
	}()

	disableErr := <-engineDisabled

	stateSaved := make(chan error)
	go func() {
		seelog.Debug("Saving state before shutting down")
		saveTimer := time.AfterFunc(finalSaveTimeout, func() {
			stateSaved <- errors.New("final save: timed out trying to save to disk")
		})
		saveStateAll(state, dataClient)
		saveTimer.Stop()
		stateSaved <- nil
	}()

	saveErr := <-stateSaved

	if disableErr != nil || saveErr != nil {
		return apierrors.NewMultiError(disableErr, saveErr)
	}
	return nil
}

func saveStateAll(state dockerstate.TaskEngineState, dataClient data.Client) {
	// save all tasks state data
	for _, task := range state.AllTasks() {
		if err := dataClient.SaveTask(task); err != nil {
			seelog.Errorf("Failed to save data for task %s: %v", task.Arn, err)
		}
	}

	// save all container and docker container state data
	for _, containerId := range state.GetAllContainerIDs() {
		container, ok := state.ContainerByID(containerId)
		if !ok {
			seelog.Errorf("Unable to find container for %s", containerId)
		}
		if err := dataClient.SaveDockerContainer(container); err != nil {
			seelog.Errorf("Failed to save data for docker container %s: %v", container.Container.Name, err)
		}
	}

	// save all image state data
	for _, imageState := range state.AllImageStates() {
		if err := dataClient.SaveImageState(imageState); err != nil {
			seelog.Errorf("Failed to save data for image state %s:, %v", imageState.GetImageID(), err)
		}
	}

	// save all eni attachment data
	for _, eniAttachment := range state.AllENIAttachments() {
		if err := dataClient.SaveENIAttachment(eniAttachment); err != nil {
			seelog.Errorf("Failed to save data for eni attachment %s: %v", eniAttachment.AttachmentARN, err)
		}
	}
	// Wait to ensure data is saved.
	time.Sleep(bolt.DefaultMaxBatchDelay)
}
