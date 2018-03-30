// +build !integration
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

	"github.com/aws/amazon-ecs-agent/agent/api"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"

	"github.com/golang/mock/gomock"
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
	err := DependenciesAreResolved(task.Containers[0], task.Containers, "", nil, nil)
	assert.NoError(t, err, "One container should resolve trivially")

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

	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Shouldn't be resolved; db isn't running")

	err = DependenciesAreResolved(db, task.Containers, "", nil, nil)
	assert.Error(t, err, "Shouldn't be resolved; dbdatavolume isn't created")

	err = DependenciesAreResolved(dbdata, task.Containers, "", nil, nil)
	assert.NoError(t, err, "data volume with no deps should resolve")

	dbdata.KnownStatusUnsafe = api.ContainerCreated
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Php shouldn't run, db is not created")

	db.KnownStatusUnsafe = api.ContainerCreated
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Php shouldn't run, db is not running")

	err = DependenciesAreResolved(db, task.Containers, "", nil, nil)
	assert.NoError(t, err, "db should be resolved, dbdata volume is Created")
	db.KnownStatusUnsafe = api.ContainerRunning

	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Php should resolve")
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

	assert.Error(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should not be resolved")
	task.Containers[1].SetDesiredStatus(api.ContainerRunning)
	assert.Error(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should not be resolved")

	task.Containers[0].KnownStatusUnsafe = api.ContainerRunning
	assert.NoError(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should be resolved")

	task.Containers[1].SetDesiredStatus(api.ContainerCreated)
	assert.NoError(t, DependenciesAreResolved(c1, task.Containers, "", nil, nil), "Dependencies should be resolved")
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
		err := DependenciesAreResolved(container, task.Containers, "", nil, nil)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}

	err := DependenciesAreResolved(pause, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Pause container's dependencies should be resolved")

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
		err = DependenciesAreResolved(container, task.Containers, "", nil, nil)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}
	pause.KnownStatusUnsafe = api.ContainerResourcesProvisioned
	// Dependecies should be resolved now that the 'pause' container has
	// transitioned into RESOURCES_PROVISIONED
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Php should resolve")
}

func TestVolumeCanResolve(t *testing.T) {
	testcases := []struct {
		TargetDesired api.ContainerStatus
		VolumeDesired api.ContainerStatus
		Resolvable    bool
	}{
		{
			TargetDesired: api.ContainerCreated,
			VolumeDesired: api.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeDesired: api.ContainerCreated,
			Resolvable:    true,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeDesired: api.ContainerRunning,
			Resolvable:    true,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeDesired: api.ContainerStopped,
			Resolvable:    true,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeDesired: api.ContainerZombie,
			Resolvable:    false,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeDesired: api.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeDesired: api.ContainerCreated,
			Resolvable:    true,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeDesired: api.ContainerRunning,
			Resolvable:    true,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeDesired: api.ContainerStopped,
			Resolvable:    true,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeDesired: api.ContainerZombie,
			Resolvable:    false,
		},
		{
			TargetDesired: api.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: api.ContainerStopped,
			Resolvable:    false,
		},
		{
			TargetDesired: api.ContainerZombie,
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
		TargetDesired api.ContainerStatus
		VolumeKnown   api.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: api.ContainerCreated,
			VolumeKnown:   api.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeKnown:   api.ContainerCreated,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeKnown:   api.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeKnown:   api.ContainerStopped,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerCreated,
			VolumeKnown:   api.ContainerZombie,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeKnown:   api.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeKnown:   api.ContainerCreated,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeKnown:   api.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeKnown:   api.ContainerStopped,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerRunning,
			VolumeKnown:   api.ContainerZombie,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerStopped,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerZombie,
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
		TargetDesired api.ContainerStatus
		RunKnown      api.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: api.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerPulled,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerCreated,
			RunKnown:      api.ContainerCreated,
			Resolved:      false,
		},
		{
			TargetDesired: api.ContainerCreated,
			RunKnown:      api.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: api.ContainerCreated,
			RunKnown:      api.ContainerStopped,
			Resolved:      true,
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+R:%s", tc.TargetDesired.String(), tc.RunKnown.String()),
			assertResolved(onSteadyStateIsResolved, tc.TargetDesired, tc.RunKnown, tc.Resolved))
	}
}

func assertCanResolve(f func(target *api.Container, dep *api.Container) bool, targetDesired, depKnown api.ContainerStatus, expectedResolvable bool) func(t *testing.T) {
	return func(t *testing.T) {
		target := &api.Container{
			DesiredStatusUnsafe: targetDesired,
		}
		dep := &api.Container{
			DesiredStatusUnsafe: depKnown,
		}
		resolvable := f(target, dep)
		assert.Equal(t, expectedResolvable, resolvable)
	}
}

func assertResolved(f func(target *api.Container, dep *api.Container) bool, targetDesired, depKnown api.ContainerStatus, expectedResolved bool) func(t *testing.T) {
	return func(t *testing.T) {
		target := &api.Container{
			DesiredStatusUnsafe: targetDesired,
		}
		dep := &api.Container{
			KnownStatusUnsafe: depKnown,
		}
		resolved := f(target, dep)
		assert.Equal(t, expectedResolved, resolved)
	}
}

func TestVerifyTransitionDependenciesResolved(t *testing.T) {
	testcases := []struct {
		Name            string
		TargetKnown     api.ContainerStatus
		TargetDesired   api.ContainerStatus
		TargetNext      api.ContainerStatus
		DependencyName  string
		DependencyKnown api.ContainerStatus
		SatisfiedStatus api.ContainerStatus
		ResolvedErr     error
	}{
		{
			Name:            "Nothing running, pull depends on running",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerStatusNone,
			SatisfiedStatus: api.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},

		{
			Name:            "Nothing running, pull depends on resources provisioned",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerStatusNone,
			SatisfiedStatus: api.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Nothing running, create depends on running",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: api.ContainerStatusNone,
			SatisfiedStatus: api.ContainerRunning,
		},
		{
			Name:            "Dependency created, pull depends on running",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerCreated,
			SatisfiedStatus: api.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency created, pull depends on resources provisioned",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerCreated,
			SatisfiedStatus: api.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency running, pull depends on running",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerRunning,
			SatisfiedStatus: api.ContainerRunning,
		},
		{
			Name:            "Dependency running, pull depends on resources provisioned",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerRunning,
			SatisfiedStatus: api.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency resources provisioned, pull depends on resources provisioned",
			TargetKnown:     api.ContainerStatusNone,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: api.ContainerResourcesProvisioned,
			SatisfiedStatus: api.ContainerResourcesProvisioned,
		},
		{
			Name:            "Dependency running, create depends on created",
			TargetKnown:     api.ContainerPulled,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: api.ContainerRunning,
			SatisfiedStatus: api.ContainerCreated,
		},
		{
			Name:            "Target running, create depends on running",
			TargetKnown:     api.ContainerRunning,
			TargetDesired:   api.ContainerRunning,
			TargetNext:      api.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: api.ContainerRunning,
			SatisfiedStatus: api.ContainerCreated,
		},
		{
			Name:            "Target pulled, desired stopped",
			TargetKnown:     api.ContainerPulled,
			TargetDesired:   api.ContainerStopped,
			TargetNext:      api.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: api.ContainerStatusNone,
			SatisfiedStatus: api.ContainerCreated,
		},
		// Note: Not all possible situations are tested here.  The only situations tested here are ones that are
		// expected to reasonably happen at the time this code was written.  Other behavior is not expected to occur,
		// so it is not tested.
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			target := &api.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				DesiredStatusUnsafe:       tc.TargetDesired,
				TransitionDependenciesMap: make(map[api.ContainerStatus]api.TransitionDependencySet),
			}
			target.BuildContainerDependency(tc.DependencyName, tc.SatisfiedStatus, tc.TargetNext)
			dep := &api.Container{
				Name:              tc.DependencyName,
				KnownStatusUnsafe: tc.DependencyKnown,
			}
			containers := make(map[string]*api.Container)
			containers[dep.Name] = dep
			resolved := verifyTransitionDependenciesResolved(target, containers, nil)
			assert.Equal(t, tc.ResolvedErr, resolved)
		})
	}
}

func TestVerifyResourceDependenciesResolved(t *testing.T) {
	testcases := []struct {
		Name             string
		TargetKnown      api.ContainerStatus
		TargetDep        api.ContainerStatus
		DependencyKnown  taskresource.ResourceStatus
		RequiredStatus   taskresource.ResourceStatus
		ExpectedResolved bool
	}{
		{
			Name:             "resource none,container pull depends on resource created",
			TargetKnown:      api.ContainerStatusNone,
			TargetDep:        api.ContainerPulled,
			DependencyKnown:  taskresource.ResourceStatus(0),
			RequiredStatus:   taskresource.ResourceStatus(1),
			ExpectedResolved: false,
		},
		{
			Name:             "resource created,container pull depends on resource created",
			TargetKnown:      api.ContainerStatusNone,
			TargetDep:        api.ContainerPulled,
			DependencyKnown:  taskresource.ResourceStatus(1),
			RequiredStatus:   taskresource.ResourceStatus(1),
			ExpectedResolved: true,
		},
		{
			Name:             "resource none,container create depends on resource created",
			TargetKnown:      api.ContainerStatusNone,
			TargetDep:        api.ContainerCreated,
			DependencyKnown:  taskresource.ResourceStatus(0),
			RequiredStatus:   taskresource.ResourceStatus(1),
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
			target := &api.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[api.ContainerStatus]api.TransitionDependencySet),
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
		TargetKnown     api.ContainerStatus
		TargetDep       api.ContainerStatus
		DependencyKnown taskresource.ResourceStatus
		RequiredStatus  taskresource.ResourceStatus
		ResolvedErr     error
	}{
		{
			Name:            "resource none,container pull depends on resource created",
			TargetKnown:     api.ContainerStatusNone,
			TargetDep:       api.ContainerPulled,
			DependencyKnown: taskresource.ResourceStatus(0),
			RequiredStatus:  taskresource.ResourceStatus(1),
			ResolvedErr:     ErrResourceDependencyNotResolved,
		},
		{
			Name:            "resource created,container pull depends on resource created",
			TargetKnown:     api.ContainerStatusNone,
			TargetDep:       api.ContainerPulled,
			DependencyKnown: taskresource.ResourceStatus(1),
			RequiredStatus:  taskresource.ResourceStatus(1),
		},
		{
			Name:            "resource none,container create depends on resource created",
			TargetKnown:     api.ContainerStatusNone,
			TargetDep:       api.ContainerCreated,
			DependencyKnown: taskresource.ResourceStatus(0),
			RequiredStatus:  taskresource.ResourceStatus(1),
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
			target := &api.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[api.ContainerStatus]api.TransitionDependencySet),
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
		mockResource.EXPECT().SetKnownStatus(taskresource.ResourceStatus(1)),
	)
	mockResource.SetKnownStatus(taskresource.ResourceStatus(1))
	target := &api.Container{
		KnownStatusUnsafe:         api.ContainerStatusNone,
		TransitionDependenciesMap: make(map[api.ContainerStatus]api.TransitionDependencySet),
	}
	target.BuildResourceDependency("resource", taskresource.ResourceStatus(1), api.ContainerPulled)
	resources := make(map[string]taskresource.TaskResource)
	resources["resource1"] = mockResource // different resource name
	resolved := verifyTransitionDependenciesResolved(target, nil, resources)
	assert.Equal(t, ErrResourceDependencyNotResolved, resolved)
}
