//go:build linux

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//    http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aws/amazon-ecs-agent/ecs-agent/gpu/dcgm"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"
	"github.com/cihub/seelog"
)

const (
	START = "start"
	STOP  = "stop"
)

func main() {
	socketPath := flag.String("socket-path", dcgm.DefaultSocketPath, "Path to the DCGM nv-hostengine Unix domain socket")
	outputPath := flag.String("output", dcgm.DefaultOutputPath, "Path to write GPU metrics JSON output")
	collectionFreq := flag.Duration("interval", dcgm.DefaultCollectionFreq, "Metrics collection interval")
	oneShot := flag.Bool("once", false, "Collect metrics once and exit")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	logger.InitSeelog()
	defer seelog.Flush()

	logger.Info("dcgm-init invoked", logger.Fields{"command": args[0]})

	eng := dcgm.NewEngine(*socketPath, *outputPath, *collectionFreq, *oneShot)
	actions := actions(eng)

	action, ok := actions[args[0]]
	if !ok {
		usage()
		os.Exit(1)
	}

	if err := action.function(); err != nil {
		die(err)
	}
}

type action struct {
	function    func() error
	description string
}

func actions(eng *dcgm.Engine) map[string]action {
	return map[string]action{
		START: {
			function:    eng.Start,
			description: "Start collecting GPU metrics",
		},
		STOP: {
			function:    eng.Stop,
			description: "Stop collecting GPU metrics (handled by SIGTERM)",
		},
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags] COMMAND\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, " Available commands:\n")
	for cmd, a := range actions(nil) {
		fmt.Fprintf(os.Stderr, "  %-10s  %s\n", cmd, a.description)
	}
	fmt.Fprintf(os.Stderr, "\n")
}

func die(err error) {
	logger.Error("dcgm-init failed", logger.Fields{"error": err})
	seelog.Flush()
	os.Exit(1)
}
