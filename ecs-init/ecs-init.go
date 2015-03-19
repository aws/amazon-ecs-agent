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
	"os"

	"github.com/aws/amazon-ecs-init/ecs-init/cache"
	"github.com/aws/amazon-ecs-init/ecs-init/docker"

	log "github.com/cihub/seelog"
)

func main() {
	defer log.Flush()

	downloader := cache.NewDownloader()
	err := downloader.DownloadAgent()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	agentImage, err := downloader.LoadCachedAgent()
	if err != nil {
		log.Error(err)
		os.Exit(2)
	}

	docker, err := docker.NewClient()
	if err != nil {
		log.Error(err)
		os.Exit(3)
	}

	loaded, err := docker.IsAgentImageLoaded()
	if err != nil {
		log.Error(err)
		os.Exit(4)
	}
	log.Infof("Image loaded: %t", loaded)

	err = docker.RemoveExistingAgentContainer()
	if err != nil {
		log.Error(err)
		os.Exit(5)
	}

	err = docker.LoadImage(agentImage)
	if err != nil {
		log.Error(err)
		os.Exit(6)
	}

	retval, err := docker.StartAgent()
	if err != nil {
		log.Error(err)
		os.Exit(7)
	}
	log.Infof("Agent exited with code %d", retval)
}
