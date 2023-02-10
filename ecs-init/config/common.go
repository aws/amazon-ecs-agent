// Copyright 2015-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/cihub/seelog"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	godocker "github.com/fsouza/go-dockerclient"
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

	// DefaultAgentVersion is the version of the agent that will be
	// fetched if required. This should look like v1.2.3 or an
	// 8-character sha, as is downloadable from S3.
	DefaultAgentVersion = "v1.68.2"

	// AgentPartitionBucketName is the name of the paritional s3 bucket that stores the agent
	AgentPartitionBucketName = "amazon-ecs-agent"

	// DefaultRegionName is the default region to fall back if the user's region is not a region containing
	// the agent bucket
	DefaultRegionName = endpoints.UsEast1RegionID

	// dockerJSONLogMaxSize is the maximum allowed size of the
	// individual backing json log files for the managed container.
	dockerJSONLogMaxSize = "16m"
	// dockerJSONLogMaxSizeEnvVar is the environment variable that may
	// be used to override the default value of dockerJSONLogMaxSize
	// used for managed containers.
	dockerJSONLogMaxSizeEnvVar = "ECS_INIT_DOCKER_LOG_FILE_SIZE"

	// dockerJSONLogMaxFiles is the maximum rotated number of backing
	// json log files on disk managed by docker for the managed
	// container.
	dockerJSONLogMaxFiles = "4"
	// dockerJSONLogMaxSizeEnvVar is the environment variable that may
	// be used to override the default value used of
	// dockerJSONLogMaxFiles for managed containers.
	dockerJSONLogMaxFilesEnvVar = "ECS_INIT_DOCKER_LOG_FILE_NUM"

	// agentLogDriverEnvVar is the environment variable that may be used
	// to set a log driver for the agent container
	agentLogDriverEnvVar = "ECS_LOG_DRIVER"
	// agentLogOptionsEnvVar is the environment variable that may be used to specify options
	// for the log driver set in agentLogDriverEnvVar
	agentLogOptionsEnvVar = "ECS_LOG_OPTS"
	// defaultLogDriver is the logging driver that will be used if one is not explicitly
	// set in agentLogDriverEnvVar
	defaultLogDriver = "json-file"

	// GPUSupportEnvVar indicates that the AMI has support for GPU
	GPUSupportEnvVar = "ECS_ENABLE_GPU_SUPPORT"

	// DockerHostEnvVar is the environment variable that specifies the location of the Docker daemon socket.
	DockerHostEnvVar = "DOCKER_HOST"

	// ExternalEnvVar is the environment variable for specifying whether we are running in external (non-EC2) environment.
	ExternalEnvVar = "ECS_EXTERNAL"

	// DefaultRegionEnvVar is the environment variable for specifying the default AWS region to use.
	DefaultRegionEnvVar = "AWS_DEFAULT_REGION"

	// ECSGMSASupportEnvVar indicates that the gMSA is supported
	ECSGMSASupportEnvVar = "ECS_GMSA_SUPPORTED"

	// CredentialsFetcherHostEnvVar is the environment variable that specifies the location of the credentials-fetcher daemon socket.
	CredentialsFetcherHostEnvVar = "CREDENTIALS_FETCHER_HOST"

	// this socket is exposed by credentials-fetcher (daemon for gMSA support on Linux)
	// defaultCredentialsFetcherSocketPath is set to /var/credentials-fetcher/socket/credentials_fetcher.sock
	// in case path is not passed in the env variable
	DefaultCredentialsFetcherSocketPath = "/var/credentials-fetcher/socket/credentials_fetcher.sock"
)

// partitionBucketRegion provides the "partitional" bucket region
// suitable for downloading agent from.
var partitionBucketRegion = map[string]string{
	endpoints.AwsPartitionID:      endpoints.UsEast1RegionID,
	endpoints.AwsCnPartitionID:    endpoints.CnNorth1RegionID,
	endpoints.AwsUsGovPartitionID: endpoints.UsGovWest1RegionID,
}

// goarch is an injectable GOARCH runtime string. This controls the
// formatting of configuration for supported architectures.
var goarch string = runtime.GOARCH

// validDrivers is the set of all supported Docker logging drivers that
// can be used as the log driver for the Agent container
var validDrivers = map[string]struct{}{
	"awslogs":    {},
	"fluentd":    {},
	"gelf":       {},
	"json-file":  {},
	"journald":   {},
	"logentries": {},
	"syslog":     {},
	"splunk":     {},
}

// GetAgentPartitionBucketRegion returns the s3 bucket region where ECS Agent artifact is located
func GetAgentPartitionBucketRegion(region string) (string, error) {
	regionPartition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region)
	if !ok {
		return "", errors.Errorf("could not resolve partition ID for region %q", region)
	}

	bucketRegion, ok := partitionBucketRegion[regionPartition.ID()]
	if !ok {
		return "", errors.Errorf("no bucket available for partition ID %q", regionPartition.ID())
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

// AgentRemoteTarballKey is the remote filename of the Agent image, used for populating the cache
func AgentRemoteTarballKey() (string, error) {
	name, err := agentArtifactName(DefaultAgentVersion, goarch)
	if err != nil {
		return "", errors.Wrap(err, "no artifact available")
	}
	return fmt.Sprintf("%s.tar", name), nil
}

// AgentRemoteTarballMD5Key is the remote file of a md5sum used to verify the integrity of the AgentRemoteTarball
func AgentRemoteTarballMD5Key() (string, error) {
	tarballKey, err := AgentRemoteTarballKey()
	if err != nil {
		return "", err
	}
	return tarballKey + ".md5", nil
}

// DesiredImageLocatorFile returns the location on disk of a well-known file describing an Agent image to load
func DesiredImageLocatorFile() string {
	return CacheDirectory() + "/desired-image"
}

// DockerUnixSocket returns the docker socket endpoint and whether it's read from DockerHostEnvVar
func DockerUnixSocket() (string, bool) {
	if dockerHost := os.Getenv(DockerHostEnvVar); strings.HasPrefix(dockerHost, UnixSocketPrefix) {
		return strings.TrimPrefix(dockerHost, UnixSocketPrefix), true
	}
	// return /var/run instead of /var/run/docker.sock, in case the /var/run/docker.sock is deleted and recreated
	// outside the container, eg: Docker daemon restart
	return "/var/run", false
}

// credentialsFetcherUnixSocketHostPath returns the credentials fetcher daemon socket endpoint and whether it reads from CredentialsFetcherEnvVar
func credentialsFetcherUnixSocket() string {
	if credentialsFetcherHost := os.Getenv(CredentialsFetcherHostEnvVar); strings.HasPrefix(credentialsFetcherHost, UnixSocketPrefix) {
		return strings.TrimPrefix(credentialsFetcherHost, UnixSocketPrefix)
	}

	return DefaultCredentialsFetcherSocketPath
}

// HostCredentialsFetcherPath() returns the daemon socket location if it is available
func HostCredentialsFetcherPath() (string, bool) {
	if credentialsFetcherHost := credentialsFetcherUnixSocket(); len(credentialsFetcherHost) > 0 {
		return credentialsFetcherHost, true
	}
	return "", false
}

// CgroupMountpoint returns the cgroup mountpoint for the system
func CgroupMountpoint() string {
	return cgroupMountpoint
}

// HostCertsDirPath() returns the CA store path on the host
func HostCertsDirPath() string {
	if _, err := os.Stat(hostCertsDirPath); err != nil {
		return ""
	}
	return hostCertsDirPath
}

// HostPKIDirPath() returns the CA store path on the host
func HostPKIDirPath() string {
	if _, err := os.Stat(hostPKIDirPath); err != nil {
		return ""
	}
	return hostPKIDirPath
}

// AgentDockerLogDriverConfiguration returns a LogConfig object
// suitable for used with the managed container.
func AgentDockerLogDriverConfiguration() godocker.LogConfig {
	driver := defaultLogDriver
	options := parseLogOptions()
	if envDriver := os.Getenv(agentLogDriverEnvVar); envDriver != "" {
		if _, ok := validDrivers[envDriver]; ok {
			driver = envDriver
		} else {
			seelog.Warnf("Input value for \"ECS_LOG_DRIVER\" is not a supported log driver, overriding to %s and using default log options", defaultLogDriver)
			options = nil
		}
	}
	if driver == defaultLogDriver && options == nil {
		maxSize := dockerJSONLogMaxSize
		if fromEnv := os.Getenv(dockerJSONLogMaxSizeEnvVar); fromEnv != "" {
			maxSize = fromEnv
		}
		maxFiles := dockerJSONLogMaxFiles
		if fromEnv := os.Getenv(dockerJSONLogMaxFilesEnvVar); fromEnv != "" {
			maxFiles = fromEnv
		}
		options = map[string]string{
			"max-size": maxSize,
			"max-file": maxFiles,
		}
	}
	return godocker.LogConfig{
		Type:   driver,
		Config: options,
	}
}

func parseLogOptions() map[string]string {
	opts := os.Getenv(agentLogOptionsEnvVar)
	logOptsDecoder := json.NewDecoder(strings.NewReader(opts))
	var logOptions map[string]string
	err := logOptsDecoder.Decode(&logOptions)
	// blank string is not a warning
	if err != io.EOF && err != nil {
		seelog.Warnf("Invalid format for \"ECS_LOG_OPTS\", expected a json object with string key value. error: %v", err)
	}
	return logOptions
}

// InstanceConfigDirectory returns the location on disk for custom instance configuration
func InstanceConfigDirectory() string {
	return directoryPrefix + "/var/lib/ecs"
}

// InstanceConfigFile returns the location of a file of custom environment variables
func InstanceConfigFile() string {
	return InstanceConfigDirectory() + "/ecs.config"
}

// RunPrivileged returns if agent should be invoked with '--privileged'. This is not
// recommended and may be removed in future versions of amazon-ecs-init.
func RunPrivileged() bool {
	envVar := os.Getenv("ECS_AGENT_RUN_PRIVILEGED")
	return envVar == "true"
}

// RunningInExternal returns whether we are running in external (non-EC2) environment.
func RunningInExternal() bool {
	envVar := os.Getenv(ExternalEnvVar)
	return envVar == "true"
}

func agentArtifactName(version string, arch string) (string, error) {
	var interpose string
	switch arch {
	case "amd64":
		interpose = ""
	case "arm64":
		interpose = "-" + arch
	default:
		return "", errors.Errorf("unknown architecture %q", arch)
	}
	return fmt.Sprintf("ecs-agent%s-%s", interpose, version), nil
}
