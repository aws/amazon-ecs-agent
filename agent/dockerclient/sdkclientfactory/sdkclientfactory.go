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
	// if the client version returns a MinAPIVersion, use it to return all the
	// Docker clients between MinAPIVersion and APIVersion, else try pinging
	// the clients in getKnownAPIVersions
	var minAPIVersion, apiVersion string
	// get a Docker client with the default supported version
	client, err := newVersionedClient(endpoint, string(minDockerAPIVersion))
	if err == nil {
		derivedCtx, cancel := context.WithTimeout(ctx, dockerclient.VersionTimeout)
		defer cancel()
		serverVersion, err := client.ServerVersion(derivedCtx)
		if err == nil {
			apiVersion = serverVersion.APIVersion
			// check if the docker.Version has MinAPIVersion value
			if serverVersion.MinAPIVersion != "" {
				minAPIVersion = serverVersion.MinAPIVersion
			}
		}
	}
	clients := make(map[dockerclient.DockerVersion]sdkclient.Client)
	for _, version := range dockerclient.GetKnownAPIVersions() {
		dockerClient, err := getDockerClientForVersion(endpoint, string(version), minAPIVersion, apiVersion, ctx)
		if err != nil {
			log.Infof("Unable to get Docker client for version %s: %v", version, err)
			continue
		}
		clients[version] = dockerClient
	}
	return clients
}

func getDockerClientForVersion(
	endpoint string,
	version string,
	minAPIVersion string,
	apiVersion string,
	derivedCtx context.Context) (sdkclient.Client, error) {
	if minAPIVersion != "" && apiVersion != "" {
		lessThanMinCheck := "<" + minAPIVersion
		moreThanMaxCheck := ">" + apiVersion
		minVersionCheck, err := dockerclient.DockerAPIVersion(version).Matches(lessThanMinCheck)
		if err != nil {
			return nil, errors.Wrapf(err, "version detection using MinAPIVersion: unable to get min version: %s", minAPIVersion)
		}
		maxVersionCheck, err := dockerclient.DockerAPIVersion(version).Matches(moreThanMaxCheck)
		if err != nil {
			return nil, errors.Wrapf(err, "version detection using MinAPIVersion: unable to get max version: %s", apiVersion)
		}
		// Do not add the version when it is outside minVersion to maxVersion
		if minVersionCheck || maxVersionCheck {
			return nil, errors.Errorf("version detection using MinAPIVersion: unsupported version: %s", version)
		}
	}
	client, err := newVersionedClient(endpoint, string(version))
	if err != nil {
		return nil, errors.Wrapf(err, "version detection check: unable to create Docker client for version: %s", version)
	}
	_, err = client.Ping(derivedCtx)
	if err != nil {
		return nil, errors.Wrapf(err, "version detection check: failed to ping with Docker version: %s", string(version))
	}
	return client, nil
}
