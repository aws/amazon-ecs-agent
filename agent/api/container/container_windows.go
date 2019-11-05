// +build windows

// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package container

import (
	"encoding/json"
	"strings"

	"github.com/cihub/seelog"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/pkg/errors"
)

const (
	// DockerContainerMinimumMemoryInBytes is the minimum amount of
	// memory to be allocated to a docker container
	DockerContainerMinimumMemoryInBytes = 256 * 1024 * 1024 // 256MB
)

// RequiresCredentialSpec checks if container needs a credentialspec resource
func (c *Container) RequiresCredentialSpec() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return false
	}

	hostConfig := &dockercontainer.HostConfig{}
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil {
		seelog.Warnf("Encountered error when trying to get hostConfig for container %s: %v", err)
		return false
	}

	if len(hostConfig.SecurityOpt) == 0 {
		return false
	}

	for _, opt := range hostConfig.SecurityOpt {
		if strings.HasPrefix(opt, "credentialspec") {
			return true
		}
	}

	return false
}

// GetCredentialSpec is used to retrieve the current credentialspec resource
func (c *Container) GetCredentialSpec() (string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return "", errors.New("empty container hostConfig")
	}

	hostConfig := &dockercontainer.HostConfig{}
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil {
		seelog.Warnf("Encountered error when trying to get hostConfig for container %s: %v", err)
		return "", errors.New("unable to unmarshal container hostConfig")
	}

	if len(hostConfig.SecurityOpt) == 0 {
		return "", errors.New("unable to find container security options")
	}

	for _, opt := range hostConfig.SecurityOpt {
		if strings.HasPrefix(opt, "credentialspec") {
			return opt, nil
		}
	}

	return "", errors.New("unable to obtain credentialspec")
}
