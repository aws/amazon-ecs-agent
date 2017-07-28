// Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"encoding/json"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/api"

	docker "github.com/fsouza/go-dockerclient"
)

// dockerMetadataClient is a wrapper for the docker interface functions we need
// We use this as a dummy type to be able to pass in engine.DockerClient to
// our functions without creating import cycles
type dockerMetadataClient interface {
	InspectContainer(string, time.Duration) (*docker.Container, error)
}

// NetworkMetadata keeps track of the data we parse from the Network Settings
// in docker containers
type NetworkMetadata struct {
	networkMode string
	ipv4Address string
	ipv4Gateway string
	ipv6Address string
	ipv6Gateway string
}

// DockerContainerMD keeps track of all metadata acquired from Docker inspection
// Has redundancies with engine.DockerContainerMetadata but this packages all
// docker metadata we want in the service so we can change features easily
type DockerContainerMD struct {
	containerID         string
	dockerContainerName string
	imageID             string
	imageName           string
	ports               []api.PortBinding
	networkInfo         NetworkMetadata
}

// TaskMetadata keeps track of all metadata associated with a task
// provided by AWS, does not depend on the creation of the container
type TaskMetadata struct {
	containerName string
	cluster       string
	taskARN       string
}

// Metadata packages all acquired metadata and is used to format it
// into JSON to write to the metadata file. We have it flattened, rather
// than simply containing the previous three structures to simplify JSON
// parsing and avoid exposing those structs in the final metadata file.
type Metadata struct {
	taskMetadata            TaskMetadata
	dockerContainerMetadata DockerContainerMD
	containerInstanceARN    string
	createTime              time.Time
	updateTime              time.Time
}

func (m Metadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		struct {
			Cluster              string            `json:"Cluster,omitempty"`
			ContainerInstanceARN string            `json:"ContainerInstanceARN,omitempty"`
			TaskARN              string            `json:"TaskARN,omitempty"`
			ContainerID          string            `json:"ContainerID,omitempty"`
			ContainerName        string            `json:"ContainerName,omitempty"`
			DockerContainerName  string            `json:"DockerContainerName,omitempty"`
			ImageID              string            `json:"ImageID,omitempty"`
			ImageName            string            `json:"ImageName,omitempty"`
			Ports                []api.PortBinding `json:"PortMappings,omitempty"`
			NetworkMode          string            `json:"NetworkMode,omitempty"`
			IPv4Address          string            `json:"IPv4Address,omitempty"`
			IPv4Gateway          string            `json:"IPv4Gateway,omitempty"`
			IPv6Address          string            `json:"IPv6Address,omitempty"`
			IPv6Gateway          string            `json:"IPv6Gateway,omitempty"`
			CreateTime           time.Time         `json:"CreateTime,omitempty"`
			UpdateTime           time.Time         `json:"UpdateTime,omitempty"`
		}{
			Cluster:              m.taskMetadata.cluster,
			ContainerInstanceARN: m.containerInstanceARN,
			TaskARN:              m.taskMetadata.taskARN,
			ContainerID:          m.dockerContainerMetadata.containerID,
			ContainerName:        m.taskMetadata.containerName,
			DockerContainerName:  m.dockerContainerMetadata.dockerContainerName,
			ImageID:              m.dockerContainerMetadata.imageID,
			ImageName:            m.dockerContainerMetadata.imageName,
			Ports:                m.dockerContainerMetadata.ports,
			NetworkMode:          m.dockerContainerMetadata.networkInfo.networkMode,
			IPv4Address:          m.dockerContainerMetadata.networkInfo.ipv4Address,
			IPv4Gateway:          m.dockerContainerMetadata.networkInfo.ipv4Gateway,
			IPv6Address:          m.dockerContainerMetadata.networkInfo.ipv6Address,
			IPv6Gateway:          m.dockerContainerMetadata.networkInfo.ipv6Gateway,
			CreateTime:           m.createTime,
			UpdateTime:           m.updateTime,
		})
}
