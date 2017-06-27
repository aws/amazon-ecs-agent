// +build !integration
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

package dependencygraph

import (
	"testing"

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/stretchr/testify/assert"
)

func volumeStrToVol(vols []string) []api.VolumeFrom {
	ret := make([]api.VolumeFrom, len(vols))
	for i, v := range vols {
		ret[i] = api.VolumeFrom{SourceContainer: v, ReadOnly: false}
	}
	return ret
}

func steadyStateContainer(name string, links, volumes []string, desiredState api.ContainerStatus, steadyState api.ContainerStatus) *api.Container {
	container := api.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.Links = links
	container.VolumesFrom = volumeStrToVol(volumes)
	container.DesiredStatusUnsafe = desiredState
	return container
}

func createdContainer(name string, links, volumes []string, steadyState api.ContainerStatus) *api.Container {
	container := api.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.Links = links
	container.VolumesFrom = volumeStrToVol(volumes)
	container.DesiredStatusUnsafe = api.ContainerCreated
	return container
}

func TestValidDependencies(t *testing.T) {
	// Empty task
	task := &api.Task{}
	resolveable := ValidDependencies(task)
	assert.True(t, resolveable, "The zero dependency graph should resolve")

	task = &api.Task{
		Containers: []*api.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: api.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task)
	assert.True(t, resolveable, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, api.ContainerRunning, api.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, api.ContainerRunning, api.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, api.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, api.ContainerRunning, api.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, api.ContainerRunning, api.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, api.ContainerRunning)

	task = &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolveable = ValidDependencies(task)
	assert.True(t, resolveable, "The webserver group should resolve just fine")
}

func TestValidDependenciesWithCycles(t *testing.T) {
	// Unresolveable: cycle
	task := &api.Task{
		Containers: []*api.Container{
			steadyStateContainer("a", []string{"b"}, []string{}, api.ContainerRunning, api.ContainerRunning),
			steadyStateContainer("b", []string{"a"}, []string{}, api.ContainerRunning, api.ContainerRunning),
		},
	}
	resolveable := ValidDependencies(task)
	assert.False(t, resolveable, "Cycle should not be resolveable")
}

func TestValidDependenciesWithUnresolvedReference(t *testing.T) {
	// Unresolveable, reference doesn't exist
	task := &api.Task{
		Containers: []*api.Container{
			steadyStateContainer("php", []string{"db"}, []string{}, api.ContainerRunning, api.ContainerRunning),
		},
	}
	resolveable := ValidDependencies(task)
	assert.False(t, resolveable, "Nonexistent reference shouldn't resolve")
}

func TestDependenciesAreResolvedWhenSteadyStateIsRunning(t *testing.T) {
	task := &api.Task{
		Containers: []*api.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: api.ContainerRunning,
			},
		},
	}
	resolved := DependenciesAreResolved(task.Containers[0], task.Containers)
	assert.True(t, resolved, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, api.ContainerRunning, api.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, api.ContainerRunning, api.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, api.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, api.ContainerRunning, api.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, api.ContainerRunning, api.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, api.ContainerRunning)

	task = &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolved = DependenciesAreResolved(php, task.Containers)
	assert.False(t, resolved, "Shouldn't be resolved; db isn't running")

	resolved = DependenciesAreResolved(db, task.Containers)
	assert.False(t, resolved, "Shouldn't be resolved; dbdatavolume isn't created")

	resolved = DependenciesAreResolved(dbdata, task.Containers)
	assert.True(t, resolved, "data volume with no deps should resolve")

	dbdata.KnownStatusUnsafe = api.ContainerCreated
	resolved = DependenciesAreResolved(php, task.Containers)
	assert.False(t, resolved, "Php shouldn't run, db is not created")

	db.KnownStatusUnsafe = api.ContainerCreated
	resolved = DependenciesAreResolved(php, task.Containers)
	assert.False(t, resolved, "Php shouldn't run, db is not running")

	resolved = DependenciesAreResolved(db, task.Containers)
	assert.True(t, resolved, "db should be resolved, dbdata volume is Created")
	db.KnownStatusUnsafe = api.ContainerRunning

	resolved = DependenciesAreResolved(php, task.Containers)
	assert.True(t, resolved, "Php should resolve")
}

func TestRunDependencies(t *testing.T) {
	c1 := &api.Container{
		Name:              "a",
		KnownStatusUnsafe: api.ContainerStatusNone,
	}
	c2 := &api.Container{
		Name:                    "b",
		KnownStatusUnsafe:       api.ContainerStatusNone,
		DesiredStatusUnsafe:     api.ContainerCreated,
		SteadyStateDependencies: []string{"a"},
	}
	task := &api.Task{Containers: []*api.Container{c1, c2}}

	assert.False(t, DependenciesAreResolved(c2, task.Containers), "Dependencies should not be resolved")
	task.Containers[1].SetDesiredStatus(api.ContainerRunning)
	assert.False(t, DependenciesAreResolved(c2, task.Containers), "Dependencies should not be resolved")

	task.Containers[0].KnownStatusUnsafe = api.ContainerRunning
	assert.True(t, DependenciesAreResolved(c2, task.Containers), "Dependencies should be resolved")

	task.Containers[1].SetDesiredStatus(api.ContainerCreated)
	assert.True(t, DependenciesAreResolved(c1, task.Containers), "Dependencies should be resolved")
}

func TestRunDependenciesWhenSteadyStateIsResourcesProvisionedForOneContainer(t *testing.T) {
	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, api.ContainerRunning, api.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, api.ContainerRunning, api.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, api.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, api.ContainerRunning, api.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, api.ContainerRunning, api.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, api.ContainerRunning)
	// The Pause container, being added to the webserver stack
	pause := steadyStateContainer("pause", []string{}, []string{}, api.ContainerResourcesProvisioned, api.ContainerResourcesProvisioned)

	task := &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles, pause,
		},
	}

	// Add a dependency on the pause container for all containers in the webserver stack
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		container.SteadyStateDependencies = []string{"pause"}
		resolved := DependenciesAreResolved(container, task.Containers)
		assert.False(t, resolved, "Shouldn't be resolved; pause isn't running")
	}

	resolved := DependenciesAreResolved(pause, task.Containers)
	assert.True(t, resolved, "Pause container's dependencies should be resolved")

	// Transition pause container to RUNNING
	pause.KnownStatusUnsafe = api.ContainerRunning
	// Transition dependencies in webserver stack to CREATED/RUNNING state
	dbdata.KnownStatusUnsafe = api.ContainerCreated
	db.KnownStatusUnsafe = api.ContainerRunning
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		// Assert that dependencies remain unresolved until the pause container reaches
		// RESOURCES_PROVISIONED
		resolved = DependenciesAreResolved(container, task.Containers)
		assert.False(t, resolved, "Shouldn't be resolved; pause isn't running")
	}
	pause.KnownStatusUnsafe = api.ContainerResourcesProvisioned
	// Dependecies should be resolved now that the 'pause' container has
	// transitioned into RESOURCES_PROVISIONED
	resolved = DependenciesAreResolved(php, task.Containers)
	assert.True(t, resolved, "Php should resolve")
}
