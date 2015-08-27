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

	log "github.com/cihub/seelog"
	docker "github.com/fsouza/go-dockerclient"
)

type dockerVersion string

const (
	version_1_17 dockerVersion = "1.17"
	version_1_18 dockerVersion = "1.18"
	version_1_19 dockerVersion = "1.19"
	version_1_20 dockerVersion = "1.20"

	defaultVersion = version_1_17
)

var supportedVersions []dockerVersion

func init() {
	supportedVersions = []dockerVersion{
		version_1_17,
		version_1_18,
		version_1_19,
		version_1_20,
	}
}

type Factory interface {
	// GetDefaultClient returns a versioned client for the default version
	GetDefaultClient() (Client, error)

	// GetClient returns a client with the specified version
	GetClient(version dockerVersion) (Client, error)

	// FindAvailableVersions tests each supported version and returns a slice
	// of available versions
	FindAvailableVersions() []dockerVersion
}

type factory struct {
	endpoint string
	lock     sync.Mutex
	clients  map[dockerVersion]Client
}

// newVersionedClient is a variable such that the implementation can be
// swapped out for unit tests
var newVersionedClient = func(endpoint, version string) (Client, error) {
	return docker.NewVersionedClient(endpoint, version)
}

func NewFactory(endpoint string) *factory {
	return &factory{
		endpoint: endpoint,
		clients:  make(map[dockerVersion]Client),
	}
}

func (f *factory) GetDefaultClient() (Client, error) {
	return f.GetClient(defaultVersion)
}

func (f *factory) GetClient(version dockerVersion) (Client, error) {
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

func (f *factory) FindAvailableVersions() []dockerVersion {
	var availableVersions []dockerVersion
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
