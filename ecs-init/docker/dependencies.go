// Copyright 2015-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package docker

//go:generate mockgen.sh $GOPACKAGE $GOFILE

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/amazon-ecs-agent/ecs-init/backoff"
	"github.com/aws/amazon-ecs-agent/ecs-init/config"

	log "github.com/cihub/seelog"
	godocker "github.com/fsouza/go-dockerclient"
)

const (
	// dockerClientAPIVersion specifies the minimum docker client API version
	// required by ECS Init
	// Version 1.25 is required for setting Init to true when constructing
	// the HostConfig for creating the ECS Agent to enable the Task
	// Networking with ENI capability
	dockerClientAPIVersion = "1.25"
)

type dockerclient interface {
	ListImages(opts godocker.ListImagesOptions) ([]godocker.APIImages, error)
	LoadImage(opts godocker.LoadImageOptions) error
	Logs(opts godocker.LogsOptions) error
	ListContainers(opts godocker.ListContainersOptions) ([]godocker.APIContainers, error)
	RemoveContainer(opts godocker.RemoveContainerOptions) error
	CreateContainer(opts godocker.CreateContainerOptions) (*godocker.Container, error)
	StartContainer(id string, hostConfig *godocker.HostConfig) error
	WaitContainer(id string) (int, error)
	StopContainer(id string, timeout uint) error
	Ping() error
}

type _dockerclient struct {
	docker dockerclient
}

type dockerClientFactory interface {
	NewVersionedClient(endpoint string, apiVersionString string) (dockerclient, error)
}

type godockerClientFactory struct{}

func (client godockerClientFactory) NewVersionedClient(endpoint string, apiVersionString string) (dockerclient, error) {
	return godocker.NewVersionedClient(endpoint, apiVersionString)
}

func newDockerClient(dockerClientFactory dockerClientFactory, pingBackoff backoff.Backoff) (dockerclient, error) {
	dockerUnixSocketSourcePath, fromEnv := config.DockerUnixSocket()
	if !fromEnv {
		dockerUnixSocketSourcePath = "/var/run/docker.sock"
	}
	client, err := dockerClientFactory.NewVersionedClient(
		config.UnixSocketPrefix+dockerUnixSocketSourcePath, dockerClientAPIVersion)
	if err != nil {
		return nil, err
	}
	for {
		err = client.Ping()
		if err == nil {
			break
		}
		shouldRetry := (isNetworkError(err) || isRetryablePingError(err)) && pingBackoff.ShouldRetry()
		if !shouldRetry {
			break
		}
		backoffDuration := pingBackoff.Duration()
		log.Infof("Error connecting to docker, backing off for %s, error: %s", backoffDuration, err)
		time.Sleep(backoffDuration)
	}
	return &_dockerclient{
		docker: client,
	}, err
}

func (d *_dockerclient) ListImages(opts godocker.ListImagesOptions) ([]godocker.APIImages, error) {
	return d.docker.ListImages(opts)
}

func (d *_dockerclient) LoadImage(opts godocker.LoadImageOptions) error {
	return d.docker.LoadImage(opts)
}

func (d *_dockerclient) Logs(opts godocker.LogsOptions) error {
	return d.docker.Logs(opts)
}

func (d *_dockerclient) ListContainers(opts godocker.ListContainersOptions) ([]godocker.APIContainers, error) {
	return d.docker.ListContainers(opts)
}

func (d *_dockerclient) RemoveContainer(opts godocker.RemoveContainerOptions) error {
	return d.docker.RemoveContainer(opts)
}

func (d *_dockerclient) CreateContainer(opts godocker.CreateContainerOptions) (*godocker.Container, error) {
	return d.docker.CreateContainer(opts)
}

func (d *_dockerclient) StartContainer(id string, hostConfig *godocker.HostConfig) error {
	return d.docker.StartContainer(id, hostConfig)
}

func (d *_dockerclient) WaitContainer(id string) (int, error) {
	return d.docker.WaitContainer(id)
}

func (d *_dockerclient) StopContainer(id string, timeout uint) error {
	return d.docker.StopContainer(id, timeout)
}

func (d *_dockerclient) Ping() error {
	return d.docker.Ping()
}

type fileSystem interface {
	ReadFile(filename string) ([]byte, error)
}

type _standardFS struct{}

var standardFS = &_standardFS{}

func (s *_standardFS) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func isNetworkError(err error) bool {
	wrapped, isWrapped := err.(*url.Error)
	if isWrapped {
		_, ok := wrapped.Err.(*net.OpError)
		return ok
	}
	return false
}

func isRetryablePingError(err error) bool {
	godockerError, ok := err.(*godocker.Error)
	if ok {
		return godockerError.Status != http.StatusOK
	}
	return false
}
