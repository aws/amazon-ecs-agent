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

package dockerclient

import (
	"errors"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface"
	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

// Factory provides a collection of docker remote clients that include a
// recommended client version as well as a set of alternative supported
// docker clients.
type Factory interface {
	// GetDefaultClient returns a versioned client for the default version
	GetDefaultClient() (dockeriface.Client, error)

	// GetClient returns a client with the specified version or an error
	// if the client doesn't exist.
	GetClient(version DockerVersion) (dockeriface.Client, error)

	// FindSupportedAPIVersions tests each agent supported API version
	// against the Docker daemon and returns a slice of supported
	// versions. The slice will always be a subset (non-strict)
	// of agent supported versions.
	FindSupportedAPIVersions() []DockerVersion
}

type factory struct {
	endpoint string
	clients  map[DockerVersion]dockeriface.Client
}

// newVersionedClient is a variable such that the implementation can be
// swapped out for unit tests
var newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
	return docker.NewVersionedClient(endpoint, version)
}

// NewFactory initializes a client factory using a specified endpoint.
func NewFactory(endpoint string) Factory {
	return &factory{
		endpoint: endpoint,
		clients:  findDockerVersions(endpoint),
	}
}

func (f *factory) GetDefaultClient() (dockeriface.Client, error) {
	return f.GetClient(getDefaultVersion())
}

func (f *factory) FindSupportedAPIVersions() []DockerVersion {
	var supportedVersions []DockerVersion
	for _, agentVersion := range getAgentVersions() {
		_, err := f.GetClient(agentVersion)
		if err != nil {
			continue
		}
		supportedVersions = append(supportedVersions, agentVersion)
	}
	return supportedVersions
}

// getClient returns a client specified by the docker version. Its wrapped
// by GetClient so that it can do platform-specific magic
func (f *factory) getClient(version DockerVersion) (dockeriface.Client, error) {
	client, ok := f.clients[version]
	if ok {
		return client, nil
	} else {
		return nil, errors.New("docker client factory: client not found for docker version: " + string(version))
	}
}

// findDockerVersions loops over all known API versions and finds which ones
// are supported by the docker daemon on the host
func findDockerVersions(endpoint string) map[DockerVersion]dockeriface.Client {
	clients := make(map[DockerVersion]dockeriface.Client)
	for _, version := range getKnownAPIVersions() {
		client, err := newVersionedClient(endpoint, string(version))
		if err != nil {
			log.Infof("Error while creating client: %v", err)
			continue
		}

		err = client.Ping()
		if err != nil {
			log.Infof("Failed to ping with Docker version %s: %v", version, err)
		}
		clients[version] = client
	}
	return clients
}
