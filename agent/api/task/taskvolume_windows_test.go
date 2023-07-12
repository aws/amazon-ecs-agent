//go:build windows && unit
// +build windows,unit

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

package task

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/taskresource/fsxwindowsfileserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalTaskVolumeFSxWindowsFileServer(t *testing.T) {
	task := &Task{
		Arn: "test",
		Volumes: []TaskVolume{
			{
				Name: "1",
				Type: FSxWindowsFileServerVolumeType,
				Volume: &fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig{
					FileSystemID:  "fs-12345678",
					RootDirectory: "/test",
					AuthConfig: fsxwindowsfileserver.FSxWindowsFileServerAuthConfig{
						CredentialsParameter: "arn",
						Domain:               "test",
					},
				},
			},
		},
	}

	marshal, err := json.Marshal(task)
	require.NoError(t, err, "Could not marshal task")
	expectedTaskDef := `{
		"Arn": "test",
		"Family": "",
		"Version": "",
		"ServiceName": "",
		"Containers": null,
		"associations": null,
		"resources": null,	
		"volumes": [
			{
				"fsxWindowsFileServerVolumeConfiguration": {
					"authorizationConfig": {
						"credentialsParameter": "arn",
						"domain": "test"
					},
					"fileSystemId": "fs-12345678",
					"rootDirectory": "/test",
					"fsxWindowsFileServerHostPath": ""
				},
				"name": "1",
				"type": "fsxWindowsFileServer"
			}
		],
		"DesiredStatus": "NONE",
		"KnownStatus": "NONE",
		"KnownTime": "0001-01-01T00:00:00Z",
		"PullStartedAt": "0001-01-01T00:00:00Z",
		"PullStoppedAt": "0001-01-01T00:00:00Z",
		"ExecutionStoppedAt": "0001-01-01T00:00:00Z",
		"SentStatus": "NONE",
		"executionCredentialsID": "",
		"ENI": null,
		"AppMesh": null,
		"PlatformFields": %s
	  }`
	expectedTaskDef = fmt.Sprintf(expectedTaskDef, `{"cpuUnbounded": null, "memoryUnbounded": null}`)
	require.JSONEq(t, expectedTaskDef, string(marshal))
}

func TestUnmarshalTaskVolumeFSxWindowsFileServer(t *testing.T) {
	taskDef := []byte(`{
		"Arn": "test",
		"Family": "",
		"Version": "",
		"Containers": null,
		"associations": null,
		"resources": null,	
		"volumes": [
			{
				"fsxWindowsFileServerVolumeConfiguration": {
					"authorizationConfig": {
						"credentialsParameter": "arn",
						"domain": "test"
					},
					"fileSystemId": "fs-12345678",
					"rootDirectory": "/test",
					"fsxWindowsFileServerHostPath": ""
				},
				"name": "1",
				"type": "fsxWindowsFileServer"
			}
		],
		"DesiredStatus": "NONE",
		"KnownStatus": "NONE",
		"KnownTime": "0001-01-01T00:00:00Z",
		"PullStartedAt": "0001-01-01T00:00:00Z",
		"PullStoppedAt": "0001-01-01T00:00:00Z",
		"ExecutionStoppedAt": "0001-01-01T00:00:00Z",
		"SentStatus": "NONE",
		"executionCredentialsID": "",
		"ENI": null,
		"AppMesh": null,
		"PlatformFields": {}
	  }`)
	var task Task
	err := json.Unmarshal(taskDef, &task)
	require.NoError(t, err, "Could not unmarshal task")

	require.Len(t, task.Volumes, 1)
	assert.Equal(t, "fsxWindowsFileServer", task.Volumes[0].Type)
	assert.Equal(t, "1", task.Volumes[0].Name)
	fsxWindowsFileServerConfig, ok := task.Volumes[0].Volume.(*fsxwindowsfileserver.FSxWindowsFileServerVolumeConfig)
	assert.True(t, ok)
	assert.Equal(t, "fs-12345678", fsxWindowsFileServerConfig.FileSystemID)
	assert.Equal(t, "/test", fsxWindowsFileServerConfig.RootDirectory)
	assert.Equal(t, "arn", fsxWindowsFileServerConfig.AuthConfig.CredentialsParameter)
	assert.Equal(t, "test", fsxWindowsFileServerConfig.AuthConfig.Domain)
}
