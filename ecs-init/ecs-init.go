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

	"github.com/aws/amazon-ecs-init/ecs-init/cache"

	log "github.com/cihub/seelog"
)

const (
	configDirectory = "/tmp/etc/ecs"
	ecsConfigFile   = configDirectory + "/ecs.config"
	ecsJsonConfig   = configDirectory + "/ecs.config.json"
	imageName       = "amazon/amazon-ecs-agent"
	logDirectory    = "/tmp/var/log/ecs"
	initLogFile     = logDirectory + "/ecs-init.log"
	agentLogFile    = logDirectory + "/ecs-agent.log"
	dataDirectory   = "/tmp/var/lib/ecs/data"
)

func main() {
	defer log.Flush()
	displayVersion := flag.Bool("v", false, "Version")
	flag.Parse()

	if *displayVersion {
		fmt.Printf("Version %d\n", 1)
	}

	log.Info("Hello, world!")

	err := cache.NewDownloader().DownloadAgent()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

}
