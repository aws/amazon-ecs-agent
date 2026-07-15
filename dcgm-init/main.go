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
	"path/filepath"

	"github.com/aws/amazon-ecs-agent/dcgm-init/engine"
	"github.com/aws/amazon-ecs-agent/dcgm-init/version"
	"github.com/aws/amazon-ecs-agent/ecs-agent/logger"

	"github.com/cihub/seelog"
)

// log config
const (
	logFile                      = "/var/log/ecs/dcgm-init.log"
	logDirPermission os.FileMode = 0755
)

// all supported commands
const (
	VERSION = "version"
	START   = "start"
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		usage(actions(nil))
		os.Exit(1)
	}

	configureLogging()
	defer seelog.Flush()

	if args[0] == VERSION {
		if err := version.PrintVersion(); err != nil {
			seelog.Errorf("failed to print version info: %v", err)
		}
		return
	}

	init, err := engine.New()
	if err != nil {
		die(err, engine.DefaultInitErrorExitCode)
	}
	seelog.Info(args[0])
	actions := actions(init)
	action, ok := actions[args[0]]
	if !ok {
		usage(actions)
		seelog.Flush()
		os.Exit(1)
	}
	err = action.function()

	if err != nil {
		die(err, engine.DefaultInitErrorExitCode)
	}
}

// configureLogging sets up dcgm-init's file logging to logFile.
func configureLogging() {
	logger.InitSeelog()

	// Set our file first to override any ECS_LOGFILE from the environment.
	logger.SetConfigLogFile(logFile)
	logger.SetRolloverType("date")

	// The file level defaults to "off" under ECS_LOG_DRIVER, so force it from
	// ECS_LOGLEVEL (info when unset/invalid) so the file is always written.
	logger.SetInstanceLogLevel(logger.DEFAULT_LOGLEVEL)
	if level := os.Getenv(logger.LOGLEVEL_ENV_VAR); level != "" {
		logger.SetInstanceLogLevel(level)
	}

	// Best-effort: seelog also creates the dir lazily; this just warns clearly
	// on failure and applies our mode.
	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, logDirPermission); err != nil {
		seelog.Warnf("dcgm-init could not create log directory %s: %v", logDir, err)
	}
}

type action struct {
	function    func() error
	description string
}

func actions(engine *engine.Engine) map[string]action {
	return map[string]action{
		START: action{
			function:    engine.Start,
			description: "Start collecting GPU metrics",
		},
	}
}

func usage(actions map[string]action) {
	fmt.Printf("Usage: %s ACTION\n", os.Args[0])
	fmt.Println("")
	fmt.Println(" Available actions:")
	for command, action := range actions {
		fmt.Printf("  %-15s  %s\n", command, action.description)
	}
	fmt.Println("")
}

func die(err error, exitCode int) {
	seelog.Error(err.Error())
	seelog.Flush()
	os.Exit(exitCode)
}
