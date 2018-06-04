// +build unit

// Copyright 2017-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package dockerstate

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	stateFileContents = `
{
      "Tasks": [
        {
          "Arn": "task1",
          "Family": "mytask",
          "Version": "1",
          "Containers": [
            {
              "Name": "foo",
              "Image": "myimage:latest",
              "ImageID": "sha256:invalid",
              "Command": null,
              "Cpu": 0,
              "Memory": 256,
              "Links": null,
              "volumesFrom": [],
              "mountPoints": [],
              "portMappings": null,
              "Essential": true,
              "EntryPoint": null,
              "environment": {},
              "overrides": {
                "command": null
              },
              "dockerConfig": {
                "config": "{}",
                "hostConfig": "{\"NetworkMode\":\"awsvpc\",\"CapAdd\":[],\"CapDrop\":[]}",
                "version": "1.18"
              },
              "registryAuthentication": {
                "type": "ecr",
                "ecrAuthData": {
                  "endpointOverride": "",
                  "region": "us-west-2",
                  "registryId": "2013"
                }
              },
              "desiredStatus": "RUNNING",
              "KnownStatus": "RUNNING",
              "TransitionDependencySet": {
                "ContainerDependencies": [
                  {
                    "ContainerName": "~internal~ecs~pause",
                    "SatisfiedStatus": "RESOURCES_PROVISIONED",
                    "DependentStatus": "PULLED"
                  }
                ]
              },
              "RunDependencies": null,
              "IsInternal": "NORMAL",
              "AppliedStatus": "NONE",
              "ApplyingError": null,
              "SentStatus": "NONE",
              "KnownPortBindingsUnsafe": []
            },
            {
              "Name": "~internal~ecs~pause",
              "Image": "amazon/amazon-ecs-pause:0.1.0",
              "ImageID": "",
              "Command": null,
              "Cpu": 0,
              "Memory": 0,
              "Links": null,
              "volumesFrom": null,
              "mountPoints": null,
              "portMappings": null,
              "Essential": true,
              "EntryPoint": null,
              "environment": null,
              "overrides": {
                "command": null
              },
              "dockerConfig": {
                "config": null,
                "hostConfig": null,
                "version": null
              },
              "registryAuthentication": null,
              "desiredStatus": "RESOURCES_PROVISIONED",
              "KnownStatus": "RESOURCES_PROVISIONED",
              "TransitionDependencySet": {
                "ContainerDependencies": null
              },
              "RunDependencies": null,
              "IsInternal": "CNI_PAUSE",
              "AppliedStatus": "NONE",
              "ApplyingError": null,
              "SentStatus": "NONE",
              "KnownPortBindingsUnsafe": [],
              "SteadyStateStatus": "RESOURCES_PROVISIONED"
            }
          ],
          "volumes": [],
          "DesiredStatus": "RUNNING",
          "KnownStatus": "RUNNING",
          "KnownTime": "2017-11-01T20:24:21.449897483Z",
          "SentStatus": "RUNNING",
          "StartSequenceNumber": 9,
          "StopSequenceNumber": 0,
          "ENI": {
            "ec2Id": "eni-abcd",
            "IPV4Addresses": [
              {
                "Primary": true,
                "Address": "10.0.0.142"
              }
            ],
            "IPV6Addresses": null,
            "MacAddress": "0a:1b:2c:3d:4e:5f"
          }
        }
      ],
      "IdToContainer": {
        "042fd42cf3016526ccff43ef6ccb2c6a4b6f112bb74d5175cbebca75059b0c38": {
          "DockerId": "042fd42cf3016526ccff43ef6ccb2c6a4b6f112bb74d5175cbebca75059b0c38",
          "DockerName": "ecs-mytask-1-internalecspause-a4d2fdeaf1eed2cf8001",
          "Container": {
            "Name": "~internal~ecs~pause",
            "Image": "amazon/amazon-ecs-pause:0.1.0",
            "ImageID": "",
            "Command": null,
            "Cpu": 0,
            "Memory": 0,
            "Links": null,
            "volumesFrom": null,
            "mountPoints": null,
            "portMappings": null,
            "Essential": true,
            "EntryPoint": null,
            "environment": null,
            "overrides": {
              "command": null
            },
            "dockerConfig": {
              "config": null,
              "hostConfig": null,
              "version": null
            },
            "registryAuthentication": null,
            "desiredStatus": "RESOURCES_PROVISIONED",
            "KnownStatus": "RESOURCES_PROVISIONED",
            "TransitionDependencySet": {
              "ContainerDependencies": null
            },
            "RunDependencies": null,
            "IsInternal": "CNI_PAUSE",
            "AppliedStatus": "NONE",
            "ApplyingError": null,
            "SentStatus": "NONE",
            "KnownPortBindingsUnsafe": [],
            "SteadyStateStatus": "RESOURCES_PROVISIONED"
          }
        },
        "40109a71187ddd35effcd4e20067f97c140cd8ca7bef6a62028743c7f5b88a53": {
          "DockerId": "40109a71187ddd35effcd4e20067f97c140cd8ca7bef6a62028743c7f5b88a53",
          "DockerName": "ecs-mytask-1-foo-96ec97fe92a5e9dc4f00",
          "Container": {
            "Name": "foo",
            "Image": "746413922013.dkr.ecr.us-west-2.amazonaws.com/fortune-server:latest",
            "ImageID": "sha256:invalid",
            "Command": null,
            "Cpu": 0,
            "Memory": 256,
            "Links": null,
            "volumesFrom": [],
            "mountPoints": [],
            "portMappings": null,
            "Essential": true,
            "EntryPoint": null,
            "environment": {},
            "overrides": {
              "command": null
            },
            "dockerConfig": {
              "config": "{}",
              "hostConfig": "{\"NetworkMode\":\"awsvpc\",\"CapAdd\":[],\"CapDrop\":[]}",
              "version": "1.18"
            },
            "registryAuthentication": {
              "type": "ecr",
              "ecrAuthData": {
                "endpointOverride": "",
                "region": "us-west-2",
                "registryId": "746413922013"
              }
            },
            "desiredStatus": "RUNNING",
            "KnownStatus": "RUNNING",
            "TransitionDependencySet": {
              "ContainerDependencies": [
                {
                  "ContainerName": "~internal~ecs~pause",
                  "SatisfiedStatus": "RESOURCES_PROVISIONED",
                  "DependentStatus": "PULLED"
                }
              ]
            },
            "RunDependencies": null,
            "IsInternal": "NORMAL",
            "AppliedStatus": "NONE",
            "ApplyingError": null,
            "SentStatus": "NONE",
            "KnownPortBindingsUnsafe": []
          }
        }
      },
      "IdToTask": {
        "042fd42cf3016526ccff43ef6ccb2c6a4b6f112bb74d5175cbebca75059b0c38": "task1",
        "40109a71187ddd35effcd4e20067f97c140cd8ca7bef6a62028743c7f5b88a53": "task1"
      },
      "ImageStates": [
        {
          "Image": {
            "ImageID": "sha256:invalid",
            "Names": [
              "myimage:latest"
            ],
            "Size": 150947595
          },
          "PulledAt": "2017-11-01T15:26:56.610709526Z",
          "LastUsedAt": "2017-11-01T15:26:56.610709991Z",
          "PullSucceeded": true
        }
      ],
      "ENIAttachments": [
        {
          "taskArn": "task1",
          "attachmentArn": "attachment1",
          "attachSent": true,
          "macAddress": "0a:1b:2c:3d:4e:5f",
          "status": 1,
          "expiresAt": "2017-11-01T15:29:39.239357758Z"
        }
      ],
      "IPToTask": {
        "169.254.172.2": "task1"
      }
}
`
)

func TestUnmarshalMarshal(t *testing.T) {
	// Create a new state object
	state := newDockerTaskEngineState()
	// Validate all the upper level fields unmarshaled
	validateUnmarshaledState(t, state, []byte(stateFileContents))
	// Marshal the state again
	stateContents, err := state.MarshalJSON()
	assert.NoError(t, err)
	t.Logf("Validating marshaled state by unmarshaling it again")
	validateUnmarshaledState(t, state, stateContents)
}

func validateUnmarshaledState(t *testing.T, state *DockerTaskEngineState, contents []byte) {
	state.initializeDockerTaskEngineState()
	// Load state from string
	err := state.UnmarshalJSON(contents)
	assert.NoError(t, err)
	tasks := state.AllTasks()
	assert.Len(t, tasks, 1)
	assert.Equal(t, "task1", tasks[0].Arn)
	_, ok := state.idToContainer["042fd42cf3016526ccff43ef6ccb2c6a4b6f112bb74d5175cbebca75059b0c38"]
	assert.True(t, ok)
	_, ok = state.idToContainer["40109a71187ddd35effcd4e20067f97c140cd8ca7bef6a62028743c7f5b88a53"]
	assert.True(t, ok)
	images := state.AllImageStates()
	assert.Len(t, images, 1)
	assert.Equal(t, "sha256:invalid", images[0].Image.ImageID)
	attachments := state.AllENIAttachments()
	assert.Len(t, attachments, 1)
	assert.Equal(t, "attachment1", attachments[0].AttachmentARN)
	_, ok = state.ipToTask["169.254.172.2"]
	assert.True(t, ok, fmt.Sprintf("%s", state.ipToTask))
}
