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

// Package dockeriface contains an interface for go-dockerclient matching the
// subset used by the agent
package dockeriface

import docker "github.com/fsouza/go-dockerclient"

// Client is an interface specifying the subset of
// github.com/fsouza/go-dockerclient.Client that the agent uses.
type Client interface {
	AddEventListener(listener chan<- *docker.APIEvents) error
	CreateContainer(opts docker.CreateContainerOptions) (*docker.Container, error)
	ImportImage(opts docker.ImportImageOptions) error
	InspectContainer(id string) (*docker.Container, error)
	InspectImage(name string) (*docker.Image, error)
	ListContainers(opts docker.ListContainersOptions) ([]docker.APIContainers, error)
	Ping() error
	PullImage(opts docker.PullImageOptions, auth docker.AuthConfiguration) error
	RemoveContainer(opts docker.RemoveContainerOptions) error
	RemoveEventListener(listener chan *docker.APIEvents) error
	StartContainer(id string, hostConfig *docker.HostConfig) error
	StopContainer(id string, timeout uint) error
	Stats(opts docker.StatsOptions) error
	Version() (*docker.Env, error)
	RemoveImage(imageName string) error
}
