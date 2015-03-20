// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
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

	"github.com/aws/amazon-ecs-init/ecs-init/engine"

	log "github.com/cihub/seelog"
)

func main() {
	defer log.Flush()
	flag.Parse()
	actions := flag.Args()

	if len(actions) == 0 {
		usage()
		os.Exit(1)
	}

	init, err := engine.New()
	if err != nil {
		die(err)
	}
	action := actions[0]
	log.Info(action)
	switch action {
	case "pre-start":
		err = init.PreStart()
	case "pre-stop":
		err = init.PreStop()
	case "start":
		err = init.Start()
	case "update-cache":
		err = init.UpdateCache()
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		die(err)
	}
}

func usage() {
	fmt.Printf("Usage: %s ACTION\n", os.Args[0])
	fmt.Println("")
	fmt.Println(" Available actions:")
	fmt.Println("  pre-start\tPrepare the ECS Agent for starting")
	fmt.Println("  start\tStart the ECS Agent and wait for it to stop")
	fmt.Println("  pre-stop\tStop the ECS Agent")
	fmt.Println("  update-cache\tUpdate the cached image of the ECS Agent")
	fmt.Println("")
}

func die(err error) {
	log.Error(err.Error())
	log.Flush()
	os.Exit(-1)
}
