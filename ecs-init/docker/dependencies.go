// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"io/ioutil"

	"github.com/aws/amazon-ecs-init/ecs-init/config"

	godocker "github.com/fsouza/go-dockerclient"
)

type dockerclient interface {
	ListImages(opts godocker.ListImagesOptions) ([]godocker.APIImages, error)
	LoadImage(opts godocker.LoadImageOptions) error
	ListContainers(opts godocker.ListContainersOptions) ([]godocker.APIContainers, error)
	RemoveContainer(opts godocker.RemoveContainerOptions) error
	CreateContainer(opts godocker.CreateContainerOptions) (*godocker.Container, error)
	StartContainer(id string, hostConfig *godocker.HostConfig) error
	WaitContainer(id string) (int, error)
	StopContainer(id string, timeout uint) error
}

type _dockerclient struct {
	docker *godocker.Client
}

func newDockerClient() (*_dockerclient, error) {
	client, err := godocker.NewVersionedClient(config.UnixSocketPrefix+config.DockerUnixSocket(), "1.15")
	if err != nil {
		return nil, err
	}
	err = client.Ping()
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

type fileSystem interface {
	ReadFile(filename string) ([]byte, error)
}

type _standardFS struct{}

var standardFS = &_standardFS{}

func (s *_standardFS) ReadFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}
