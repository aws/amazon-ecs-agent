// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

package api

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/utils"
	"github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/assert"
)

func TestOverridden(t *testing.T) {
	container := &Container{
		Name:                "name",
		Image:               "image",
		Command:             []string{"foo", "bar"},
		CPU:                 1,
		Memory:              1,
		Links:               []string{},
		Ports:               []PortBinding{{10, 10, "", TransportProtocolTCP}},
		Overrides:           ContainerOverrides{},
		DesiredStatusUnsafe: ContainerRunning,
		AppliedStatus:       ContainerRunning,
		KnownStatusUnsafe:   ContainerRunning,
	}

	overridden := container.Overridden()
	// No overrides, should be identity
	assert.True(t, reflect.DeepEqual(container, overridden))
	assert.Equal(t, container, overridden)
	overridden.Name = "mutated"
	assert.Equal(t, container.Name, "name", "Should make a copy")
}

type configPair struct {
	Container *Container
	Config    *docker.Config
}

func (pair configPair) Equal() bool {
	conf := pair.Config
	cont := pair.Container

	if (conf.Memory / 1024 / 1024) != int64(cont.Memory) {
		return false
	}
	if conf.CPUShares != int64(cont.CPU) {
		return false
	}
	if conf.Image != cont.Image {
		return false
	}
	if cont.EntryPoint == nil && !utils.StrSliceEqual(conf.Entrypoint, []string{}) {
		return false
	}
	if cont.EntryPoint != nil && !utils.StrSliceEqual(conf.Entrypoint, *cont.EntryPoint) {
		return false
	}
	if !utils.StrSliceEqual(cont.Command, conf.Cmd) {
		return false
	}
	// TODO, Volumes, VolumesFrom, ExposedPorts

	return true
}

func TestGetSteadyStateStatusReturnsRunningByDefault(t *testing.T) {
	container := &Container{}
	assert.Equal(t, container.GetSteadyStateStatus(), ContainerRunning)
}

func TestIsKnownSteadyState(t *testing.T) {
	// This creates a container with `iota` ContainerStatus (NONE)
	container := &Container{}
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to PULLED, still not in steady state
	container.SetKnownStatus(ContainerPulled)
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to CREATED, still not in steady state
	container.SetKnownStatus(ContainerCreated)
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to RUNNING, now we're in steady state
	container.SetKnownStatus(ContainerRunning)
	assert.True(t, container.IsKnownSteadyState())
	// Now, set steady state to RESOURCES_PROVISIONED
	resourcesProvisioned := ContainerResourcesProvisioned
	container.SteadyStateStatusUnsafe = &resourcesProvisioned
	// Container is not in steady state anymore
	assert.False(t, container.IsKnownSteadyState())
	// Transition container to RESOURCES_PROVISIONED, we're in
	// steady state again
	container.SetKnownStatus(ContainerResourcesProvisioned)
	assert.True(t, container.IsKnownSteadyState())
}

func TestGetNextStateProgression(t *testing.T) {
	// This creates a container with `iota` ContainerStatus (NONE)
	container := &Container{}
	// NONE should transition to PULLED
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerPulled)
	container.SetKnownStatus(ContainerPulled)
	// PULLED should transition to CREATED
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerCreated)
	container.SetKnownStatus(ContainerCreated)
	// CREATED should transition to RUNNING
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerRunning)
	container.SetKnownStatus(ContainerRunning)
	// RUNNING should transition to STOPPED
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerStopped)

	resourcesProvisioned := ContainerResourcesProvisioned
	container.SteadyStateStatusUnsafe = &resourcesProvisioned
	// Set steady state to RESOURCES_PROVISIONED
	// RUNNING should transition to RESOURCES_PROVISIONED based on steady state
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerResourcesProvisioned)
	container.SetKnownStatus(ContainerResourcesProvisioned)
	assert.Equal(t, container.GetNextKnownStateProgression(), ContainerStopped)
}

func TestIsInternal(t *testing.T) {
	testCases := []struct {
		container *Container
		internal  bool
	}{
		{&Container{}, false},
		{&Container{Type: ContainerNormal}, false},
		{&Container{Type: ContainerCNIPause}, true},
		{&Container{Type: ContainerEmptyHostVolume}, true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("IsInternal shoukd return %t for %s", tc.internal, tc.container.String()),
			func(t *testing.T) {
				assert.Equal(t, tc.internal, tc.container.IsInternal())
			})
	}
}
