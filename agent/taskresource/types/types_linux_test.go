//go:build linux && unit
// +build linux,unit

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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	cgroupres "github.com/aws/amazon-ecs-agent/agent/taskresource/cgroup"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/firelens"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
)

func TestUnmarshalResourcesMap(t *testing.T) {
	cgroupRoot := "/ecs/taskid"
	cgroupMountPath := "/sys/fs/cgroup"
	bytes := []byte(`{"cgroup":[{"CgroupRoot":"/ecs/taskid","CgroupMountPath":"/sys/fs/cgroup","CreatedAt":"0001-01-01T00:00:00Z","DesiredStatus":"REMOVED","KnownStatus":"REMOVED"}]}`)
	unmarshalledMap := make(ResourcesMap)
	err := unmarshalledMap.UnmarshalJSON(bytes)
	assert.NoError(t, err)
	var cgroupResource *cgroupres.CgroupResource
	cgroupResource = unmarshalledMap["cgroup"][0].(*cgroupres.CgroupResource)
	assert.Equal(t, cgroupRoot, cgroupResource.GetCgroupRoot())
	assert.Equal(t, cgroupMountPath, cgroupResource.GetCgroupMountPath())
	assert.Equal(t, time.Time{}, cgroupResource.GetCreatedAt())
	assert.Equal(t, resourcestatus.ResourceStatus(cgroupres.CgroupRemoved), cgroupResource.GetDesiredStatus())
	assert.Equal(t, resourcestatus.ResourceStatus(cgroupres.CgroupRemoved), cgroupResource.GetKnownStatus())
}

func TestMarshalUnmarshalFirelensResource(t *testing.T) {
	resources := make(map[string][]taskresource.TaskResource)
	firelensResources := []taskresource.TaskResource{
		&firelens.FirelensResource{},
	}
	firelensResources[0].SetDesiredStatus(resourcestatus.ResourceCreated)
	firelensResources[0].SetKnownStatus(resourcestatus.ResourceStatusNone)

	resources["firelens"] = firelensResources
	data, err := json.Marshal(resources)
	require.NoError(t, err)

	var unmarshalledResource ResourcesMap
	err = json.Unmarshal(data, &unmarshalledResource)
	assert.NoError(t, err)
	unMarshalledFirelensResources, ok := unmarshalledResource["firelens"]
	assert.True(t, ok)
	assert.Equal(t, unMarshalledFirelensResources[0].GetDesiredStatus(), resourcestatus.ResourceCreated)
	assert.Equal(t, unMarshalledFirelensResources[0].GetKnownStatus(), resourcestatus.ResourceStatusNone)
}
