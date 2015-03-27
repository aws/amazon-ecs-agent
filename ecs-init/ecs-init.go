// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
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

	"github.com/aws/amazon-ecs-init/ecs-init/config"
	"github.com/aws/amazon-ecs-init/ecs-init/engine"

	log "github.com/cihub/seelog"
)

func main() {
	defer log.Flush()
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		usage(actions(nil))
		os.Exit(1)
	}

	logger, err := log.LoggerFromConfigAsString(config.Logger())
	if err != nil {
		die(err)
	}
	log.ReplaceLogger(logger)

	init, err := engine.New()
	if err != nil {
		die(err)
	}
	log.Info(args[0])
	actions := actions(init)
	action, ok := actions[args[0]]
	if !ok {
		usage(actions)
		os.Exit(1)
	}
	err = action.function()
	if err != nil {
		die(err)
	}
}

type action struct {
	function    func() error
	description string
}

func actions(engine *engine.Engine) map[string]action {
	return map[string]action{
		"pre-start": action{
			function:    engine.PreStart,
			description: "Prepare the ECS Agent for starting",
		},
		"start": action{
			function:    engine.Start,
			description: "Start the ECS Agent and wait for it to stop",
		},
		"pre-stop": action{
			function:    engine.PreStop,
			description: "Stop the ECS Agent",
		},
		"update-cache": action{
			function:    engine.UpdateCache,
			description: "Update the cached image of the ECS Agent",
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

func die(err error) {
	log.Error(err.Error())
	log.Flush()
	os.Exit(-1)
}
