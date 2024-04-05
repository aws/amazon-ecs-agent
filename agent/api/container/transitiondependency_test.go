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

package container

import (
	"encoding/json"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that TransitionDependencySet marshaled in different formats (as changes are introduced)
// can all be unmarshaled.
func TestUnmarshalTransitionDependencySet(t *testing.T) {
	tcs := []struct {
		name      string
		marshaled []byte
	}{
		{
			name:      "with dependent status - this is the oldest format",
			marshaled: []byte(`{"ContainerDependencies": [{"ContainerName": "container", "SatisfiedStatus": "RUNNING", "DependentStatus": "PULLED"}]}`),
		},
		{
			name:      "as a map with integer keys for statuses - this is an older format",
			marshaled: []byte(`{"1":{"ContainerDependencies":[{"ContainerName":"container","SatisfiedStatus":"RUNNING"}]}}`),
		},
		{
			name:      "as a map with string keys for statuses - this is the latest format",
			marshaled: []byte(`{"PULLED":{"ContainerDependencies":[{"ContainerName":"container","SatisfiedStatus":"RUNNING"}]}}`),
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			unmarshalledTdMap := TransitionDependenciesMap{}
			err := json.Unmarshal(tc.marshaled, &unmarshalledTdMap)
			require.NoError(t, err)
			require.Len(t, unmarshalledTdMap, 1)
			require.NotNil(t, unmarshalledTdMap[apicontainerstatus.ContainerPulled])
			dep := unmarshalledTdMap[apicontainerstatus.ContainerPulled].ContainerDependencies
			require.Len(t, dep, 1)
			assert.Equal(t, "container", dep[0].ContainerName)
			assert.Equal(t, apicontainerstatus.ContainerRunning, dep[0].SatisfiedStatus)
			assert.Equal(t, apicontainerstatus.ContainerStatusNone, dep[0].DependentStatus)
		})
	}
}

// Tests that marshaled TransitionDependenciesMap can be unmarshaled.
func TestMarshalUnmarshalTransitionDependencySet(t *testing.T) {
	var depMap TransitionDependenciesMap = map[apicontainerstatus.ContainerStatus]TransitionDependencySet{
		apicontainerstatus.ContainerRunning: {
			ContainerDependencies: []ContainerDependency{
				{
					ContainerName:   "db",
					SatisfiedStatus: apicontainerstatus.ContainerRunning,
				},
			},
			ResourceDependencies: []ResourceDependency{
				{
					Name:           "config",
					RequiredStatus: status.ResourceCreated,
				},
			},
		},
	}
	marshaled, err := json.Marshal(depMap)
	require.NoError(t, err)
	var unmarshaled TransitionDependenciesMap
	err = json.Unmarshal(marshaled, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, depMap, unmarshaled)
}

// Tests that marshaling of TransitionDependenciesMap works as expected.
func TestMarshalTransitionDependencySet(t *testing.T) {
	var depMap TransitionDependenciesMap = map[apicontainerstatus.ContainerStatus]TransitionDependencySet{
		apicontainerstatus.ContainerPulled: {
			ContainerDependencies: []ContainerDependency{
				{
					ContainerName:   "pre",
					SatisfiedStatus: apicontainerstatus.ContainerStopped,
				},
			},
		},
	}
	marshaled, err := json.Marshal(depMap)
	require.NoError(t, err)
	assert.Equal(
		t,
		`{"PULLED":{"ContainerDependencies":[{"ContainerName":"pre","SatisfiedStatus":"STOPPED"}],"ResourceDependencies":null}}`,
		string(marshaled))
}
