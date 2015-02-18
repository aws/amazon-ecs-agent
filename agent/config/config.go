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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/ec2"
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

// Complete returns true if all fields of the config are populated / nonzero
func (cfg *Config) Complete() bool {
	cfgElem := reflect.ValueOf(cfg).Elem()

	for i := 0; i < cfgElem.NumField(); i++ {
		if utils.ZeroOrNil(cfgElem.Field(i).Interface()) {
			return false
		}
	}
	return true
}

// CheckMissing checks all zero-valued fields for tags of the form
// missing:STRING and acts based on that string. Current options are: fatal,
// warn. Fatal will result in a fatal error, warn will result in a warning that
// the field is missing being logged
func (cfg *Config) CheckMissingAndDepreciated() {
	cfgElem := reflect.ValueOf(cfg).Elem()
	cfgStructField := reflect.Indirect(reflect.ValueOf(cfg)).Type()

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
				os.Exit(1)
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
}

func DefaultConfig() Config {
	awsRegion := "us-west-2"
	return Config{
		APIEndpoint:    ecsEndpoint(awsRegion),
		APIPort:        443,
		DockerEndpoint: "unix:///var/run/docker.sock",
		AWSRegion:      awsRegion,
		ReservedPorts:  []uint16{SSH_PORT, DOCKER_RESERVED_PORT, DOCKER_RESERVED_SSL_PORT, AGENT_INTROSPECTION_PORT},
		DataDir:        "/data/",
	}
}

func FileConfig() Config {
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

// EnvironmentConfig reads the given configs from the environment and attempts
// to convert them to the given type
func EnvironmentConfig() Config {
	endpoint := os.Getenv("ECS_BACKEND_HOST")
	port, _ := strconv.Atoi(os.Getenv("ECS_BACKEND_PORT"))

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

	return Config{
		Cluster:        clusterRef,
		APIEndpoint:    endpoint,
		APIPort:        uint16(port),
		AWSRegion:      awsRegion,
		DockerEndpoint: dockerEndpoint,
		ReservedPorts:  reservedPorts,
		DataDir:        dataDir,
		Checkpoint:     checkpoint,
		EngineAuthType: engineAuthType,
		EngineAuthData: []byte(engineAuthData),
	}
}

func EC2MetadataConfig() Config {
	iid, err := ec2.GetInstanceIdentityDocument()
	if err == nil {
		return Config{AWSRegion: iid.Region, APIEndpoint: ecsEndpoint(iid.Region)}
	}
	return Config{}
}

func ecsEndpoint(awsRegion string) string {
	endpoint := fmt.Sprintf("ecs.%s.amazonaws.com", awsRegion)
	return endpoint
}

func NewConfig() (*Config, error) {
	ctmp := EnvironmentConfig() //Environment overrides all else
	config := &ctmp
	defer func() {
		config.CheckMissingAndDepreciated()
		config.Merge(DefaultConfig())
	}()

	if config.Complete() {
		// No need to do file / network IO
		return config, nil
	}

	config.Merge(FileConfig())

	if config.AWSRegion == "" || config.APIEndpoint == "" {
		// Get it from metadata only if we need to (network io)
		config.Merge(EC2MetadataConfig())
	}

	return config, nil
}
