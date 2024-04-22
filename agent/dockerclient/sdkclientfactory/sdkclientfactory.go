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

package sdkclientfactory

import (
	"context"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/dockerclient"
	"github.com/aws/amazon-ecs-agent/agent/dockerclient/sdkclient"
	log "github.com/cihub/seelog"
	docker "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

// Factory provides a collection of docker remote clients that include a
// recommended client version as well as a set of alternative supported
// docker clients.
type Factory interface {
	// GetDefaultClient returns a versioned client for the default version
	GetDefaultClient() (sdkclient.Client, error)

	// GetClient returns a client with the specified version or an error
	// if the client doesn't exist.
	GetClient(version dockerclient.DockerVersion) (sdkclient.Client, error)

	// FindSupportedAPIVersions returns a slice of agent-supported Docker API
	// versions. Versions are tested by making calls against the Docker daemon
	// and may occur either at Factory creation time or lazily upon invocation
	// of this function. The slice represents the intersection of
	// agent-supported versions and daemon-supported versions.
	FindSupportedAPIVersions() []dockerclient.DockerVersion

	// FindKnownAPIVersions returns a slice of Docker API versions that are
	// known to the Docker daemon. Versions are tested by making calls against
	// the Docker daemon and may occur either at Factory creation time or
	// lazily upon invocation of this function. The slice represents the
	// intersection of the API versions that the agent knows exist (but does
	// not necessarily fully support) and the versions that result in
	// successful responses by the Docker daemon.
	FindKnownAPIVersions() []dockerclient.DockerVersion

	// FindClientAPIVersion returns the client api version
	FindClientAPIVersion(sdkclient.Client) dockerclient.DockerVersion
}

type factory struct {
	endpoint string
	clients  map[dockerclient.DockerVersion]sdkclient.Client
}

// newVersionedClient is a variable such that the implementation can be
// swapped out for unit tests
var newVersionedClient = func(endpoint, version string) (sdkclient.Client, error) {
	return docker.NewClientWithOpts(docker.WithVersion(version), docker.WithHost(endpoint))
}

// NewFactory initializes a client factory using a specified endpoint.
func NewFactory(ctx context.Context, endpoint string) Factory {
	return &factory{
		endpoint: endpoint,
		clients:  findDockerVersions(ctx, endpoint),
	}
}

func (f *factory) GetDefaultClient() (sdkclient.Client, error) {
	return f.GetClient(GetDefaultVersion())
}

func (f *factory) FindSupportedAPIVersions() []dockerclient.DockerVersion {
	var supportedVersions []dockerclient.DockerVersion
	for _, testVersion := range getAgentSupportedDockerVersions() {
		_, err := f.GetClient(testVersion)
		if err != nil {
			continue
		}
		supportedVersions = append(supportedVersions, testVersion)
	}
	return supportedVersions
}

func (f *factory) FindKnownAPIVersions() []dockerclient.DockerVersion {
	var knownVersions []dockerclient.DockerVersion
	for _, testVersion := range dockerclient.GetKnownAPIVersions() {
		_, err := f.GetClient(testVersion)
		if err != nil {
			continue
		}
		knownVersions = append(knownVersions, testVersion)
	}
	return knownVersions
}

// FindClientAPIVersion returns the version of the client from the map
func (f *factory) FindClientAPIVersion(client sdkclient.Client) dockerclient.DockerVersion {
	return dockerclient.DockerVersion(client.ClientVersion())
}

// getClient returns a client specified by the docker version. It's wrapped
// by GetClient so that it can do platform-specific magic.
func (f *factory) getClient(version dockerclient.DockerVersion) (sdkclient.Client, error) {
	client, ok := f.clients[version]
	if !ok {
		return nil, errors.New("docker client factory: client not found for docker version: " + string(version))
	}
	return client, nil
}

// findDockerVersions loops over all known API versions and finds which ones
// are supported by the docker daemon on the host
func findDockerVersions(ctx context.Context, endpoint string) map[dockerclient.DockerVersion]sdkclient.Client {
	clients := make(map[dockerclient.DockerVersion]sdkclient.Client)
	var minVersion dockerclient.DockerVersion
	for _, version := range dockerclient.GetKnownAPIVersions() {
		dockerClient, err := newVersionedClient(endpoint, string(version))
		if err != nil {
			log.Infof("Unable to get Docker client for version %s: %v", version, err)
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		serverVer, err := dockerClient.ServerVersion(ctx)
		if err != nil {
			log.Infof("Unable to get Docker client for version %s: %v", version, err)
			continue
		}
		if version.Compare(dockerclient.DockerVersion(serverVer.APIVersion)) > 0 {
			// Required client API version exceeds server's API version
			log.Infof(
				"Unable to get Docker client for version %s: server's API version is lower at %s",
				version, serverVer.APIVersion)
			continue
		}
		clients[version] = dockerClient
		if minVersion == "" {
			minVersion = version
		}
	}
	if minVersion.Compare(dockerclient.MinDockerAPIVersion) == 1 {
		// If the min supported docker api version is greater than the current min
		// version, then set the min version to this new value.
		// Only do it if greater to avoid "downgrading" the min version, such as
		// from 1.21 to 1.17.
		dockerclient.SetMinDockerAPIVersion(minVersion)
	}
	return clients
}
