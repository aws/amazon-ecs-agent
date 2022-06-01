//go:build windows
// +build windows

// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
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
	credSpec, err := c.getCredentialSpec()
	if err != nil || credSpec == "" {
		return false
	}

	return true
}

// GetCredentialSpec is used to retrieve the current credentialspec resource
func (c *Container) GetCredentialSpec() (string, error) {
	return c.getCredentialSpec()
}

func (c *Container) getCredentialSpec() (string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.DockerConfig.HostConfig == nil {
		return "", errors.New("empty container hostConfig")
	}

	hostConfig := &dockercontainer.HostConfig{}
	err := json.Unmarshal([]byte(*c.DockerConfig.HostConfig), hostConfig)
	if err != nil || len(hostConfig.SecurityOpt) == 0 {
		return "", errors.New("unable to obtain security options from container hostConfig")
	}

	for _, opt := range hostConfig.SecurityOpt {
		if strings.HasPrefix(opt, "credentialspec") {
			return opt, nil
		}
	}

	return "", errors.New("unable to obtain credentialspec")
}
