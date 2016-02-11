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

package config

import (
	"os"
	"strings"
)

const (
	// AgentImageName is the name of the Docker image containing the Agent
	AgentImageName = "amazon/amazon-ecs-agent:latest"

	// AgentContainerName is the name of the Agent container started by this program
	AgentContainerName = "ecs-agent"

	// AgentLogFile is the name of the log file used by the Agent
	AgentLogFile = "ecs-agent.log"

	UnixSocketPrefix = "unix://"
)

// AgentConfigDirectory returns the location on disk for configuration
func AgentConfigDirectory() string {
	return directoryPrefix + "/etc/ecs"
}

// AgentConfigFile returns the location of a file of environment variables passed to the Agent
func AgentConfigFile() string {
	return AgentConfigDirectory() + "/ecs.config"
}

// AgentJSONConfigFile returns the location of a file containing configuration expressed in JSON
func AgentJSONConfigFile() string {
	return AgentConfigDirectory() + "/ecs.config.json"
}

// LogDirectory returns the location on disk where logs should be placed
func LogDirectory() string {
	return directoryPrefix + "/var/log/ecs"
}

func initLogFile() string {
	return LogDirectory() + "/ecs-init.log"
}

// AgentDataDirectory returns the location on disk where state should be saved
func AgentDataDirectory() string {
	return directoryPrefix + "/var/lib/ecs/data"
}

// CacheDirectory returns the location on disk where Agent images should be cached
func CacheDirectory() string {
	return directoryPrefix + "/var/cache/ecs"
}

// CacheState returns the location on disk where cache state is stored
func CacheState() string {
	return CacheDirectory() + "/state"
}

// AgentTarball returns the location on disk of the cached Agent image
func AgentTarball() string {
	return CacheDirectory() + "/ecs-agent.tar"
}

// AgentRemoteTarball is the remote location of the Agent image, used for populating the cache
func AgentRemoteTarball() string {
	return "https://s3.amazonaws.com/" + s3Bucket + "/ecs-agent-v1.8.0.tar"
}

// AgentRemoteTarballMD5 is the remote location of a md5sum used to verify the integrity of the AgentRemoteTarball
func AgentRemoteTarballMD5() string {
	return AgentRemoteTarball() + ".md5"
}

// DesiredImageLocatorFile returns the location on disk of a well-known file describing an Agent image to load
func DesiredImageLocatorFile() string {
	return CacheDirectory() + "/desired-image"
}

func CgroupDirectory() string {
	return cgroupDirectory
}

func ExecDriverDirectory() string {
	return execDriverDirectory
}

func DockerUnixSocket() string {
	if dockerHost := os.Getenv("DOCKER_HOST"); strings.HasPrefix(dockerHost, UnixSocketPrefix) {
		return strings.TrimPrefix(dockerHost, UnixSocketPrefix)
	}
	return "/var/run/docker.sock"
}
