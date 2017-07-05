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

package containermetadata

import (
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"

	docker "github.com/fsouza/go-dockerclient"
)

// dockerDummyClient is a wrapper for the docker interface functions we need
// We use this as a dummy type to be able to pass in engine.DockerClient to
// our functions without creating import cycles
type dockerDummyClient interface {
	InspectContainer(string, time.Duration) (*docker.Container, error)
	Version() (string, error)
}

// NetworkMetadata keeps track of the data we parse from the Network Settings
// in docker containers
type NetworkMetadata struct {
	ports       []api.PortBinding
	networkMode string
	gateway     string
	iPAddress   string
	iPv6Gateway string
}

// DockerContainerMD keeps track of all metadata acquired from Docker inspection
// Has redundancies with engine.DockerContainerMetadata but this packages all
// docker metadata we want in the service so we can change features easily
type DockerContainerMD struct {
	status        string
	containerID   string
	containerName string
	imageID       string
	imageName     string
	networkInfo   NetworkMetadata
}

// TaskMetadata keeps track of all metadata associated with a task
// provided by AWS, does not depend on the creation of the container
type TaskMetadata struct {
	version           string
	clusterArn        string
	containerInstance string
	taskArn           string
}

// Metadata packages all acquired metadata and is used to format it
// into JSON to write to the metadata file
type Metadata struct {
	Version           string            `json:"DockerVersion, omitempty"`
	Status            string            `json:"Status, omitempty"`
	ClusterArn        string            `json:"ClusterArn, omitempty"`
	ContainerInstance string            `json:"ContainerInstanceArn, omitempty"`
	TaskArn           string            `json:"TaskArn, omitempty"`
	ContainerID       string            `json:"ContainerID, omitempty"`
	ContainerName     string            `json:"ContainerName, omitempty"`
	ImageID           string            `json:"ImageID, omitempty"`
	ImageName         string            `json:"ImageName, omitempty"`
	Ports             []api.PortBinding `json:"PortMappings, omitempty"`
	NetworkMode       string            `json:"NetworkMode, omitempty"`
	Gateway           string            `json:"Gateway, omitempty"`
	IPAddress         string            `json:"IPAddress, omitempty"`
	IPv6Gateway       string            `json:"IPv6Gateway, omitempty"`
}

/*
func (m *Metadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Version           string            `json:"DockerVersion, omitempty"`
		Status            string            `json:"Status, omitempty"`
		ClusterArn        string            `json:"ClusterArn, omitempty"`
		ContainerInstance string            `json:"ContainerInstanceArn, omitempty"`
		TaskArn           string            `json:"TaskArn, omitempty"`
		ContainerID       string            `json:"ContainerID, omitempty"`
		ContainerName     string            `json:"ContainerName, omitempty"`
		ImageID           string            `json:"ImageID, omitempty"`
		ImageName         string            `json:"ImageName, omitempty"`
		Ports             []api.PortBinding `json:"PortMappings, omitempty"`
		NetworkMode       string            `json:"NetworkMode, omitempty"`
		Gateway           string            `json:"Gateway, omitempty"`
		IPAddress         string            `json:"IPAddress, omitempty"`
		IPv6Gateway       string            `json:"IPv6Gateway, omitempty"`
	}{
		Version:           m.version,
		Status:            m.status,
		ClusterArn:        m.clusterArn,
		ContainerInstance: m.containerInstance,
		TaskArn:           m.taskArn,
		ImageName:         m.imageName,
		ImageID:           m.imageID,
		ContainerName:     m.containerName,
		ContainerID:       m.containerID,
		Ports:             m.ports,
		NetworkMode:       m.networkMode,
		Gateway:           m.gateway,
		IPAddress:         m.iPAddress,
		IPv6Gateway:       m.iPv6Gateway,
	})
}*/
