// Copyright 2014-2016 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
	"github.com/aws/amazon-ecs-agent/agent/engine/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/cihub/seelog"
)

const (
	// http://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml?search=docker
	DockerReservedPort    = 2375
	DockerReservedSSLPort = 2376

	SSHPort = 22

	// AgentIntrospectionPort is used to serve the metadata about the agent and to query the tasks being managed by the agent.
	AgentIntrospectionPort = 51678

	// AgentCredentialsPort is used to serve the credentials for tasks.
	AgentCredentialsPort = 51679

	// DefaultClusterName is the name of the default cluster.
	DefaultClusterName = "default"

	// DefaultTaskCleanupWaitDuration specifies the default value for task cleanup duration. It is used to
	// clean up task's containers.
	DefaultTaskCleanupWaitDuration = 3 * time.Hour

	// DefaultDockerStopTimeout specifies the value for container stop timeout duration
	DefaultDockerStopTimeout = 30 * time.Second

	// DefaultImageCleanupTimeInterval specifies the default value for image cleanup duration. It is used to
	// remove the images pulled by agent.
	DefaultImageCleanupTimeInterval = 30 * time.Minute

	// DefaultNumImagesToDeletePerCycle specifies the default number of images to delete when agent performs
	// image cleanup.
	DefaultNumImagesToDeletePerCycle = 5

	//DefaultImageDeletionAge specifies the default value for minimum amount of elapsed time after an image
	// has been pulled before it can be deleted.
	DefaultImageDeletionAge = 1 * time.Hour

	// minimumTaskCleanupWaitDuration specifies the minimum duration to wait before cleaning up
	// a task's container. This is used to enforce sane values for the config.TaskCleanupWaitDuration field.
	minimumTaskCleanupWaitDuration = 1 * time.Minute

	// minimumDockerStopTimeout specifies the minimum value for docker StopContainer API
	minimumDockerStopTimeout = 1 * time.Second

	// minimumImageCleanupInterval specifies the minimum time for agent to wait before performing
	// image cleanup.
	minimumImageCleanupInterval = 10 * time.Minute

	// minimumNumImagesToDeletePerCycle specifies the minimum number of images that to be deleted when
	// performing image cleanup.
	minimumNumImagesToDeletePerCycle = 1
)

// Merge merges two config files, preferring the ones on the left. Any nil or
// zero values present in the left that are not present in the right will be
// overridden
func (lhs *Config) Merge(rhs Config) *Config {
	left := reflect.ValueOf(lhs).Elem()
	right := reflect.ValueOf(&rhs).Elem()

	for i := 0; i < left.NumField(); i++ {
		leftField := left.Field(i)
		if utils.ZeroOrNil(leftField.Interface()) {
			leftField.Set(reflect.ValueOf(right.Field(i).Interface()))
		}
	}

	return lhs //make it chainable
}

// complete returns true if all fields of the config are populated / nonzero
func (cfg *Config) complete() bool {
	cfgElem := reflect.ValueOf(cfg).Elem()

	for i := 0; i < cfgElem.NumField(); i++ {
		if utils.ZeroOrNil(cfgElem.Field(i).Interface()) {
			return false
		}
	}
	return true
}

// checkMissingAndDeprecated checks all zero-valued fields for tags of the form
// missing:STRING and acts based on that string. Current options are: fatal,
// warn. Fatal will result in an error being returned, warn will result in a
// warning that the field is missing being logged.
func (cfg *Config) checkMissingAndDepreciated() error {
	cfgElem := reflect.ValueOf(cfg).Elem()
	cfgStructField := reflect.Indirect(reflect.ValueOf(cfg)).Type()

	fatalFields := []string{}
	for i := 0; i < cfgElem.NumField(); i++ {
		cfgField := cfgElem.Field(i)
		if utils.ZeroOrNil(cfgField.Interface()) {
			missingTag := cfgStructField.Field(i).Tag.Get("missing")
			if len(missingTag) == 0 {
				continue
			}
			switch missingTag {
			case "warn":
				seelog.Warnf("Configuration key not set, key: %v", cfgStructField.Field(i).Name)
			case "fatal":
				seelog.Criticalf("Configuration key not set, key: %v", cfgStructField.Field(i).Name)
				fatalFields = append(fatalFields, cfgStructField.Field(i).Name)
			default:
				seelog.Warnf("Unexpected `missing` tag value, tag %v", missingTag)
			}
		} else {
			// present
			deprecatedTag := cfgStructField.Field(i).Tag.Get("deprecated")
			if len(deprecatedTag) == 0 {
				continue
			}
			seelog.Warnf("Use of deprecated configuration key, key: %v message: %v", cfgStructField.Field(i).Name, deprecatedTag)
		}
	}
	if len(fatalFields) > 0 {
		return errors.New("Missing required fields: " + strings.Join(fatalFields, ", "))
	}
	return nil
}

// trimWhitespace trims whitespace from all string config values with the
// `trim` tag
func (cfg *Config) trimWhitespace() {
	cfgElem := reflect.ValueOf(cfg).Elem()
	cfgStructField := reflect.Indirect(reflect.ValueOf(cfg)).Type()

	for i := 0; i < cfgElem.NumField(); i++ {
		cfgField := cfgElem.Field(i)
		if !cfgField.CanInterface() {
			continue
		}
		trimTag := cfgStructField.Field(i).Tag.Get("trim")
		if len(trimTag) == 0 {
			continue
		}

		if cfgField.Kind() != reflect.String {
			seelog.Warnf("Cannot trim non-string field type %v index %v", cfgField.Kind().String(), i)
			continue
		}
		str := cfgField.Interface().(string)
		cfgField.SetString(strings.TrimSpace(str))
	}
}

func fileConfig() (Config, error) {
	config_file := utils.DefaultIfBlank(os.Getenv("ECS_AGENT_CONFIG_FILE_PATH"), "/etc/ecs_container_agent/config.json")
	config := Config{}

	file, err := os.Open(config_file)
	if err != nil {
		return config, nil
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		seelog.Errorf("Unable to read config file, err %v", err)
		return config, err
	}
	if strings.TrimSpace(string(data)) == "" {
		// empty file, not an error
		return config, nil
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		seelog.Errorf("Error reading config json data, err %v", err)
	}

	// Handle any deprecated keys correctly here
	if utils.ZeroOrNil(config.Cluster) && !utils.ZeroOrNil(config.ClusterArn) {
		config.Cluster = config.ClusterArn
	}
	return config, nil
}

// environmentConfig reads the given configs from the environment and attempts
// to convert them to the given type
func environmentConfig() (Config, error) {
	var errs []error
	endpoint := os.Getenv("ECS_BACKEND_HOST")

	clusterRef := os.Getenv("ECS_CLUSTER")
	awsRegion := os.Getenv("AWS_DEFAULT_REGION")

	dockerEndpoint := os.Getenv("DOCKER_HOST")
	engineAuthType := os.Getenv("ECS_ENGINE_AUTH_TYPE")
	engineAuthData := os.Getenv("ECS_ENGINE_AUTH_DATA")

	var checkpoint bool
	dataDir := os.Getenv("ECS_DATADIR")
	if dataDir != "" {
		// if we have a directory to checkpoint to, default it to be on
		checkpoint = utils.ParseBool(os.Getenv("ECS_CHECKPOINT"), true)
	} else {
		// if the directory is not set, default to checkpointing off for
		// backwards compatibility
		checkpoint = utils.ParseBool(os.Getenv("ECS_CHECKPOINT"), false)
	}

	// Format: json array, e.g. [1,2,3]
	reservedPortEnv := os.Getenv("ECS_RESERVED_PORTS")
	portDecoder := json.NewDecoder(strings.NewReader(reservedPortEnv))
	var reservedPorts []uint16
	err := portDecoder.Decode(&reservedPorts)
	// EOF means the string was blank as opposed to UnexepctedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		err := fmt.Errorf("Invalid format for \"ECS_RESERVED_PORTS\" environment variable; expected a JSON array like [1,2,3]. err %v", err)
		seelog.Warn(err)
	}

	reservedPortUDPEnv := os.Getenv("ECS_RESERVED_PORTS_UDP")
	portDecoderUDP := json.NewDecoder(strings.NewReader(reservedPortUDPEnv))
	var reservedPortsUDP []uint16
	err = portDecoderUDP.Decode(&reservedPortsUDP)
	// EOF means the string was blank as opposed to UnexepctedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		err := fmt.Errorf("Invalid format for \"ECS_RESERVED_PORTS_UDP\" environment variable; expected a JSON array like [1,2,3]. err %v", err)
		seelog.Warn(err)
	}

	updateDownloadDir := os.Getenv("ECS_UPDATE_DOWNLOAD_DIR")
	updatesEnabled := utils.ParseBool(os.Getenv("ECS_UPDATES_ENABLED"), false)

	disableMetrics := utils.ParseBool(os.Getenv("ECS_DISABLE_METRICS"), false)

	reservedMemory := parseEnvVariableUint16("ECS_RESERVED_MEMORY")

	var dockerStopTimeout time.Duration
	parsedStopTimeout := parseEnvVariableDuration("ECS_CONTAINER_STOP_TIMEOUT")
	if parsedStopTimeout >= minimumDockerStopTimeout {
		dockerStopTimeout = parsedStopTimeout
	} else if parsedStopTimeout != 0 {
		seelog.Warnf("Discarded invalid value for docker stop timeout, parsed as: %v", parsedStopTimeout)
	}

	taskCleanupWaitDuration := parseEnvVariableDuration("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")
	availableLoggingDriversEnv := os.Getenv("ECS_AVAILABLE_LOGGING_DRIVERS")
	loggingDriverDecoder := json.NewDecoder(strings.NewReader(availableLoggingDriversEnv))
	var availableLoggingDrivers []dockerclient.LoggingDriver
	err = loggingDriverDecoder.Decode(&availableLoggingDrivers)
	// EOF means the string was blank as opposed to UnexepctedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		err := fmt.Errorf("Invalid format for \"ECS_AVAILABLE_LOGGING_DRIVERS\" environment variable; expected a JSON array like [\"json-file\",\"syslog\"]. err %v", err)
		seelog.Warn(err)
	}

	privilegedDisabled := utils.ParseBool(os.Getenv("ECS_DISABLE_PRIVILEGED"), false)
	seLinuxCapable := utils.ParseBool(os.Getenv("ECS_SELINUX_CAPABLE"), false)
	appArmorCapable := utils.ParseBool(os.Getenv("ECS_APPARMOR_CAPABLE"), false)
	taskIAMRoleEnabled := utils.ParseBool(os.Getenv("ECS_ENABLE_TASK_IAM_ROLE"), false)
	taskIAMRoleEnabledForNetworkHost := utils.ParseBool(os.Getenv("ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST"), false)

	credentialsAuditLogFile := os.Getenv("ECS_AUDIT_LOGFILE")
	credentialsAuditLogDisabled := utils.ParseBool(os.Getenv("ECS_AUDIT_LOGFILE_DISABLED"), false)

	imageCleanupDisabled := utils.ParseBool(os.Getenv("ECS_DISABLE_IMAGE_CLEANUP"), false)
	minimumImageDeletionAge := parseEnvVariableDuration("ECS_IMAGE_MINIMUM_CLEANUP_AGE")
	imageCleanupInterval := parseEnvVariableDuration("ECS_IMAGE_CLEANUP_INTERVAL")
	numImagesToDeletePerCycleEnvVal := os.Getenv("ECS_NUM_IMAGES_DELETE_PER_CYCLE")
	numImagesToDeletePerCycle, err := strconv.Atoi(numImagesToDeletePerCycleEnvVal)
	if numImagesToDeletePerCycleEnvVal != "" && err != nil {
		seelog.Warnf("Invalid format for \"ECS_NUM_IMAGES_DELETE_PER_CYCLE\", expected an integer. err %v", err)
	}

	instanceAttributesEnv := os.Getenv("ECS_INSTANCE_ATTRIBUTES")
	attributeDecoder := json.NewDecoder(strings.NewReader(instanceAttributesEnv))
	var instanceAttributes map[string]string

	err = attributeDecoder.Decode(&instanceAttributes)
	if err != io.EOF && err != nil {
		err := fmt.Errorf("Invalid format for ECS_INSTANCE_ATTRIBUTES. Expected a json hash")
		seelog.Warn(err)
		errs = append(errs, err)
	}
	for attributeKey, attributeValue := range instanceAttributes {
		seelog.Debugf("Setting instance attribute %v: %v", attributeKey, attributeValue)
	}

	if len(errs) > 0 {
		err = utils.NewMultiError(errs...)
	} else {
		err = nil
	}
	return Config{
		Cluster:                          clusterRef,
		APIEndpoint:                      endpoint,
		AWSRegion:                        awsRegion,
		DockerEndpoint:                   dockerEndpoint,
		ReservedPorts:                    reservedPorts,
		ReservedPortsUDP:                 reservedPortsUDP,
		DataDir:                          dataDir,
		Checkpoint:                       checkpoint,
		EngineAuthType:                   engineAuthType,
		EngineAuthData:                   NewSensitiveRawMessage([]byte(engineAuthData)),
		UpdatesEnabled:                   updatesEnabled,
		UpdateDownloadDir:                updateDownloadDir,
		DisableMetrics:                   disableMetrics,
		ReservedMemory:                   reservedMemory,
		AvailableLoggingDrivers:          availableLoggingDrivers,
		PrivilegedDisabled:               privilegedDisabled,
		SELinuxCapable:                   seLinuxCapable,
		AppArmorCapable:                  appArmorCapable,
		TaskCleanupWaitDuration:          taskCleanupWaitDuration,
		TaskIAMRoleEnabled:               taskIAMRoleEnabled,
		DockerStopTimeout:                dockerStopTimeout,
		CredentialsAuditLogFile:          credentialsAuditLogFile,
		CredentialsAuditLogDisabled:      credentialsAuditLogDisabled,
		TaskIAMRoleEnabledForNetworkHost: taskIAMRoleEnabledForNetworkHost,
		ImageCleanupDisabled:             imageCleanupDisabled,
		MinimumImageDeletionAge:          minimumImageDeletionAge,
		ImageCleanupInterval:             imageCleanupInterval,
		NumImagesToDeletePerCycle:        numImagesToDeletePerCycle,
		InstanceAttributes:               instanceAttributes,
	}, err
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

func ec2MetadataConfig(ec2client ec2.EC2MetadataClient) Config {
	iid, err := ec2client.InstanceIdentityDocument()
	if err != nil {
		seelog.Criticalf("Unable to communicate with EC2 Metadata service to infer region: %v", err.Error())
		return Config{}
	}
	return Config{AWSRegion: iid.Region}
}

// NewConfig returns a config struct created by merging environment variables,
// a config file, and EC2 Metadata info.
// The 'config' struct it returns can be used, even if an error is returned. An
// error is returned, however, if the config is incomplete in some way that is
// considered fatal.
func NewConfig(ec2client ec2.EC2MetadataClient) (config *Config, err error) {
	var errs []error
	var errTmp error
	envConfig, errTmp := environmentConfig() //Environment overrides all else
	if errTmp != nil {
		errs = append(errs, errTmp)
	}
	config = &envConfig
	defer func() {
		config.trimWhitespace()
		config.Merge(DefaultConfig())
		errTmp = config.validateAndOverrideBounds()
		if errTmp != nil {
			errs = append(errs, errTmp)
		}
		if len(errs) != 0 {
			err = utils.NewMultiError(errs...)
		} else {
			err = nil
		}
	}()

	if config.complete() {
		// No need to do file / network IO
		return config, nil
	}

	fcfg, errTmp := fileConfig()
	if errTmp != nil {
		errs = append(errs, errTmp)
	}
	config.Merge(fcfg)

	if config.AWSRegion == "" {
		// Get it from metadata only if we need to (network io)
		config.Merge(ec2MetadataConfig(ec2client))
	}

	return config, err
}

// validateAndOverrideBounds performs validation over members of the Config struct
// and check the value against the minimum required value.
func (config *Config) validateAndOverrideBounds() error {
	err := config.checkMissingAndDepreciated()
	if err != nil {
		return err
	}

	if config.DockerStopTimeout < minimumDockerStopTimeout {
		return fmt.Errorf("Invalid negative DockerStopTimeout: %v", config.DockerStopTimeout.String())
	}
	var badDrivers []string
	for _, driver := range config.AvailableLoggingDrivers {
		_, ok := dockerclient.LoggingDriverMinimumVersion[driver]
		if !ok {
			badDrivers = append(badDrivers, string(driver))
		}
	}
	if len(badDrivers) > 0 {
		return errors.New("Invalid logging drivers: " + strings.Join(badDrivers, ", "))
	}

	// If a value has been set for taskCleanupWaitDuration and the value is less than the minimum allowed cleanup duration,
	// print a warning and override it
	if config.TaskCleanupWaitDuration < minimumTaskCleanupWaitDuration {
		seelog.Warnf("Invalid value for image cleanup duration, will be overridden with the default value: %s. Parsed value: %v, minimum value: %v.", DefaultTaskCleanupWaitDuration.String(), config.TaskCleanupWaitDuration, minimumTaskCleanupWaitDuration)
		config.TaskCleanupWaitDuration = DefaultTaskCleanupWaitDuration
	}

	if config.ImageCleanupInterval < minimumImageCleanupInterval {
		seelog.Warnf("Invalid value for image cleanup duration, will be overridden with the default value: %s. Parsed value: %v, minimum value: %v.", DefaultImageCleanupTimeInterval.String(), config.ImageCleanupInterval, minimumImageCleanupInterval)
		config.ImageCleanupInterval = DefaultImageCleanupTimeInterval
	}

	if config.NumImagesToDeletePerCycle < minimumNumImagesToDeletePerCycle {
		seelog.Warnf("Invalid value for number of images to delete for image cleanup, will be overriden with the default value: %d. Parsed value: %d, minimum value: %d.", DefaultImageDeletionAge, config.NumImagesToDeletePerCycle, minimumNumImagesToDeletePerCycle)
		config.NumImagesToDeletePerCycle = DefaultNumImagesToDeletePerCycle
	}

	config.platformOverrides()

	return nil
}

// String returns a lossy string representation of the config suitable for human readable display.
// Consequently, it *should not* return any sensitive information.
func (config *Config) String() string {
	return fmt.Sprintf("Cluster: %v, Region: %v, DataDir: %v, Checkpoint: %v, AuthType: %v, UpdatesEnabled: %v, DisableMetrics: %v, ReservedMem: %v, TaskCleanupWaitDuration: %v, DockerStopTimeout: %v", config.Cluster, config.AWSRegion, config.DataDir, config.Checkpoint, config.EngineAuthType, config.UpdatesEnabled, config.DisableMetrics, config.ReservedMemory, config.TaskCleanupWaitDuration, config.DockerStopTimeout)
}
