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
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	// AgentImageName is the name of the Docker image containing the Agent
	AgentImageName = "amazon/amazon-ecs-agent:latest"

	// AgentContainerName is the name of the Agent container started by this program
	AgentContainerName = "ecs-agent"

	// AgentLogFile is the name of the log file used by the Agent
	AgentLogFile = "ecs-agent.log"

	UnixSocketPrefix = "unix://"

	// Default region name
	DefaultRegionName = "default"
)

var S3BucketMap = map[string]string{
	"us-east-1" : "https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-v1.14.1.tar",
	"cn-north-1" : "https://s3.cn-north-1.amazonaws.com.cn/amazon-ecs-agent/ecs-agent-v1.14.1.tar",
	"default" : "https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-v1.14.1.tar",
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
	return FindTarballUrl()
}

// AgentRemoteTarballMD5 is the remote location of a md5sum used to verify the integrity of the AgentRemoteTarball
func AgentRemoteTarballMD5() string {
	return AgentRemoteTarball() + ".md5"
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

// Find AZ information from Metadata. If error return a blank result
func EC2MetadataAZ () string {
	sessionInstance := session.Must(session.NewSession())
	metadata := ec2metadata.New(sessionInstance)
	azName, err := metadata.GetMetadata("placement/availability-zone")

	if err != nil {
		return ""
	}

	return azName
}

//convert AZ name to Region name or default if blank is returned
func AZToRegionName (azName string) string {
	if azName == "" {
		return DefaultRegionName
	}

	return azName[0:len(azName)-1]
}

// Get Bucket from list of S3 Buckets by region name or default if key is not found
func GetS3BucketMapByRegion(regionName string) string {
	val, exists := S3BucketMap[regionName]
	if !exists {
		return S3BucketMap[DefaultRegionName]
	}

	return val
}

// Retreive tarball URL from S3 Bucket list by searching for region name
func FindTarballUrl () string {
	azName := EC2MetadataAZ()
	regionName := AZToRegionName(azName)
	return GetS3BucketMapByRegion(regionName)
}
