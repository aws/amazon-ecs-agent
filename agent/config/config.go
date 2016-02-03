// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"github.com/aws/amazon-ecs-agent/agent/logger"
	"github.com/aws/amazon-ecs-agent/agent/utils"
)

var log = logger.ForModule("config")

const (
	// http://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml?search=docker
	DOCKER_RESERVED_PORT     = 2375
	DOCKER_RESERVED_SSL_PORT = 2376

	SSH_PORT = 22

	AGENT_INTROSPECTION_PORT = 51678

	DEFAULT_CLUSTER_NAME = "default"

	// DefaultTaskCleanupWaitDuration specifies the default value for task cleanup duration. It is used to
	// clean up task's containers.
	DefaultTaskCleanupWaitDuration = 3 * time.Hour

	// minimumTaskCleanupWaitDuration specifies the minimum duration to wait before cleaning up
	// a task's container. This is used to enforce sane values for the config.TaskCleanupWaitDuration field.
	minimumTaskCleanupWaitDuration = 1 * time.Minute
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
				log.Warn("Configuration key not set", "key", cfgStructField.Field(i).Name)
			case "fatal":
				log.Crit("Configuration key not set", "key", cfgStructField.Field(i).Name)
				fatalFields = append(fatalFields, cfgStructField.Field(i).Name)
			default:
				log.Warn("Unexpected `missing` tag value", "tag", missingTag)
			}
		} else {
			// present
			deprecatedTag := cfgStructField.Field(i).Tag.Get("deprecated")
			if len(deprecatedTag) == 0 {
				continue
			}
			log.Warn("Use of deprecated configuration key", "key", cfgStructField.Field(i).Name, "message", deprecatedTag)
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
			log.Warn("Cannot trim non-string field", "type", cfgField.Kind().String(), "index", i)
			continue
		}
		str := cfgField.Interface().(string)
		cfgField.SetString(strings.TrimSpace(str))
	}
}

func DefaultConfig() Config {
	return Config{
		DockerEndpoint:          "unix:///var/run/docker.sock",
		ReservedPorts:           []uint16{SSH_PORT, DOCKER_RESERVED_PORT, DOCKER_RESERVED_SSL_PORT, AGENT_INTROSPECTION_PORT},
		ReservedPortsUDP:        []uint16{},
		DataDir:                 "/data/",
		DisableMetrics:          false,
		DockerGraphPath:         "/var/lib/docker",
		ReservedMemory:          0,
		AvailableLoggingDrivers: []dockerclient.LoggingDriver{dockerclient.JsonFileDriver},
		TaskCleanupWaitDuration: DefaultTaskCleanupWaitDuration,
	}
}

func fileConfig() Config {
	config_file := utils.DefaultIfBlank(os.Getenv("ECS_AGENT_CONFIG_FILE_PATH"), "/etc/ecs_container_agent/config.json")

	file, err := os.Open(config_file)
	if err != nil {
		return Config{}
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Error("Unable to read config file", "err", err)
		return Config{}
	}
	if strings.TrimSpace(string(data)) == "" {
		// empty file, not an error
		return Config{}
	}

	config := Config{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		log.Error("Error reading config json data", "err", err)
	}

	// Handle any deprecated keys correctly here
	if utils.ZeroOrNil(config.Cluster) && !utils.ZeroOrNil(config.ClusterArn) {
		config.Cluster = config.ClusterArn
	}
	return config
}

// environmentConfig reads the given configs from the environment and attempts
// to convert them to the given type
func environmentConfig() Config {
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
		log.Warn("Invalid format for \"ECS_RESERVED_PORTS\" environment variable; expected a JSON array like [1,2,3].", "err", err)
	}

	reservedPortUDPEnv := os.Getenv("ECS_RESERVED_PORTS_UDP")
	portDecoderUDP := json.NewDecoder(strings.NewReader(reservedPortUDPEnv))
	var reservedPortsUDP []uint16
	err = portDecoderUDP.Decode(&reservedPortsUDP)
	// EOF means the string was blank as opposed to UnexepctedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		log.Warn("Invalid format for \"ECS_RESERVED_PORTS_UDP\" environment variable; expected a JSON array like [1,2,3].", "err", err)
	}

	updateDownloadDir := os.Getenv("ECS_UPDATE_DOWNLOAD_DIR")
	updatesEnabled := utils.ParseBool(os.Getenv("ECS_UPDATES_ENABLED"), false)

	disableMetrics := utils.ParseBool(os.Getenv("ECS_DISABLE_METRICS"), false)
	dockerGraphPath := os.Getenv("ECS_DOCKER_GRAPHPATH")

	reservedMemory := parseEnvVariableUint16("ECS_RESERVED_MEMORY")

	taskCleanupWaitDuration := parseEnvVariableDuration("ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION")
	availableLoggingDriversEnv := os.Getenv("ECS_AVAILABLE_LOGGING_DRIVERS")
	loggingDriverDecoder := json.NewDecoder(strings.NewReader(availableLoggingDriversEnv))
	var availableLoggingDrivers []dockerclient.LoggingDriver
	err = loggingDriverDecoder.Decode(&availableLoggingDrivers)
	// EOF means the string was blank as opposed to UnexepctedEof which means an
	// invalid parse
	// Blank is not a warning; we have sane defaults
	if err != io.EOF && err != nil {
		log.Warn("Invalid format for \"ECS_AVAILABLE_LOGGING_DRIVERS\" environment variable; expected a JSON array like [\"json-file\",\"syslog\"].", "err", err)
	}

	privilegedDisabled := utils.ParseBool(os.Getenv("ECS_DISABLE_PRIVILEGED"), false)
	seLinuxCapable := utils.ParseBool(os.Getenv("ECS_SELINUX_CAPABLE"), false)
	appArmorCapable := utils.ParseBool(os.Getenv("ECS_APPARMOR_CAPABLE"), false)

	return Config{
		Cluster:                 clusterRef,
		APIEndpoint:             endpoint,
		AWSRegion:               awsRegion,
		DockerEndpoint:          dockerEndpoint,
		ReservedPorts:           reservedPorts,
		ReservedPortsUDP:        reservedPortsUDP,
		DataDir:                 dataDir,
		Checkpoint:              checkpoint,
		EngineAuthType:          engineAuthType,
		EngineAuthData:          NewSensitiveRawMessage([]byte(engineAuthData)),
		UpdatesEnabled:          updatesEnabled,
		UpdateDownloadDir:       updateDownloadDir,
		DisableMetrics:          disableMetrics,
		DockerGraphPath:         dockerGraphPath,
		ReservedMemory:          reservedMemory,
		AvailableLoggingDrivers: availableLoggingDrivers,
		PrivilegedDisabled:      privilegedDisabled,
		SELinuxCapable:          seLinuxCapable,
		AppArmorCapable:         appArmorCapable,
		TaskCleanupWaitDuration: taskCleanupWaitDuration,
	}
}

func parseEnvVariableUint16(envVar string) uint16 {
	envVal := os.Getenv(envVar)
	var var16 uint16
	if envVal != "" {
		var64, err := strconv.ParseUint(envVal, 10, 16)
		if err != nil {
			log.Warn("Invalid format for \""+envVar+"\" environment variable; expected unsigned integer.", "err", err)
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
		log.Debug("Environment variable empty: " + envVar)
	} else {
		var err error
		duration, err = time.ParseDuration(envVal)
		if err != nil {
			log.Warn("Could not parse duration value: "+envVal+" for Environment Variable "+envVar+" : ", err)
		}
	}
	return duration
}

func ec2MetadataConfig(ec2client ec2.EC2MetadataClient) Config {
	iid, err := ec2client.InstanceIdentityDocument()
	if err != nil {
		log.Crit("Unable to communicate with EC2 Metadata service to infer region: " + err.Error())
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
	ctmp := environmentConfig() //Environment overrides all else
	config = &ctmp
	defer func() {
		config.trimWhitespace()
		err = config.validate()
		config.Merge(DefaultConfig())
	}()

	if config.complete() {
		// No need to do file / network IO
		return config, nil
	}

	config.Merge(fileConfig())

	if config.AWSRegion == "" {
		// Get it from metadata only if we need to (network io)
		config.Merge(ec2MetadataConfig(ec2client))
	}

	// If a value has been set for taskCleanupWaitDuration and the value is less than the minimum allowed cleanup duration,
	// print a warning and override it
	if config.TaskCleanupWaitDuration < minimumTaskCleanupWaitDuration {
		log.Warn("Invalid value for task cleanup duration, will be overridden to "+DefaultTaskCleanupWaitDuration.String(), "parsed value", config.TaskCleanupWaitDuration, "minimum threshold", minimumTaskCleanupWaitDuration)
		config.TaskCleanupWaitDuration = DefaultTaskCleanupWaitDuration
	}

	return config, err
}

// validate performs validation over members of the Config struct
func (config *Config) validate() error {
	err := config.checkMissingAndDepreciated()
	if err != nil {
		return err
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

	return nil
}

// String returns a lossy string representation of the config suitable for human readable display.
// Consequently, it *should not* return any sensitive information.
func (config *Config) String() string {
	return fmt.Sprintf("Cluster: %v, Region: %v, DataDir: %v, Checkpoint: %v, AuthType: %v, UpdatesEnabled: %v, DisableMetrics: %v, ReservedMem: %v, TaskCleanupWaitDuration: %v", config.Cluster, config.AWSRegion, config.DataDir, config.Checkpoint, config.EngineAuthType, config.UpdatesEnabled, config.DisableMetrics, config.ReservedMemory, config.TaskCleanupWaitDuration)
}
