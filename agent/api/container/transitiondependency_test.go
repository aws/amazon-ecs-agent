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

package container

import (
	"encoding/json"
	"testing"

	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/stretchr/testify/assert"
)

func TestUnmarshalOldTransitionDependencySet(t *testing.T) {
	bytes := []byte(`{
	  "ContainerDependencies": [
	    {
	      "ContainerName": "container",
	      "SatisfiedStatus": "RUNNING",
	      "DependentStatus": "RUNNING"
	    }
	  ]
	}`)
	unmarshalledTdMap := TransitionDependenciesMap{}
	err := json.Unmarshal(bytes, &unmarshalledTdMap)
	assert.NoError(t, err)
	assert.Len(t, unmarshalledTdMap, 1)
	assert.NotNil(t, unmarshalledTdMap[apicontainerstatus.ContainerRunning])
	dep := unmarshalledTdMap[apicontainerstatus.ContainerRunning].ContainerDependencies
	assert.Len(t, dep, 1)
	assert.Equal(t, "container", dep[0].ContainerName)
	assert.Equal(t, apicontainerstatus.ContainerRunning, dep[0].SatisfiedStatus)
	assert.Equal(t, apicontainerstatus.ContainerStatusNone, dep[0].DependentStatus)
}

func TestUnmarshalNewTransitionDependencySet(t *testing.T) {
	bytes := []byte(`{"1":{"ContainerDependencies":[{"ContainerName":"container","SatisfiedStatus":"RUNNING"}]}}`)
	unmarshalledTdMap := TransitionDependenciesMap{}
	err := json.Unmarshal(bytes, &unmarshalledTdMap)
	assert.NoError(t, err)
	assert.Len(t, unmarshalledTdMap, 1)
	assert.NotNil(t, unmarshalledTdMap[apicontainerstatus.ContainerPulled])
	dep := unmarshalledTdMap[apicontainerstatus.ContainerPulled].ContainerDependencies
	assert.Len(t, dep, 1)
	assert.Equal(t, "container", dep[0].ContainerName)
	assert.Equal(t, apicontainerstatus.ContainerRunning, dep[0].SatisfiedStatus)
	assert.Equal(t, apicontainerstatus.ContainerStatusNone, dep[0].DependentStatus)
}
