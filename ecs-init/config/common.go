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

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/pkg/errors"
)

const (
	// AgentImageName is the name of the Docker image containing the Agent
	AgentImageName = "amazon/amazon-ecs-agent:latest"

	// AgentContainerName is the name of the Agent container started by this program
	AgentContainerName = "ecs-agent"

	// AgentLogFile is the name of the log file used by the Agent
	AgentLogFile = "ecs-agent.log"

	UnixSocketPrefix = "unix://"

	// Used to mount /proc for agent container
	ProcFS = "/proc"

	// AgentFilename is the filename, including version number, of the agent to be downloaded.
	AgentFilename = "ecs-agent-v1.19.1.tar"

	// AgentPartitionBucketName is the name of the paritional s3 bucket that stores the agent
	AgentPartitionBucketName = "amazon-ecs-agent"

	// DefaultRegionName is the default region to fall back if the user's region is not a region containing
	// the agent bucket
	DefaultRegionName = endpoints.UsEast1RegionID
)

var partitionBucketMap = map[string]string{
	endpoints.AwsPartitionID:      endpoints.UsEast1RegionID,
	endpoints.AwsCnPartitionID:    endpoints.CnNorth1RegionID,
	endpoints.AwsUsGovPartitionID: endpoints.UsGovWest1RegionID,
}

// GetAgentPartitionBucketRegion returns the s3 bucket region where ECS Agent artifact is located
func GetAgentPartitionBucketRegion(region string) (string, error) {
	regionPartition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region)
	if !ok {
		return "", errors.Errorf("GetAgentBucketRegion: partition not found for region: %s", region)
	}

	bucketRegion, ok := partitionBucketMap[regionPartition.ID()]
	if !ok {
		return "", errors.Errorf("GetAgentBucketRegion: partition not found: %s", regionPartition)
	}

	return bucketRegion, nil
}

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

// AgentDHClientLeasesDirectory returns the location on disk where dhclient
// leases information is tracked for ENIs attached to tasks
func AgentDHClientLeasesDirectory() string {
	return directoryPrefix + "/var/lib/ecs/dhclient"
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

// AgentRemoteTarball is the remote filename of the Agent image, used for populating the cache
func AgentRemoteTarballKey() string {
	return AgentFilename
}

// AgentRemoteTarballMD5 is the remote file of a md5sum used to verify the integrity of the AgentRemoteTarball
func AgentRemoteTarballMD5Key() string {
	return AgentRemoteTarballKey() + ".md5"
}

// DesiredImageLocatorFile returns the location on disk of a well-known file describing an Agent image to load
func DesiredImageLocatorFile() string {
	return CacheDirectory() + "/desired-image"
}

// DockerUnixSocket returns the docker socket endpoint and whether it's read from DOCKER_HOST
func DockerUnixSocket() (string, bool) {
	if dockerHost := os.Getenv("DOCKER_HOST"); strings.HasPrefix(dockerHost, UnixSocketPrefix) {
		return strings.TrimPrefix(dockerHost, UnixSocketPrefix), true
	}
	// return /var/run instead of /var/run/docker.sock, in case the /var/run/docker.sock is deleted and recreated outside the container,
	// eg: Docker daemon restart
	return "/var/run", false
}

// CgroupMountpoint returns the cgroup mountpoint for the system
func CgroupMountpoint() string {
	return cgroupMountpoint
}
