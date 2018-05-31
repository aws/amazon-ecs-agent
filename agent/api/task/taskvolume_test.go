// +build unit

// Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
	"testing"

	taskresourcevolume "github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnmarshalTaskVolumes(t *testing.T) {
	task := &Task{
		Arn: "test",
		Volumes: []TaskVolume{
			TaskVolume{Name: "1", Type: HostVolumeType, Volume: &taskresourcevolume.LocalDockerVolume{}},
			TaskVolume{Name: "2", Type: HostVolumeType, Volume: &taskresourcevolume.FSHostVolume{FSSourcePath: "/path"}},
			TaskVolume{Name: "3", Type: DockerVolumeType, Volume: &taskresourcevolume.DockerVolumeConfig{Scope: "task", Driver: "local"}},
		},
	}

	marshal, err := json.Marshal(task)
	require.NoError(t, err, "Could not marshal task")

	var out Task
	err = json.Unmarshal(marshal, &out)
	require.NoError(t, err, "Could not unmarshal task")
	require.Len(t, out.Volumes, 3, "Incorrect number of volumes")

	var v1, v2, v3 TaskVolume

	for _, v := range out.Volumes {
		switch v.Name {
		case "1":
			v1 = v
		case "2":
			v2 = v
		case "3":
			v3 = v
		}
	}

	_, ok := v1.Volume.(*taskresourcevolume.LocalDockerVolume)
	assert.True(t, ok, "Expected v1 to be local empty volume")
	assert.Equal(t, "/path", v2.Volume.Source(), "Expected v2 to have 'sourcepath' work correctly")
	_, ok = v2.Volume.(*taskresourcevolume.FSHostVolume)
	assert.True(t, ok, "Expected v2 to be host volume")
	assert.Equal(t, "/path", v2.Volume.(*taskresourcevolume.FSHostVolume).FSSourcePath, "Unmarshaled v2 didn't match marshalled v2")

	dockerVolume, ok := v3.Volume.(*taskresourcevolume.DockerVolumeConfig)
	assert.True(t, ok, "incorrect DockerVolumeConfig type")
	assert.Equal(t, "task", dockerVolume.Scope)
	assert.Equal(t, "local", dockerVolume.Driver)
}
