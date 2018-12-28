// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
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
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
	cnitypes "github.com/containernetworking/cni/pkg/types"
)

func parseCheckpoint(dataDir string) bool {
	var checkPoint bool
	if dataDir != "" {
		// if we have a directory to checkpoint to, default it to be on
		checkPoint = utils.ParseBool(os.Getenv("ECS_CHECKPOINT"), true)
	} else {
		// if the directory is not set, default to checkpointing off for
		// backwards compatibility
		checkPoint = utils.ParseBool(os.Getenv("ECS_CHECKPOINT"), false)
	}
	return checkPoint
}

func parseReservedPorts(env string) []uint16 {
	// Format: json array, e.g. [1,2,3]
	reservedPortEnv := os.Getenv(env)
	portDecoder := json.NewDecoder(strings.NewReader(reservedPortEnv))
	var reservedPorts []uint16
	err := portDecoder.Decode(&reservedPorts)
	// EOF means the string was blank as opposed to UnexepctedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		err := fmt.Errorf("Invalid format for \"%s\" environment variable; expected a JSON array like [1,2,3]. err %v", env, err)
		seelog.Warn(err)
	}

	return reservedPorts
}

func parseDockerStopTimeout() time.Duration {
	var dockerStopTimeout time.Duration
	parsedStopTimeout := parseEnvVariableDuration("ECS_CONTAINER_STOP_TIMEOUT")
	if parsedStopTimeout >= minimumDockerStopTimeout {
		dockerStopTimeout = parsedStopTimeout
		// if the ECS_CONTAINER_STOP_TIMEOUT is invalid or empty, then the parsedStopTimeout
		// will be 0, in this case we should return a 0,
		// because the DockerStopTimeout will merge with the DefaultDockerStopTimeout,
		// only when the DockerStopTimeout is empty
	} else if parsedStopTimeout != 0 {
		// if the configured ECS_CONTAINER_STOP_TIMEOUT is smaller than minimumDockerStopTimeout,
		// DockerStopTimeout will be set to minimumDockerStopTimeout
		// if the ECS_CONTAINER_STOP_TIMEOUT is 0, empty or an invalid value, then DockerStopTimeout
		// will be set to defaultDockerStopTimeout during the config merge operation
		dockerStopTimeout = minimumDockerStopTimeout
		seelog.Warnf("Discarded invalid value for docker stop timeout, parsed as: %v", parsedStopTimeout)
	}
	return dockerStopTimeout
}

func parseContainerStartTimeout() time.Duration {
	var containerStartTimeout time.Duration
	parsedStartTimeout := parseEnvVariableDuration("ECS_CONTAINER_START_TIMEOUT")
	if parsedStartTimeout >= minimumContainerStartTimeout {
		containerStartTimeout = parsedStartTimeout
		// do the parsedStartTimeout != 0 check for the same reason as in getDockerStopTimeout()
	} else if parsedStartTimeout != 0 {
		containerStartTimeout = minimumContainerStartTimeout
		seelog.Warnf("Discarded invalid value for container start timeout, parsed as: %v", parsedStartTimeout)
	}
	return containerStartTimeout
}

func parseImagePullInactivityTimeout() time.Duration {
	var imagePullInactivityTimeout time.Duration
	parsedImagePullInactivityTimeout := parseEnvVariableDuration("ECS_IMAGE_PULL_INACTIVITY_TIMEOUT")
	if parsedImagePullInactivityTimeout >= minimumImagePullInactivityTimeout {
		imagePullInactivityTimeout = parsedImagePullInactivityTimeout
		// do the parsedStartTimeout != 0 check for the same reason as in getDockerStopTimeout()
	} else if parsedImagePullInactivityTimeout != 0 {
		imagePullInactivityTimeout = minimumImagePullInactivityTimeout
		seelog.Warnf("Discarded invalid value for image pull inactivity timeout, parsed as: %v", parsedImagePullInactivityTimeout)
	}
	return imagePullInactivityTimeout
}

func parseAvailableLoggingDrivers() []dockerclient.LoggingDriver {
	availableLoggingDriversEnv := os.Getenv("ECS_AVAILABLE_LOGGING_DRIVERS")
	loggingDriverDecoder := json.NewDecoder(strings.NewReader(availableLoggingDriversEnv))
	var availableLoggingDrivers []dockerclient.LoggingDriver
	err := loggingDriverDecoder.Decode(&availableLoggingDrivers)
	// EOF means the string was blank as opposed to UnexpectedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		err := fmt.Errorf("Invalid format for \"ECS_AVAILABLE_LOGGING_DRIVERS\" environment variable; expected a JSON array like [\"json-file\",\"syslog\"]. err %v", err)
		seelog.Warn(err)
	}

	return availableLoggingDrivers
}

func parseNumImagesToDeletePerCycle() int {
	numImagesToDeletePerCycleEnvVal := os.Getenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE")
	numImagesToDeletePerCycle, err := strconv.Atoi(numImagesToDeletePerCycleEnvVal)
	if numImagesToDeletePerCycleEnvVal != "" && err != nil {
		seelog.Warnf("Invalid format for \"ECS_NUM_IMAGES_DELETE_PER_CYCLE\", expected an integer. err %v", err)
	}

	return numImagesToDeletePerCycle
}

func parseNumNonECSContainersToDeletePerCycle() int {
	numNonEcsContainersToDeletePerCycleEnvVal := os.Getenv("NONECS_NUM_CONTAINERS_DELETE_PER_CYCLE")
	numNonEcsContainersToDeletePerCycle, err := strconv.Atoi(numNonEcsContainersToDeletePerCycleEnvVal)
	if numNonEcsContainersToDeletePerCycleEnvVal != "" && err != nil {
		seelog.Warnf("Invalid format for \"NONECS_NUM_CONTAINERS_DELETE_PER_CYCLE\", expected an integer. err %v", err)
	}
	return numNonEcsContainersToDeletePerCycle
}

func parseImagePullBehavior() ImagePullBehaviorType {
	ImagePullBehaviorString := os.Getenv("ECS_IMAGE_PULL_BEHAVIOR")
	switch ImagePullBehaviorString {
	case "always":
		return ImagePullAlwaysBehavior
	case "once":
		return ImagePullOnceBehavior
	case "prefer-cached":
		return ImagePullPreferCachedBehavior
	default:
		// Use the default image pull behavior when ECS_IMAGE_PULL_BEHAVIOR is
		// "default" or not valid
		return ImagePullDefaultBehavior
	}
}

func parseInstanceAttributes(errs []error) (map[string]string, []error) {
	var instanceAttributes map[string]string
	instanceAttributesEnv := os.Getenv("ECS_INSTANCE_ATTRIBUTES")
	err := json.Unmarshal([]byte(instanceAttributesEnv), &instanceAttributes)
	if instanceAttributesEnv != "" {
		if err != nil {
			wrappedErr := fmt.Errorf("Invalid format for ECS_INSTANCE_ATTRIBUTES. Expected a json hash: %v", err)
			seelog.Error(wrappedErr)
			errs = append(errs, wrappedErr)
		}
	}
	for attributeKey, attributeValue := range instanceAttributes {
		seelog.Debugf("Setting instance attribute %v: %v", attributeKey, attributeValue)
	}

	return instanceAttributes, errs
}

func parseAdditionalLocalRoutes(errs []error) ([]cnitypes.IPNet, []error) {
	var additionalLocalRoutes []cnitypes.IPNet
	additionalLocalRoutesEnv := os.Getenv("ECS_AWSVPC_ADDITIONAL_LOCAL_ROUTES")
	if additionalLocalRoutesEnv != "" {
		err := json.Unmarshal([]byte(additionalLocalRoutesEnv), &additionalLocalRoutes)
		if err != nil {
			seelog.Errorf("Invalid format for ECS_AWSVPC_ADDITIONAL_LOCAL_ROUTES, expected a json array of CIDRs: %v", err)
			errs = append(errs, err)
		}
	}

	return additionalLocalRoutes, errs
}

func parseTaskCPUMemLimitEnabled() Conditional {
	var taskCPUMemLimitEnabled Conditional
	taskCPUMemLimitConfigString := os.Getenv("ECS_ENABLE_TASK_CPU_MEM_LIMIT")

	// We only want to set taskCPUMemLimit if it is explicitly set to true or false.
	// We can do this by checking against the ParseBool default
	if taskCPUMemLimitConfigString != "" {
		if utils.ParseBool(taskCPUMemLimitConfigString, false) {
			taskCPUMemLimitEnabled = ExplicitlyEnabled
		} else {
			taskCPUMemLimitEnabled = ExplicitlyDisabled
		}
	}
	return taskCPUMemLimitEnabled
}

func parseTaskMetadataThrottles() (int, int) {
	var steadyStateRate, burstRate int
	rpsLimitEnvVal := os.Getenv("ECS_TASK_METADATA_RPS_LIMIT")
	if rpsLimitEnvVal == "" {
		seelog.Debug("Environment variable empty: ECS_TASK_METADATA_RPS_LIMIT")
		return 0, 0
	}
	rpsLimitSplits := strings.Split(rpsLimitEnvVal, ",")
	if len(rpsLimitSplits) != 2 {
		seelog.Warn(`Invalid format for "ECS_TASK_METADATA_RPS_LIMIT", expected: "rateLimit,burst"`)
		return 0, 0
	}
	steadyStateRate, err := strconv.Atoi(strings.TrimSpace(rpsLimitSplits[0]))
	if err != nil {
		seelog.Warnf(`Invalid format for "ECS_TASK_METADATA_RPS_LIMIT", expected integer for steady state rate: %v`, err)
		return 0, 0
	}
	burstRate, err = strconv.Atoi(strings.TrimSpace(rpsLimitSplits[1]))
	if err != nil {
		seelog.Warnf(`Invalid format for "ECS_TASK_METADATA_RPS_LIMIT", expected integer for burst rate: %v`, err)
		return 0, 0
	}
	return steadyStateRate, burstRate
}

func parseContainerInstanceTags(errs []error) (map[string]string, []error) {
	var containerInstanceTags map[string]string
	containerInstanceTagsConfigString := os.Getenv("ECS_CONTAINER_INSTANCE_TAGS")

	// If duplicate keys exist, the value of the key will be the value of latter key.
	err := json.Unmarshal([]byte(containerInstanceTagsConfigString), &containerInstanceTags)
	if containerInstanceTagsConfigString != "" {
		if err != nil {
			wrappedErr := fmt.Errorf("Invalid format for ECS_CONTAINER_INSTANCE_TAGS. Expected a json hash: %v", err)
			seelog.Error(wrappedErr)
			errs = append(errs, wrappedErr)
		}
	}

	for tagKey, tagValue := range containerInstanceTags {
		seelog.Debugf("Setting instance tag %v: %v", tagKey, tagValue)
	}

	return containerInstanceTags, errs
}

func parseContainerInstancePropagateTagsFrom() ContainerInstancePropagateTagsFromType {
	containerInstancePropagateTagsFromString := os.Getenv("ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM")
	switch containerInstancePropagateTagsFromString {
	case "ec2_instance":
		return ContainerInstancePropagateTagsFromEC2InstanceType
	default:
		// Use the default "none" type when ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM is
		// "none" or not valid.
		return ContainerInstancePropagateTagsFromNoneType
	}
}

func parseEnvVariableUint16(envVar string) uint16 {
	envVal := os.Getenv(envVar)
	var var16 uint16
	if envVal != "" {
		var64, err := strconv.ParseUint(envVal, 10, 16)
		if err != nil {
			seelog.Warnf("Invalid format for \""+envVar+"\" environment variable; expected unsigned integer. err %v", err)
		} else {
			var16 = uint16(var64)
		}
	}
	return var16
}

func parseEnvVariableDuration(envVar string) time.Duration {
	var duration time.Duration
	envVal := os.Getenv(envVar)
	if envVal == "" {
		seelog.Debugf("Environment variable empty: %v", envVar)
	} else {
		var err error
		duration, err = time.ParseDuration(envVal)
		if err != nil {
			seelog.Warnf("Could not parse duration value: %v for Environment Variable %v : %v", envVal, envVar, err)
		}
	}
	return duration
}

func parseImageCleanupExclusionList(envVar string) []string {
	imageEnv := os.Getenv(envVar)
	var imageCleanupExclusionList []string
	if imageEnv == "" {
		seelog.Debugf("Environment variable empty: %s", imageEnv)
	} else {
		imageCleanupExclusionList = strings.Split(imageEnv, ",")
	}

	// append known cached internal images to imageCleanupExclusionLis
	imageCleanupExclusionList = append(imageCleanupExclusionList, CachedImageNameAgentContainer, CachedImageNamePauseContainer)

	for _, image := range imageCleanupExclusionList {
		seelog.Infof("Image excluded from cleanup: %s", image)
	}
	return imageCleanupExclusionList
}
