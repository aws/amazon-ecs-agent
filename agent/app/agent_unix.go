// +build linux

// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package app

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/cihub/seelog"
	"golang.org/x/net/context"
)

// startSigchldHandler registers a channel to receiev SIGCHLD signals for
// the agent process. On receiving SIGCHLD, it invokes the wait4 syscall
// so that child processes are appropriately cleaned up.
func (agent *ecsAgent) startSigchldHandler(ctx context.Context) {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGCHLD)
	go processSignal(ctx, signals)
}

func processSignal(ctx context.Context, signals <-chan os.Signal) {
	log.Info("Starting the SIGCHLD handler")
	for {
		select {
		case s := <-signals:
			log.Debugf("Received SIGCHLD: %s", s.String())
			go wait()
		case <-ctx.Done():
			log.Info("Stopping the SIGCHLD handler")
			return
		}
	}
}

// wait wraps the Wait4 syscall and returns the error if any
func wait() {
	var ws syscall.WaitStatus
	var ru syscall.Rusage
	// More information on wait4 syscall can be found in manual pages
	// via `man 2 wait4` command. The syscall is used to wait for state
	// changes in the child processes of the Agent. Examples of such
	// processes include CNI plugins and the dhclient processes started
	// by these plugins. As per man pages, if a wait is not performed,
	// then the terminated child remains in a "zombie" state, which is
	// especially problematic for the dhclient process as the network
	// resources, including the namespace that holds the ENI would not
	// be properly cleaned up.
	//
	// wait4(pid, status, options, rusage) is equivalent to
	// the waitpid(pid, status, options) syscall. The value of -1 for
	// the pid field means that we wait for any child process. The
	// handles for waitstatus and rusage will be populated and can be
	// optionall used to infer the status of the child process when
	// needed. The options field is set to 0 as we are not setting any
	// additional options on the wait4 syscall.
	if _, err := syscall.Wait4(-1, &ws, 0, &ru); err != nil {
		log.Debugf("Error waiting for state change of the child process: %v", err)
		return
	}
	log.Debug("Wait for state change of the child process complete")
}
