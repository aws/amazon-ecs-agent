// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package v1

import (
	"encoding/json"
	"testing"
	
	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apieni "github.com/aws/amazon-ecs-agent/agent/api/eni"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	apitaskstatus "github.com/aws/amazon-ecs-agent/agent/api/task/status"
	"github.com/stretchr/testify/assert"
)

const (
	taskARN        = "t1"
	family         = "sleep"
	version        = "1"
	containerID    = "cid"
	containerName  = "sleepy"
	eniIPv4Address = "10.0.0.2"
)

func TestTaskResponse(t *testing.T) {
	expectedTaskResponseJSONString :=
		`{` +
			`"Arn":"t1",` +
			`"DesiredStatus":"RUNNING",` +
			`"KnownStatus":"RUNNING",` +
			`"Family":"sleep",` +
			`"Version":"1",` +
			`"Containers":[{` +
				`"DockerId":"cid",` +
				`"DockerName":"sleepy",` +
				`"Name":"sleepy",` +
				`"Ports":[{` +
					`"ContainerPort":80,` +
					`"Protocol":"tcp",` +
					`"HostPort":80` +
				`}],` +
				`"Networks":[{` +
					`"NetworkMode":"awsvpc",` +
					`"IPv4Addresses":["10.0.0.2"]` +
				`}]` +
			`}]` +
		`}`

	task := &apitask.Task{
	   Arn:                 taskARN,
	   Family:              family,
	   Version:             version,
	   DesiredStatusUnsafe: apitaskstatus.TaskRunning,
	   KnownStatusUnsafe:   apitaskstatus.TaskRunning,
	   ENI: &apieni.ENI{
	      IPV4Addresses: []*apieni.ENIIPV4Address{
	         {
	            Address: eniIPv4Address,
	         },
	      },
	   },
	}

	container := &apicontainer.Container{
	   Name: containerName,
	   Ports: []apicontainer.PortBinding{
	      {
	         ContainerPort: 80,
	         Protocol:      apicontainer.TransportProtocolTCP,
	      },
	   },
	}

	containerNameToDockerContainer := map[string]*apicontainer.DockerContainer{
	   taskARN: &apicontainer.DockerContainer{
	      DockerID:   containerID,
	      DockerName: containerName,
	      Container:  container,
	   },
	}

	taskResponse := NewTaskResponse(task, containerNameToDockerContainer)

	taskResponseJSON, err := json.Marshal(taskResponse)

	assert.NoError(t, err)
	assert.Equal(t, expectedTaskResponseJSONString, string(taskResponseJSON))
}


func TestContainerResponse(t *testing.T) {
	expectedContainerResponseJSONString :=
		`{` +
			`"DockerId":"cid",` +
			`"DockerName":"sleepy",` +
			`"Name":"sleepy",` +
			`"Ports":[{` +
				`"ContainerPort":80,` +
				`"Protocol":"tcp",` +
				`"HostPort":80` +
			`}],` +
			`"Networks":[{` +
				`"NetworkMode":"awsvpc",` +
				`"IPv4Addresses":["10.0.0.2"]` +
			`}]` +
		`}`

	container := &apicontainer.Container{
	   Name: containerName,
	   Ports: []apicontainer.PortBinding{
	      {
	         ContainerPort: 80,
	         Protocol:      apicontainer.TransportProtocolTCP,
	      },
	   },
	}

	dockerContainer := &apicontainer.DockerContainer{
	   DockerID:   containerID,
	   DockerName: containerName,
	   Container:  container,
	}

	eni := &apieni.ENI{
		IPV4Addresses: []*apieni.ENIIPV4Address{
			{
				Address: eniIPv4Address,
			},
		},
	}

	containerResponse := NewContainerResponse(dockerContainer, eni)
	containerResponseJSON, err := json.Marshal(containerResponse)

	assert.NoError(t, err)
	assert.Equal(t, expectedContainerResponseJSONString, string(containerResponseJSON))
}

func TestPortBindingsResponse(t *testing.T) {
	container := &apicontainer.Container{
		Name: containerName,
		Ports: []apicontainer.PortBinding{
			{
				ContainerPort: 80,
				HostPort:      80,
				Protocol:      apicontainer.TransportProtocolTCP,
			},
		},
	}

	dockerContainer := &apicontainer.DockerContainer{
		Container:  container,
	}

	PortBindingsResponse := NewPortBindingsResponse(dockerContainer, nil)

	assert.Equal(t, uint16(80), PortBindingsResponse[0].ContainerPort)
	assert.Equal(t, uint16(80), PortBindingsResponse[0].HostPort)
	assert.Equal(t, "tcp", PortBindingsResponse[0].Protocol)
}
