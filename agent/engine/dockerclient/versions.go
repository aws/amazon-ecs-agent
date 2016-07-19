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

package dockerclient

import (
	"sync"

	"github.com/aws/amazon-ecs-agent/agent/engine/dockeriface"
	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

type DockerVersion string

const (
	Version_1_17 DockerVersion = "1.17"
	Version_1_18 DockerVersion = "1.18"
	Version_1_19 DockerVersion = "1.19"
	Version_1_20 DockerVersion = "1.20"
	Version_1_21 DockerVersion = "1.21"
	Version_1_22 DockerVersion = "1.22"
	Version_1_23 DockerVersion = "1.23"

	defaultVersion = Version_1_17
)

var supportedVersions []DockerVersion

func init() {
	supportedVersions = []DockerVersion{
		Version_1_17,
		Version_1_18,
		Version_1_19,
		Version_1_20,
		Version_1_21,
		Version_1_22,
		Version_1_23,
	}
}

type Factory interface {
	// GetDefaultClient returns a versioned client for the default version
	GetDefaultClient() (dockeriface.Client, error)

	// GetClient returns a client with the specified version
	GetClient(version DockerVersion) (dockeriface.Client, error)

	// FindAvailableVersions tests each supported version and returns a slice
	// of available versions
	FindAvailableVersions() []DockerVersion
}

type factory struct {
	endpoint string
	lock     sync.Mutex
	clients  map[DockerVersion]dockeriface.Client
}

// newVersionedClient is a variable such that the implementation can be
// swapped out for unit tests
var newVersionedClient = func(endpoint, version string) (dockeriface.Client, error) {
	return docker.NewVersionedClient(endpoint, version)
}

func NewFactory(endpoint string) Factory {
	return &factory{
		endpoint: endpoint,
		clients:  make(map[DockerVersion]dockeriface.Client),
	}
}

func (f *factory) GetDefaultClient() (dockeriface.Client, error) {
	return f.GetClient(defaultVersion)
}

func (f *factory) GetClient(version DockerVersion) (dockeriface.Client, error) {
	client, ok := f.clients[version]
	if ok {
		return client, nil
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	// double-check now that we're in a lock
	client, ok = f.clients[version]
	if ok {
		return client, nil
	}

	client, err := newVersionedClient(f.endpoint, string(version))
	if err != nil {
		return nil, err
	}

	err = client.Ping()
	if err != nil {
		return nil, err
	}

	f.clients[version] = client
	return client, nil
}

func (f *factory) FindAvailableVersions() []DockerVersion {
	var availableVersions []DockerVersion
	for _, version := range supportedVersions {
		_, err := f.GetClient(version)
		if err == nil {
			availableVersions = append(availableVersions, version)
		} else {
			log.Debugf("Failed to ping with Docker version %s: %v", version, err)
		}
	}
	log.Infof("Detected Docker versions %v", availableVersions)
	return availableVersions
}
