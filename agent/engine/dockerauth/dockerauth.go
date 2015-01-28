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

// Package dockerauth handles storing auth configuration information for Docker
// registries
package dockerauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/logger"
	registryauth "github.com/docker/docker/registry"
	docker "github.com/fsouza/go-dockerclient"
)

var log = logger.ForModule("docker auth")

var cfg *config.Config

// SetConfig sets the config struct to read Auth from
func SetConfig(conf *config.Config) {
	cfg = conf
}

// GetAuthconfig retrieves Docker auth information from the previously set
// config struct
func GetAuthconfig(hostname string) docker.AuthConfiguration {
	switch cfg.EngineAuthType {
	case "dockercfg":
		return getDockerCfgConfig(hostname)
	case "docker":
		return getDockerConfig(hostname)
	case "":
		return docker.AuthConfiguration{}
	default:
		log.Error("Unrecognized AuthType type", "type", cfg.EngineAuthType)
	}
	return docker.AuthConfiguration{}
}

func getDockerConfig(hostname string) docker.AuthConfiguration {
	dockerConfigs := make(map[string]registryauth.AuthConfig)
	err := json.Unmarshal(cfg.EngineAuthData, &dockerConfigs)
	if err != nil {
		log.Error("Error decoding auth configuration", "err", err, "type", "docker", "host", hostname)
		return docker.AuthConfiguration{}
	}
	tmpConfig := &registryauth.ConfigFile{Configs: dockerConfigs}
	tmpAuthConfig := tmpConfig.ResolveAuthConfig(hostname)

	return docker.AuthConfiguration{
		Username:      tmpAuthConfig.Username,
		Password:      tmpAuthConfig.Password,
		Email:         tmpAuthConfig.Email,
		ServerAddress: tmpAuthConfig.ServerAddress,
	}
}

func getDockerCfgConfig(hostname string) docker.AuthConfiguration {
	tmpConfig, err := loadConfig(cfg.EngineAuthData)
	if err != nil {
		log.Error("Error decoding auth configuration", "err", err, "type", "dockercfg", "host", hostname)
		return docker.AuthConfiguration{}
	}
	tmpAuthConfig := tmpConfig.ResolveAuthConfig(hostname)
	return docker.AuthConfiguration{
		Username:      tmpAuthConfig.Username,
		Password:      tmpAuthConfig.Password,
		Email:         tmpAuthConfig.Email,
		ServerAddress: tmpAuthConfig.ServerAddress,
	}
}

// The below code is taken, almost in its entirety, from
// https://github.com/docker/docker/blob/3837c080228143378b1041364a90aa5bcf59fa00/registry/auth.go#L70
// and is copyright Docker Inc. Please see the LICENSE file in the root of this
// repository.
func loadConfig(b []byte) (*registryauth.ConfigFile, error) {
	configFile := registryauth.ConfigFile{Configs: make(map[string]registryauth.AuthConfig)}

	if err := json.Unmarshal(b, &configFile.Configs); err != nil {
		arr := strings.Split(string(b), "\n")
		if len(arr) < 2 {
			return &configFile, fmt.Errorf("The Auth config file is empty")
		}
		authConfig := registryauth.AuthConfig{}
		origAuth := strings.Split(arr[0], " = ")
		if len(origAuth) != 2 {
			return &configFile, fmt.Errorf("Invalid Auth config file")
		}
		authConfig.Username, authConfig.Password, err = decodeAuth(origAuth[1])
		if err != nil {
			return &configFile, err
		}
		origEmail := strings.Split(arr[1], " = ")
		if len(origEmail) != 2 {
			return &configFile, fmt.Errorf("Invalid Auth config file")
		}
		authConfig.Email = origEmail[1]
		authConfig.ServerAddress = registryauth.IndexServerAddress()
		// *TODO: Switch to using IndexServerName() instead?
		configFile.Configs[registryauth.IndexServerAddress()] = authConfig
	} else {
		for k, authConfig := range configFile.Configs {
			authConfig.Username, authConfig.Password, err = decodeAuth(authConfig.Auth)
			if err != nil {
				return &configFile, err
			}
			authConfig.Auth = ""
			authConfig.ServerAddress = k
			configFile.Configs[k] = authConfig
		}
	}
	return &configFile, nil

}

// The below code is taken, in its entirety, from
// https://github.com/docker/docker/blob/3837c080228143378b1041364a90aa5bcf59fa00/registry/auth.go#L49
// and is copyright Docker Inc. Please see the LICENSE file in the root of this
// repository.
// decode the auth string
func decodeAuth(authStr string) (string, string, error) {
	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		return "", "", err
	}
	if n > decLen {
		return "", "", fmt.Errorf("Something went wrong decoding auth config")
	}
	arr := strings.SplitN(string(decoded), ":", 2)
	if len(arr) != 2 {
		return "", "", fmt.Errorf("Invalid auth configuration file")
	}
	password := strings.Trim(arr[1], "\x00")
	return arr[0], password, nil
}
