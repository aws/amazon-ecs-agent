// +build unit

// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnmarshalVolumeResource(t *testing.T) {
	resources := make(map[string][]taskresource.TaskResource)

	volumes := []taskresource.TaskResource{
		&volume.VolumeResource{
			Name: "test-volume",
			VolumeConfig: volume.DockerVolumeConfig{
				DockerVolumeName: "test-volume-docker",
				Scope:            "task",
				Autoprovision:    true,
				Driver:           "local",
			},
		},
	}
	volumes[0].SetDesiredStatus(resourcestatus.ResourceCreated)
	volumes[0].SetKnownStatus(resourcestatus.ResourceStatusNone)

	resources["dockerVolume"] = volumes
	data, err := json.Marshal(resources)
	require.NoError(t, err)

	var unMarshalledResource ResourcesMap
	err = json.Unmarshal(data, &unMarshalledResource)
	assert.NoError(t, err, "unmarshal volume resource from data failed")
	unMarshalledVolumes, ok := unMarshalledResource["dockerVolume"]
	assert.True(t, ok, "volume resource not found in the resource map")
	assert.Equal(t, unMarshalledVolumes[0].GetName(), "test-volume")
	assert.Equal(t, unMarshalledVolumes[0].GetDesiredStatus(), resourcestatus.ResourceCreated)
	assert.Equal(t, unMarshalledVolumes[0].GetKnownStatus(), resourcestatus.ResourceStatusNone)
}

func TestMarshalUnmarshalSSMSecretResource(t *testing.T) {
	bytes := []byte(`{"ssmsecret":[{"TaskARN":"task_arn","RequiredSecrets":{"us-west-2":[]},"CreatedAt":"0001-01-01T00:00:00Z","DesiredStatus":"CREATED","KnownStatus":"REMOVED"}]}`)

	unmarshalledMap := make(ResourcesMap)
	err := unmarshalledMap.UnmarshalJSON(bytes)
	assert.NoError(t, err)

	ssmRes := unmarshalledMap["ssmsecret"][0].(*ssmsecret.SSMSecretResource)
	assert.Equal(t, "ssmsecret", ssmRes.GetName())
	assert.Equal(t, time.Time{}, ssmRes.GetCreatedAt())
	assert.Equal(t, resourcestatus.ResourceCreated, ssmRes.GetDesiredStatus())
	assert.Equal(t, resourcestatus.ResourceRemoved, ssmRes.GetKnownStatus())
}

func TestMarshalUnmarshalASMSecretResource(t *testing.T) {
	bytes := []byte(`{"asmsecret":[{"TaskARN":"task_arn","RequiredSecrets":{"secret-name_us-west-2":[]},"CreatedAt":"0001-01-01T00:00:00Z","DesiredStatus":"CREATED","KnownStatus":"REMOVED"}]}`)

	unmarshalledMap := make(ResourcesMap)
	err := unmarshalledMap.UnmarshalJSON(bytes)
	assert.NoError(t, err)

	asmRes := unmarshalledMap["asmsecret"][0].(*asmsecret.ASMSecretResource)
	assert.Equal(t, "asmsecret", asmRes.GetName())
	assert.Equal(t, time.Time{}, asmRes.GetCreatedAt())
	assert.Equal(t, resourcestatus.ResourceCreated, asmRes.GetDesiredStatus())
	assert.Equal(t, resourcestatus.ResourceRemoved, asmRes.GetKnownStatus())
}
