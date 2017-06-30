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

func (agent *ecsAgent) waitOnChildProcesses(ctx context.Context) {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGCHLD)

	log.Infof("Starting signal processing loop for SIGCHLD")
	go processSignal(ctx, signals)
	log.Infof("Starting wait loop for child processes")
	doWait(ctx)
}

func processSignal(ctx context.Context, signals <-chan os.Signal) {
	for {
		select {
		case s := <-signals:
			log.Debugf("Received SIGCHLD: %s", s.String())
		case <-ctx.Done():
			return
		}
	}
}

func doWait(ctx context.Context) {
	waitError := make(chan error)
	go func() { waitError <- wait() }()
	for {
		select {
		case <-waitError:
			go func() { waitError <- wait() }()
		case <-ctx.Done():
			return
		}
	}
}

func wait() error {
	var ws syscall.WaitStatus
	var ru syscall.Rusage
	_, err := syscall.Wait4(-1, &ws, 0, &ru)
	if err != nil {
		log.Errorf("Error waiting for child: %v", err)
		return err
	}

	return nil
}
