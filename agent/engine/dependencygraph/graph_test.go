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
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	"github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"

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

func steadyStateContainer(name string, links, volumes []string, desiredState apicontainer.ContainerStatus, steadyState apicontainer.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.Links = links
	container.VolumesFrom = volumeStrToVol(volumes)
	container.DesiredStatusUnsafe = desiredState
	return container
}

func createdContainer(name string, links, volumes []string, steadyState apicontainer.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.Links = links
	container.VolumesFrom = volumeStrToVol(volumes)
	container.DesiredStatusUnsafe = apicontainer.ContainerCreated
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
				DesiredStatusUnsafe: apicontainer.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task)
	assert.True(t, resolveable, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, apicontainer.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, apicontainer.ContainerRunning)

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
			steadyStateContainer("a", []string{"b"}, []string{}, apicontainer.ContainerRunning, apicontainer.ContainerRunning),
			steadyStateContainer("b", []string{"a"}, []string{}, apicontainer.ContainerRunning, apicontainer.ContainerRunning),
		},
	}
	resolveable := ValidDependencies(task)
	assert.False(t, resolveable, "Cycle should not be resolveable")
}

func TestValidDependenciesWithUnresolvedReference(t *testing.T) {
	// Unresolveable, reference doesn't exist
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			steadyStateContainer("php", []string{"db"}, []string{}, apicontainer.ContainerRunning, apicontainer.ContainerRunning),
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
				DesiredStatusUnsafe: apicontainer.ContainerRunning,
			},
		},
	}
	err := DependenciesAreResolved(task.Containers[0], task.Containers, "", nil, nil)
	assert.NoError(t, err, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, apicontainer.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, apicontainer.ContainerRunning)

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

	dbdata.KnownStatusUnsafe = apicontainer.ContainerCreated
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Php shouldn't run, db is not created")

	db.KnownStatusUnsafe = apicontainer.ContainerCreated
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.Error(t, err, "Php shouldn't run, db is not running")

	err = DependenciesAreResolved(db, task.Containers, "", nil, nil)
	assert.NoError(t, err, "db should be resolved, dbdata volume is Created")
	db.KnownStatusUnsafe = apicontainer.ContainerRunning

	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Php should resolve")
}

func TestRunDependencies(t *testing.T) {
	c1 := &apicontainer.Container{
		Name:              "a",
		KnownStatusUnsafe: apicontainer.ContainerStatusNone,
	}
	c2 := &apicontainer.Container{
		Name:                    "b",
		KnownStatusUnsafe:       apicontainer.ContainerStatusNone,
		DesiredStatusUnsafe:     apicontainer.ContainerCreated,
		SteadyStateDependencies: []string{"a"},
	}
	task := &apitask.Task{Containers: []*apicontainer.Container{c1, c2}}

	assert.Error(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should not be resolved")
	task.Containers[1].SetDesiredStatus(apicontainer.ContainerRunning)
	assert.Error(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should not be resolved")

	task.Containers[0].KnownStatusUnsafe = apicontainer.ContainerRunning
	assert.NoError(t, DependenciesAreResolved(c2, task.Containers, "", nil, nil), "Dependencies should be resolved")

	task.Containers[1].SetDesiredStatus(apicontainer.ContainerCreated)
	assert.NoError(t, DependenciesAreResolved(c1, task.Containers, "", nil, nil), "Dependencies should be resolved")
}

func TestRunDependenciesWhenSteadyStateIsResourcesProvisionedForOneContainer(t *testing.T) {
	// Webserver stack
	php := steadyStateContainer("php", []string{"db"}, []string{}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	db := steadyStateContainer("db", []string{}, []string{"dbdatavolume"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []string{}, []string{}, apicontainer.ContainerRunning)
	webserver := steadyStateContainer("webserver", []string{"php"}, []string{"htmldata"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []string{}, []string{"sharedcssfiles"}, apicontainer.ContainerRunning, apicontainer.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{}, apicontainer.ContainerRunning)
	// The Pause container, being added to the webserver stack
	pause := steadyStateContainer("pause", []string{}, []string{}, apicontainer.ContainerResourcesProvisioned, apicontainer.ContainerResourcesProvisioned)

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
	pause.KnownStatusUnsafe = apicontainer.ContainerRunning
	// Transition dependencies in webserver stack to CREATED/RUNNING state
	dbdata.KnownStatusUnsafe = apicontainer.ContainerCreated
	db.KnownStatusUnsafe = apicontainer.ContainerRunning
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		// Assert that dependencies remain unresolved until the pause container reaches
		// RESOURCES_PROVISIONED
		err = DependenciesAreResolved(container, task.Containers, "", nil, nil)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}
	pause.KnownStatusUnsafe = apicontainer.ContainerResourcesProvisioned
	// Dependecies should be resolved now that the 'pause' container has
	// transitioned into RESOURCES_PROVISIONED
	err = DependenciesAreResolved(php, task.Containers, "", nil, nil)
	assert.NoError(t, err, "Php should resolve")
}

func TestVolumeCanResolve(t *testing.T) {
	testcases := []struct {
		TargetDesired apicontainer.ContainerStatus
		VolumeDesired apicontainer.ContainerStatus
		Resolvable    bool
	}{
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeDesired: apicontainer.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeDesired: apicontainer.ContainerCreated,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeDesired: apicontainer.ContainerRunning,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeDesired: apicontainer.ContainerStopped,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeDesired: apicontainer.ContainerZombie,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeDesired: apicontainer.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeDesired: apicontainer.ContainerCreated,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeDesired: apicontainer.ContainerRunning,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeDesired: apicontainer.ContainerStopped,
			Resolvable:    true,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeDesired: apicontainer.ContainerZombie,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainer.ContainerStatusNone,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainer.ContainerStopped,
			Resolvable:    false,
		},
		{
			TargetDesired: apicontainer.ContainerZombie,
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
		TargetDesired apicontainer.ContainerStatus
		VolumeKnown   apicontainer.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeKnown:   apicontainer.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeKnown:   apicontainer.ContainerCreated,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeKnown:   apicontainer.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeKnown:   apicontainer.ContainerStopped,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			VolumeKnown:   apicontainer.ContainerZombie,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeKnown:   apicontainer.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeKnown:   apicontainer.ContainerCreated,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeKnown:   apicontainer.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeKnown:   apicontainer.ContainerStopped,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerRunning,
			VolumeKnown:   apicontainer.ContainerZombie,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerStopped,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerZombie,
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
		TargetDesired apicontainer.ContainerStatus
		RunKnown      apicontainer.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: apicontainer.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerPulled,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			RunKnown:      apicontainer.ContainerCreated,
			Resolved:      false,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			RunKnown:      apicontainer.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: apicontainer.ContainerCreated,
			RunKnown:      apicontainer.ContainerStopped,
			Resolved:      true,
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+R:%s", tc.TargetDesired.String(), tc.RunKnown.String()),
			assertResolved(onSteadyStateIsResolved, tc.TargetDesired, tc.RunKnown, tc.Resolved))
	}
}

func assertCanResolve(f func(target *apicontainer.Container, dep *apicontainer.Container) bool, targetDesired, depKnown apicontainer.ContainerStatus, expectedResolvable bool) func(t *testing.T) {
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

func assertResolved(f func(target *apicontainer.Container, dep *apicontainer.Container) bool, targetDesired, depKnown apicontainer.ContainerStatus, expectedResolved bool) func(t *testing.T) {
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
		TargetKnown     apicontainer.ContainerStatus
		TargetDesired   apicontainer.ContainerStatus
		TargetNext      apicontainer.ContainerStatus
		DependencyName  string
		DependencyKnown apicontainer.ContainerStatus
		SatisfiedStatus apicontainer.ContainerStatus
		ResolvedErr     error
	}{
		{
			Name:            "Nothing running, pull depends on running",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerStatusNone,
			SatisfiedStatus: apicontainer.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},

		{
			Name:            "Nothing running, pull depends on resources provisioned",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerStatusNone,
			SatisfiedStatus: apicontainer.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Nothing running, create depends on running",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerStatusNone,
			SatisfiedStatus: apicontainer.ContainerRunning,
		},
		{
			Name:            "Dependency created, pull depends on running",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerCreated,
			SatisfiedStatus: apicontainer.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency created, pull depends on resources provisioned",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerCreated,
			SatisfiedStatus: apicontainer.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency running, pull depends on running",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerRunning,
			SatisfiedStatus: apicontainer.ContainerRunning,
		},
		{
			Name:            "Dependency running, pull depends on resources provisioned",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerRunning,
			SatisfiedStatus: apicontainer.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency resources provisioned, pull depends on resources provisioned",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerResourcesProvisioned,
			SatisfiedStatus: apicontainer.ContainerResourcesProvisioned,
		},
		{
			Name:            "Dependency running, create depends on created",
			TargetKnown:     apicontainer.ContainerPulled,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerRunning,
			SatisfiedStatus: apicontainer.ContainerCreated,
		},
		{
			Name:            "Target running, create depends on running",
			TargetKnown:     apicontainer.ContainerRunning,
			TargetDesired:   apicontainer.ContainerRunning,
			TargetNext:      apicontainer.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerRunning,
			SatisfiedStatus: apicontainer.ContainerCreated,
		},
		{
			Name:            "Target pulled, desired stopped",
			TargetKnown:     apicontainer.ContainerPulled,
			TargetDesired:   apicontainer.ContainerStopped,
			TargetNext:      apicontainer.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: apicontainer.ContainerStatusNone,
			SatisfiedStatus: apicontainer.ContainerCreated,
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
				TransitionDependenciesMap: make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet),
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
		Name             string
		TargetKnown      apicontainer.ContainerStatus
		TargetDep        apicontainer.ContainerStatus
		DependencyKnown  taskresource.ResourceStatus
		RequiredStatus   taskresource.ResourceStatus
		ExpectedResolved bool
	}{
		{
			Name:             "resource none,container pull depends on resource created",
			TargetKnown:      apicontainer.ContainerStatusNone,
			TargetDep:        apicontainer.ContainerPulled,
			DependencyKnown:  taskresource.ResourceStatus(0),
			RequiredStatus:   taskresource.ResourceStatus(1),
			ExpectedResolved: false,
		},
		{
			Name:             "resource created,container pull depends on resource created",
			TargetKnown:      apicontainer.ContainerStatusNone,
			TargetDep:        apicontainer.ContainerPulled,
			DependencyKnown:  taskresource.ResourceStatus(1),
			RequiredStatus:   taskresource.ResourceStatus(1),
			ExpectedResolved: true,
		},
		{
			Name:             "resource none,container create depends on resource created",
			TargetKnown:      apicontainer.ContainerStatusNone,
			TargetDep:        apicontainer.ContainerCreated,
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
			target := &apicontainer.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet),
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
		TargetKnown     apicontainer.ContainerStatus
		TargetDep       apicontainer.ContainerStatus
		DependencyKnown taskresource.ResourceStatus
		RequiredStatus  taskresource.ResourceStatus
		ResolvedErr     error
	}{
		{
			Name:            "resource none,container pull depends on resource created",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDep:       apicontainer.ContainerPulled,
			DependencyKnown: taskresource.ResourceStatus(0),
			RequiredStatus:  taskresource.ResourceStatus(1),
			ResolvedErr:     ErrResourceDependencyNotResolved,
		},
		{
			Name:            "resource created,container pull depends on resource created",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDep:       apicontainer.ContainerPulled,
			DependencyKnown: taskresource.ResourceStatus(1),
			RequiredStatus:  taskresource.ResourceStatus(1),
		},
		{
			Name:            "resource none,container create depends on resource created",
			TargetKnown:     apicontainer.ContainerStatusNone,
			TargetDep:       apicontainer.ContainerCreated,
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
			target := &apicontainer.Container{
				KnownStatusUnsafe:         tc.TargetKnown,
				TransitionDependenciesMap: make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet),
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
	target := &apicontainer.Container{
		KnownStatusUnsafe:         apicontainer.ContainerStatusNone,
		TransitionDependenciesMap: make(map[apicontainer.ContainerStatus]apicontainer.TransitionDependencySet),
	}
	target.BuildResourceDependency("resource", taskresource.ResourceStatus(1), apicontainer.ContainerPulled)
	resources := make(map[string]taskresource.TaskResource)
	resources["resource1"] = mockResource // different resource name
	resolved := verifyTransitionDependenciesResolved(target, nil, resources)
	assert.Equal(t, ErrResourceDependencyNotResolved, resolved)
}
