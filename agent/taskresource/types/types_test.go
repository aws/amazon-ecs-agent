//go:build unit
// +build unit

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

package types

import (
	"encoding/json"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/asmsecret"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/ssmsecret"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	secretKeyWest1    = "/test/secretName_us-west-2"
	asmSecretKeyWest1 = "arn:aws:secretsmanager:us-west-2:11111:secret:/test/secretName_us-west-2"
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
	resources := make(map[string][]taskresource.TaskResource)
	ssmSecrets := []taskresource.TaskResource{
		&ssmsecret.SSMSecretResource{},
	}

	ssmSecrets[0].SetDesiredStatus(resourcestatus.ResourceCreated)
	ssmSecrets[0].SetKnownStatus(resourcestatus.ResourceStatusNone)

	resources["ssmsecret"] = ssmSecrets
	data, err := json.Marshal(resources)
	require.NoError(t, err)

	var unMarshalledResource ResourcesMap
	err = json.Unmarshal(data, &unMarshalledResource)
	assert.NoError(t, err)
	unMarshalledSSMSecret, ok := unMarshalledResource["ssmsecret"]
	assert.True(t, ok)
	assert.Equal(t, unMarshalledSSMSecret[0].GetDesiredStatus(), resourcestatus.ResourceCreated)
	assert.Equal(t, unMarshalledSSMSecret[0].GetKnownStatus(), resourcestatus.ResourceStatusNone)
}

func TestMarshalUnmarshalASMSecretResource(t *testing.T) {
	resources := make(map[string][]taskresource.TaskResource)
	asmSecrets := []taskresource.TaskResource{
		&asmsecret.ASMSecretResource{},
	}
	asmSecrets[0].SetDesiredStatus(resourcestatus.ResourceCreated)
	asmSecrets[0].SetKnownStatus(resourcestatus.ResourceStatusNone)

	resources["asmsecret"] = asmSecrets
	data, err := json.Marshal(resources)
	require.NoError(t, err)

	var unMarshalledResource ResourcesMap
	err = json.Unmarshal(data, &unMarshalledResource)
	assert.NoError(t, err)
	unMarshalledASMSecret, ok := unMarshalledResource["asmsecret"]
	assert.True(t, ok)
	assert.Equal(t, unMarshalledASMSecret[0].GetDesiredStatus(), resourcestatus.ResourceCreated)
	assert.Equal(t, unMarshalledASMSecret[0].GetKnownStatus(), resourcestatus.ResourceStatusNone)
}
