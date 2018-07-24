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

package dependencygraph

import (
	"fmt"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func volumeStrToVol(vols []string) []apicontainer.VolumeFrom {
	ret := make([]apicontainer.VolumeFrom, len(vols))
	for i, v := range vols {
		ret[i] = apicontainer.VolumeFrom{SourceContainer: v, ReadOnly: false}
	}
	return ret
}

func steadyStateContainer(name string, links, volumes []string, desiredState apicontainerstatus.ContainerStatus, steadyState apicontainerstatus.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.Links = links
	container.VolumesFrom = volumeStrToVol(volumes)
	container.DesiredStatusUnsafe = desiredState
	return container
}

func createdContainer(name string, links, volumes []string, steadyState apicontainerstatus.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.Links = links
	container.VolumesFrom = volumeStrToVol(volumes)
	container.DesiredStatusUnsafe = apicontainerstatus.ContainerCreated
	return container
}

func TestValidDependencies(t *testing.T) {
	// Empty task
	task := &apitask.Task{}
	resolveable := ValidDependencies(task)
	assert.True(t, resolveable, "The zero dependency graph should resolve")

	task = &apitask.Task{
		Containers: []*apicontainer.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task)
	assert.True(t, resolveable, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, apicontainerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, apicontainerstatus.ContainerRunning)

	task = &apitask.Task{
		Containers: []*apicontainer.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolveable = ValidDependencies(task)
	assert.True(t, resolveable, "The webserver group should resolve just fine")
}

func TestValidDependenciesWithCycles(t *testing.T) {
	// Unresolveable: cycle
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			steadyStateContainer("a", []string{"b"}, []string{}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning),
			steadyStateContainer("b", []string{"a"}, []string{}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning),
		},
	}
	resolveable := ValidDependencies(task)
	assert.False(t, resolveable, "Cycle should not be resolveable")
}

func TestValidDependenciesWithUnresolvedReference(t *testing.T) {
	// Unresolveable, reference doesn't exist
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			steadyStateContainer("php", []string{"db"}, []string{}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning),
		},
	}
	resolveable := ValidDependencies(task)
	assert.False(t, resolveable, "Nonexistent reference shouldn't resolve")
}

func TestDependenciesAreResolvedWhenSteadyStateIsRunning(t *testing.T) {
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}
	err := DependenciesAreResolved(task.Containers[0], task.Containers, "", nil, nil)
	assert.NoError(t, err, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, apicontainerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, apicontainerstatus.ContainerRunning)

	task = &apitask.Task{
		Containers: []*apicontainer.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Shouldn't be resolved; db isn't running")

	err = DependenciesAreResolved(db, task.Containers, "", nil, nil)
	assert.Error(t, err, "Shouldn't be resolved; dbdatavolume isn't created")

	err = DependenciesAreResolved(dbdata, task.Containers, "", nil, nil)
	assert.NoError(t, err, "data volume with no deps should resolve")

	dbdata.KnownStatusUnsafe = apicontainerstatus.ContainerCreated
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Php shouldn't run, db is not created")

	db.KnownStatusUnsafe = apicontainerstatus.ContainerCreated
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Php shouldn't run, db is not running")

	err = DependenciesAreResolved(db, task.Containers, "", nil, nil)
	assert.NoError(t, err, "db should be resolved, dbdata volume is Created")
	db.KnownStatusUnsafe = apicontainerstatus.ContainerRunning

	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Php should resolve")
}

func TestRunDependencies(t *testing.T) {
	c1 := &apicontainer.Container{
		Name:              "a",
		KnownStatusUnsafe: apicontainerstatus.ContainerStatusNone,
	}
	c2 := &apicontainer.Container{
		Name:                    "b",
		KnownStatusUnsafe:       apicontainerstatus.ContainerStatusNone,
		DesiredStatusUnsafe:     apicontainerstatus.ContainerCreated,
		SteadyStateDependencies: []string{"a"},
	}
	task := &apitask.Task{Containers: []*apicontainer.Container{c1, c2}}

	assert.Error(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should not be resolved")
	task.Containers[1].SetDesiredStatus(apicontainerstatus.ContainerRunning)
	assert.Error(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should not be resolved")

	task.Containers[0].KnownStatusUnsafe = apicontainerstatus.ContainerRunning
	assert.NoError(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should be resolved")

	task.Containers[1].SetDesiredStatus(apicontainerstatus.ContainerCreated)
	assert.NoError(t, DependenciesAreResolved(c1, task.Containers, "", nil, nil), "Dependencies should be resolved")
}

func TestRunDependenciesWhenSteadyStateIsResourcesProvisionedForOneContainer(t *testing.T) {
	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, apicontainerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, apicontainerstatus.ContainerRunning)
	// The Pause container, being added to the webserver stack
	pause := steadyStateContainer("pause", []string{}, []string{}, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerResourcesProvisioned)

	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles, pause,
		},
	}

	// Add a dependency on the pause container for all containers in the webserver stack
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		container.SteadyStateDependencies = []string{"pause"}
		err := DependenciesAreResolved(container, task.Containers, "", nil, nil)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}

	err := DependenciesAreResolved(pause, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Pause container's dependencies should be resolved")

	// Transition pause container to RUNNING
	pause.KnownStatusUnsafe = apicontainerstatus.ContainerRunning
	// Transition dependencies in webserver stack to CREATED/RUNNING state
	dbdata.KnownStatusUnsafe = apicontainerstatus.ContainerCreated
	db.KnownStatusUnsafe = apicontainerstatus.ContainerRunning
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		// Assert that dependencies remain unresolved until the pause container reaches
		// RESOURCES_PROVISIONED
		err = DependenciesAreResolved(container, task.Containers, "", nil, nil)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}
	pause.KnownStatusUnsafe = apicontainerstatus.ContainerResourcesProvisioned
	// Dependecies should be resolved now that the 'pause' container has
	// transitioned into RESOURCES_PROVISIONED
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Php should resolve")
}

func TestVolumeCanResolve(t *testing.T) {
	testcases := []struct {
		TargetDesired apicontainerstatus.ContainerStatus
		VolumeDesired apicontainerstatus.ContainerStatus
		Resolvable    bool
	}{
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeDesired: apicontainerstatus.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeDesired: apicontainerstatus.ContainerCreated,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeDesired: apicontainerstatus.ContainerRunning,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeDesired: apicontainerstatus.ContainerStopped,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeDesired: apicontainerstatus.ContainerZombie,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeDesired: apicontainerstatus.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeDesired: apicontainerstatus.ContainerCreated,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeDesired: apicontainerstatus.ContainerRunning,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeDesired: apicontainerstatus.ContainerStopped,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeDesired: apicontainerstatus.ContainerZombie,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerStopped,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerZombie,
			Resolvable:    false,
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+V:%s", tc.TargetDesired.String(), tc.VolumeDesired.String()),
			assertCanResolve(volumeCanResolve, tc.TargetDesired, tc.VolumeDesired, tc.Resolvable))
	}
}

func TestVolumeIsResolved(t *testing.T) {
	testcases := []struct {
		TargetDesired apicontainerstatus.ContainerStatus
		VolumeKnown   apicontainerstatus.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeKnown:   apicontainerstatus.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeKnown:   apicontainerstatus.ContainerCreated,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeKnown:   apicontainerstatus.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeKnown:   apicontainerstatus.ContainerStopped,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			VolumeKnown:   apicontainerstatus.ContainerZombie,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeKnown:   apicontainerstatus.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeKnown:   apicontainerstatus.ContainerCreated,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeKnown:   apicontainerstatus.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeKnown:   apicontainerstatus.ContainerStopped,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerRunning,
			VolumeKnown:   apicontainerstatus.ContainerZombie,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerStopped,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerZombie,
			Resolved:      false,
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+V:%s", tc.TargetDesired.String(), tc.VolumeKnown.String()),
			assertResolved(volumeIsResolved, tc.TargetDesired, tc.VolumeKnown, tc.Resolved))
	}
}

func TestOnSteadyStateIsResolved(t *testing.T) {
	testcases := []struct {
		TargetDesired apicontainerstatus.ContainerStatus
		RunKnown      apicontainerstatus.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: apicontainerstatus.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerPulled,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			RunKnown:      apicontainerstatus.ContainerCreated,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			RunKnown:      apicontainerstatus.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
			RunKnown:      apicontainerstatus.ContainerStopped,
			Resolved:      true,
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+R:%s", tc.TargetDesired.String(), tc.RunKnown.String()),
			assertResolved(onSteadyStateIsResolved, tc.TargetDesired, tc.RunKnown, tc.Resolved))
	}
}

func assertCanResolve(f func(target *apicontainer.Container, dep *apicontainer.Container) bool, targetDesired, depKnown apicontainerstatus.ContainerStatus, expectedResolvable bool) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			DesiredStatusUnsafe: targetDesired,
		}
		dep := &apicontainer.Container{
			DesiredStatusUnsafe: depKnown,
		}
		resolvable := f(target, dep)
		assert.Equal(t, expectedResolvable, resolvable)
	}
}

func assertResolved(f func(target *apicontainer.Container, dep *apicontainer.Container) bool, targetDesired, depKnown apicontainerstatus.ContainerStatus, expectedResolved bool) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			DesiredStatusUnsafe: targetDesired,
		}
		dep := &apicontainer.Container{
			KnownStatusUnsafe: depKnown,
		}
		resolved := f(target, dep)
		assert.Equal(t, expectedResolved, resolved)
	}
}

func TestVerifyTransitionDependenciesResolved(t *testing.T) {
	testcases := []struct {
		Name            string
		TargetKnown     apicontainerstatus.ContainerStatus
		TargetDesired   apicontainerstatus.ContainerStatus
		TargetNext      apicontainerstatus.ContainerStatus
		DependencyName  string
		DependencyKnown apicontainerstatus.ContainerStatus
		SatisfiedStatus apicontainerstatus.ContainerStatus
		ResolvedErr     error
	}{
		{
			Name:            "Nothing running, pull depends on running",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStatusNone,
			SatisfiedStatus: apicontainerstatus.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},

		{
			Name:            "Nothing running, pull depends on resources provisioned",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStatusNone,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Nothing running, create depends on running",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStatusNone,
			SatisfiedStatus: apicontainerstatus.ContainerRunning,
		},
		{
			Name:            "Dependency created, pull depends on running",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerCreated,
			SatisfiedStatus: apicontainerstatus.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency created, pull depends on resources provisioned",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerCreated,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency running, pull depends on running",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerRunning,
			SatisfiedStatus: apicontainerstatus.ContainerRunning,
		},
		{
			Name:            "Dependency running, pull depends on resources provisioned",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerRunning,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency resources provisioned, pull depends on resources provisioned",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerResourcesProvisioned,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
		},
		{
			Name:            "Dependency running, create depends on created",
			TargetKnown:     apicontainerstatus.ContainerPulled,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerRunning,
			SatisfiedStatus: apicontainerstatus.ContainerCreated,
		},
		{
			Name:            "Target running, create depends on running",
			TargetKnown:     apicontainerstatus.ContainerRunning,
			TargetDesired:   apicontainerstatus.ContainerRunning,
			TargetNext:      apicontainerstatus.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerRunning,
			SatisfiedStatus: apicontainerstatus.ContainerCreated,
		},
		{
			Name:            "Target pulled, desired stopped",
			TargetKnown:     apicontainerstatus.ContainerPulled,
			TargetDesired:   apicontainerstatus.ContainerStopped,
			TargetNext:      apicontainerstatus.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStatusNone,
			SatisfiedStatus: apicontainerstatus.ContainerCreated,
		},
		// Note: Not all possible situations are tested here.  The only situations tested here are ones that are
		// expected to reasonably happen at the time this code was written.  Other behavior is not expected to occur,
		// so it is not tested.
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			target := &apicontainer.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				DesiredStatusUnsafe:       tc.TargetDesired,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			}
			target.BuildContainerDependency(tc.DependencyName, tc.SatisfiedStatus, tc.TargetNext)
			dep := &apicontainer.Container{
				Name:              tc.DependencyName,
				KnownStatusUnsafe: tc.DependencyKnown,
			}
			containers := make(map[string]*apicontainer.Container)
			containers[dep.Name] = dep
			resolved := verifyTransitionDependenciesResolved(target, containers, nil)
			assert.Equal(t, tc.ResolvedErr, resolved)
		})
	}
}

func TestVerifyResourceDependenciesResolved(t *testing.T) {
	testcases := []struct {
		Name            string
		TargetKnown     apicontainerstatus.ContainerStatus
		TargetDep       apicontainerstatus.ContainerStatus
		DependencyKnown resourcestatus.ResourceStatus
		RequiredStatus  resourcestatus.ResourceStatus

		ExpectedResolved bool
	}{
		{
			Name:             "resource none,container pull depends on resource created",
			TargetKnown:      apicontainerstatus.ContainerStatusNone,
			TargetDep:        apicontainerstatus.ContainerPulled,
			DependencyKnown:  resourcestatus.ResourceStatus(0),
			RequiredStatus:   resourcestatus.ResourceStatus(1),
			ExpectedResolved: false,
		},
		{
			Name:             "resource created,container pull depends on resource created",
			TargetKnown:      apicontainerstatus.ContainerStatusNone,
			TargetDep:        apicontainerstatus.ContainerPulled,
			DependencyKnown:  resourcestatus.ResourceStatus(1),
			RequiredStatus:   resourcestatus.ResourceStatus(1),
			ExpectedResolved: true,
		},
		{
			Name:             "resource none,container create depends on resource created",
			TargetKnown:      apicontainerstatus.ContainerStatusNone,
			TargetDep:        apicontainerstatus.ContainerCreated,
			DependencyKnown:  resourcestatus.ResourceStatus(0),
			RequiredStatus:   resourcestatus.ResourceStatus(1),
			ExpectedResolved: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			resourceName := "resource"
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockResource := mock_taskresource.NewMockTaskResource(ctrl)
			gomock.InOrder(
				mockResource.EXPECT().SetKnownStatus(tc.DependencyKnown),
				mockResource.EXPECT().GetKnownStatus().Return(tc.DependencyKnown).AnyTimes(),
			)
			mockResource.SetKnownStatus(tc.DependencyKnown)
			target := &apicontainer.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			}
			target.BuildResourceDependency(resourceName, tc.RequiredStatus, tc.TargetDep)
			resources := make(map[string]taskresource.TaskResource)
			resources[resourceName] = mockResource
			resolved := verifyResourceDependenciesResolved(target, resources)
			assert.Equal(t, tc.ExpectedResolved, resolved)
		})
	}
}

func TestVerifyTransitionResourceDependenciesResolved(t *testing.T) {
	testcases := []struct {
		Name            string
		TargetKnown     apicontainerstatus.ContainerStatus
		TargetDep       apicontainerstatus.ContainerStatus
		DependencyKnown resourcestatus.ResourceStatus
		RequiredStatus  resourcestatus.ResourceStatus
		ResolvedErr     error
	}{
		{
			Name:            "resource none,container pull depends on resource created",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDep:       apicontainerstatus.ContainerPulled,
			DependencyKnown: resourcestatus.ResourceStatus(0),
			RequiredStatus:  resourcestatus.ResourceStatus(1),
			ResolvedErr:     ErrResourceDependencyNotResolved,
		},
		{
			Name:            "resource created,container pull depends on resource created",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDep:       apicontainerstatus.ContainerPulled,
			DependencyKnown: resourcestatus.ResourceStatus(1),
			RequiredStatus:  resourcestatus.ResourceStatus(1),
		},
		{
			Name:            "resource none,container create depends on resource created",
			TargetKnown:     apicontainerstatus.ContainerStatusNone,
			TargetDep:       apicontainerstatus.ContainerCreated,
			DependencyKnown: resourcestatus.ResourceStatus(0),
			RequiredStatus:  resourcestatus.ResourceStatus(1),
		},
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			resourceName := "resource"
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockResource := mock_taskresource.NewMockTaskResource(ctrl)
			gomock.InOrder(
				mockResource.EXPECT().SetKnownStatus(tc.DependencyKnown),
				mockResource.EXPECT().GetKnownStatus().Return(tc.DependencyKnown).AnyTimes(),
			)
			mockResource.SetKnownStatus(tc.DependencyKnown)
			target := &apicontainer.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
			}
			target.BuildResourceDependency(resourceName, tc.RequiredStatus, tc.TargetDep)
			resources := make(map[string]taskresource.TaskResource)
			resources[resourceName] = mockResource
			resolved := verifyTransitionDependenciesResolved(target, nil, resources)
			assert.Equal(t, tc.ResolvedErr, resolved)
		})
	}
}

func TestTransitionDependencyResourceNotFound(t *testing.T) {
	// this test verifies if error is thrown when the resource dependency is not
	// found in the list of task resources
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	gomock.InOrder(
		mockResource.EXPECT().SetKnownStatus(resourcestatus.ResourceStatus(1)),
	)
	mockResource.SetKnownStatus(resourcestatus.ResourceStatus(1))
	target := &apicontainer.Container{
		KnownStatusUnsafe:         apicontainerstatus.ContainerStatusNone,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
	}
	target.BuildResourceDependency("resource", resourcestatus.ResourceStatus(1), apicontainerstatus.ContainerPulled)

	resources := make(map[string]taskresource.TaskResource)
	resources["resource1"] = mockResource // different resource name
	resolved := verifyTransitionDependenciesResolved(target, nil, resources)
	assert.Equal(t, ErrResourceDependencyNotResolved, resolved)
}
