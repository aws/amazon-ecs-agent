//go:build unit

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

package dependencygraph

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-ecs-agent/agent/containerresource"
	"github.com/aws/amazon-ecs-agent/agent/containerresource/containerstatus"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	mock_taskresource "github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	"github.com/aws/aws-sdk-go/aws"
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

func steadyStateContainer(name string, dependsOn []apicontainer.DependsOn, desiredState containerstatus.ContainerStatus, steadyState containerstatus.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.DependsOnUnsafe = dependsOn
	container.DesiredStatusUnsafe = desiredState
	return container
}

func createdContainer(name string, dependsOn []apicontainer.DependsOn, steadyState containerstatus.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.DependsOnUnsafe = dependsOn
	container.DesiredStatusUnsafe = containerstatus.ContainerCreated
	return container
}

func TestValidDependencies(t *testing.T) {
	// Empty task
	task := &apitask.Task{}
	cfg := config.Config{}
	resolveable := ValidDependencies(task, &cfg)
	assert.True(t, resolveable, "The zero dependency graph should resolve")

	task = &apitask.Task{
		Containers: []*apicontainer.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: containerstatus.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task, &cfg)
	assert.True(t, resolveable, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: startCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	db := steadyStateContainer("db", []apicontainer.DependsOn{{ContainerName: "dbdatavolume", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []apicontainer.DependsOn{}, containerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []apicontainer.DependsOn{{ContainerName: "php", Condition: startCondition}, {ContainerName: "htmldata", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []apicontainer.DependsOn{{ContainerName: "sharedcssfiles", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []apicontainer.DependsOn{}, containerstatus.ContainerRunning)

	task = &apitask.Task{
		Containers: []*apicontainer.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolveable = ValidDependencies(task, &cfg)
	assert.True(t, resolveable, "The webserver group should resolve just fine")
}

func TestValidDependenciesWithCycles(t *testing.T) {
	// Unresolveable: cycle
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			steadyStateContainer("a", []apicontainer.DependsOn{{ContainerName: "b", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning),
			steadyStateContainer("b", []apicontainer.DependsOn{{ContainerName: "a", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning),
		},
	}
	cfg := config.Config{}
	resolveable := ValidDependencies(task, &cfg)
	assert.False(t, resolveable, "Cycle should not be resolveable")
}

func TestValidDependenciesWithUnresolvedReference(t *testing.T) {
	// Unresolveable, reference doesn't exist
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning),
		},
	}
	cfg := config.Config{}
	resolveable := ValidDependencies(task, &cfg)
	assert.False(t, resolveable, "Nonexistent reference shouldn't resolve")
}

func TestDependenciesAreResolvedWhenSteadyStateIsRunning(t *testing.T) {
	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: containerstatus.ContainerRunning,
			},
		},
	}
	cfg := config.Config{}
	_, err := DependenciesAreResolved(task.Containers[0], task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: startCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	db := steadyStateContainer("db", []apicontainer.DependsOn{{ContainerName: "dbdatavolume", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []apicontainer.DependsOn{}, containerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []apicontainer.DependsOn{{ContainerName: "php", Condition: startCondition}, {ContainerName: "htmldata", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []apicontainer.DependsOn{{ContainerName: "sharedcssfiles", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []apicontainer.DependsOn{}, containerstatus.ContainerRunning)

	task = &apitask.Task{
		Containers: []*apicontainer.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Shouldn't be resolved; db isn't running")

	_, err = DependenciesAreResolved(db, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Shouldn't be resolved; dbdatavolume isn't created")

	_, err = DependenciesAreResolved(dbdata, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "data volume with no deps should resolve")

	dbdata.SetKnownStatus(containerstatus.ContainerCreated)
	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Php shouldn't run, db is not created")

	db.SetKnownStatus(containerstatus.ContainerCreated)
	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Php shouldn't run, db is not running")

	_, err = DependenciesAreResolved(db, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "db should be resolved, dbdata volume is Created")
	db.SetKnownStatus(containerstatus.ContainerRunning)

	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Php should resolve")
}

func TestRunDependencies(t *testing.T) {
	c1 := &apicontainer.Container{
		Name:              "a",
		KnownStatusUnsafe: containerstatus.ContainerStatusNone,
	}
	c2 := &apicontainer.Container{
		Name:                    "b",
		KnownStatusUnsafe:       containerstatus.ContainerStatusNone,
		DesiredStatusUnsafe:     containerstatus.ContainerCreated,
		SteadyStateDependencies: []string{"a"},
	}
	task := &apitask.Task{Containers: []*apicontainer.Container{c1, c2}}
	cfg := config.Config{}
	_, err := DependenciesAreResolved(c2, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Dependencies should not be resolved")

	task.Containers[1].SetDesiredStatus(containerstatus.ContainerRunning)
	_, err = DependenciesAreResolved(c2, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Dependencies should not be resolved")

	task.Containers[0].SetKnownStatus(containerstatus.ContainerRunning)
	_, err = DependenciesAreResolved(c2, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Dependencies should be resolved")

	task.Containers[1].SetDesiredStatus(containerstatus.ContainerCreated)
	_, err = DependenciesAreResolved(c1, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Dependencies should be resolved")
}

func TestRunDependenciesWhenSteadyStateIsResourcesProvisionedForOneContainer(t *testing.T) {
	// Webserver stack
	php := steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	db := steadyStateContainer("db", []apicontainer.DependsOn{{ContainerName: "dbdatavolume", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []apicontainer.DependsOn{}, containerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []apicontainer.DependsOn{{ContainerName: "php", Condition: createCondition}, {ContainerName: "htmldata", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []apicontainer.DependsOn{{ContainerName: "sharedcssfiles", Condition: createCondition}}, containerstatus.ContainerRunning, containerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []apicontainer.DependsOn{}, containerstatus.ContainerRunning)
	// The Pause container, being added to the webserver stack
	pause := steadyStateContainer("pause", []apicontainer.DependsOn{}, containerstatus.ContainerResourcesProvisioned, containerstatus.ContainerResourcesProvisioned)

	task := &apitask.Task{
		Containers: []*apicontainer.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles, pause,
		},
	}

	cfg := config.Config{}

	// Add a dependency on the pause container for all containers in the webserver stack
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		container.SteadyStateDependencies = []string{"pause"}
		_, err := DependenciesAreResolved(container, task.Containers, "", nil, nil, &cfg)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}

	_, err := DependenciesAreResolved(pause, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Pause container's dependencies should be resolved")

	// Transition pause container to RUNNING
	pause.KnownStatusUnsafe = containerstatus.ContainerRunning
	// Transition dependencies in webserver stack to CREATED/RUNNING state
	dbdata.KnownStatusUnsafe = containerstatus.ContainerCreated
	db.KnownStatusUnsafe = containerstatus.ContainerRunning
	for _, container := range task.Containers {
		if container.Name == "pause" {
			continue
		}
		// Assert that dependencies remain unresolved until the pause container reaches
		// RESOURCES_PROVISIONED
		_, err = DependenciesAreResolved(container, task.Containers, "", nil, nil, &cfg)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}
	pause.KnownStatusUnsafe = containerstatus.ContainerResourcesProvisioned
	// Dependecies should be resolved now that the 'pause' container has
	// transitioned into RESOURCES_PROVISIONED
	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Php should resolve")
}

func TestOnSteadyStateIsResolved(t *testing.T) {
	testcases := []struct {
		TargetDesired containerstatus.ContainerStatus
		RunKnown      containerstatus.ContainerStatus
		Resolved      bool
	}{
		{
			TargetDesired: containerstatus.ContainerStatusNone,
			Resolved:      false,
		},
		{
			TargetDesired: containerstatus.ContainerPulled,
			Resolved:      false,
		},
		{
			TargetDesired: containerstatus.ContainerCreated,
			RunKnown:      containerstatus.ContainerCreated,
			Resolved:      false,
		},
		{
			TargetDesired: containerstatus.ContainerCreated,
			RunKnown:      containerstatus.ContainerRunning,
			Resolved:      true,
		},
		{
			TargetDesired: containerstatus.ContainerCreated,
			RunKnown:      containerstatus.ContainerStopped,
			Resolved:      true,
		},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+R:%s", tc.TargetDesired.String(), tc.RunKnown.String()),
			assertResolved(onSteadyStateIsResolved, tc.TargetDesired, tc.RunKnown, tc.Resolved))
	}
}

func assertResolved(f func(target *apicontainer.Container, dep *apicontainer.Container) bool, targetDesired, depKnown containerstatus.ContainerStatus, expectedResolved bool) func(t *testing.T) {
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
		TargetKnown     containerstatus.ContainerStatus
		TargetDesired   containerstatus.ContainerStatus
		TargetNext      containerstatus.ContainerStatus
		DependencyName  string
		DependencyKnown containerstatus.ContainerStatus
		SatisfiedStatus containerstatus.ContainerStatus
		ResolvedErr     error
	}{
		{
			Name:            "Nothing running, pull depends on running",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStatusNone,
			SatisfiedStatus: containerstatus.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},

		{
			Name:            "Nothing running, pull depends on resources provisioned",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStatusNone,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Nothing running, create depends on running",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStatusNone,
			SatisfiedStatus: containerstatus.ContainerRunning,
		},
		{
			Name:            "Dependency created, pull depends on running",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerCreated,
			SatisfiedStatus: containerstatus.ContainerRunning,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency created, pull depends on resources provisioned",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerCreated,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency running, pull depends on running",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerRunning,
			SatisfiedStatus: containerstatus.ContainerRunning,
		},
		{
			Name:            "Dependency running, pull depends on resources provisioned",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerRunning,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolved,
		},
		{
			Name:            "Dependency resources provisioned, pull depends on resources provisioned",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerPulled,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerResourcesProvisioned,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
		},
		{
			Name:            "Dependency running, create depends on created",
			TargetKnown:     containerstatus.ContainerPulled,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerCreated,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerRunning,
			SatisfiedStatus: containerstatus.ContainerCreated,
		},
		{
			Name:            "Target running, create depends on running",
			TargetKnown:     containerstatus.ContainerRunning,
			TargetDesired:   containerstatus.ContainerRunning,
			TargetNext:      containerstatus.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerRunning,
			SatisfiedStatus: containerstatus.ContainerCreated,
		},
		{
			Name:            "Target pulled, desired stopped",
			TargetKnown:     containerstatus.ContainerPulled,
			TargetDesired:   containerstatus.ContainerStopped,
			TargetNext:      containerstatus.ContainerRunning,
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStatusNone,
			SatisfiedStatus: containerstatus.ContainerCreated,
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
				TransitionDependenciesMap: make(map[containerstatus.ContainerStatus]containerresource.TransitionDependencySet),
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
		TargetKnown     containerstatus.ContainerStatus
		TargetDep       containerstatus.ContainerStatus
		DependencyKnown resourcestatus.ResourceStatus
		RequiredStatus  resourcestatus.ResourceStatus

		ExpectedResolved bool
	}{
		{
			Name:             "resource none,container pull depends on resource created",
			TargetKnown:      containerstatus.ContainerStatusNone,
			TargetDep:        containerstatus.ContainerPulled,
			DependencyKnown:  resourcestatus.ResourceStatus(0),
			RequiredStatus:   resourcestatus.ResourceStatus(1),
			ExpectedResolved: false,
		},
		{
			Name:             "resource created,container pull depends on resource created",
			TargetKnown:      containerstatus.ContainerStatusNone,
			TargetDep:        containerstatus.ContainerPulled,
			DependencyKnown:  resourcestatus.ResourceStatus(1),
			RequiredStatus:   resourcestatus.ResourceStatus(1),
			ExpectedResolved: true,
		},
		{
			Name:             "resource none,container create depends on resource created",
			TargetKnown:      containerstatus.ContainerStatusNone,
			TargetDep:        containerstatus.ContainerCreated,
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
				TransitionDependenciesMap: make(map[containerstatus.ContainerStatus]containerresource.TransitionDependencySet),
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
		TargetKnown     containerstatus.ContainerStatus
		TargetDep       containerstatus.ContainerStatus
		DependencyKnown resourcestatus.ResourceStatus
		RequiredStatus  resourcestatus.ResourceStatus
		ResolvedErr     error
	}{
		{
			Name:            "resource none,container pull depends on resource created",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDep:       containerstatus.ContainerPulled,
			DependencyKnown: resourcestatus.ResourceStatus(0),
			RequiredStatus:  resourcestatus.ResourceStatus(1),
			ResolvedErr:     ErrResourceDependencyNotResolved,
		},
		{
			Name:            "resource created,container pull depends on resource created",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDep:       containerstatus.ContainerPulled,
			DependencyKnown: resourcestatus.ResourceStatus(1),
			RequiredStatus:  resourcestatus.ResourceStatus(1),
		},
		{
			Name:            "resource none,container create depends on resource created",
			TargetKnown:     containerstatus.ContainerStatusNone,
			TargetDep:       containerstatus.ContainerCreated,
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
				TransitionDependenciesMap: make(map[containerstatus.ContainerStatus]containerresource.TransitionDependencySet),
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
		KnownStatusUnsafe:         containerstatus.ContainerStatusNone,
		TransitionDependenciesMap: make(map[containerstatus.ContainerStatus]containerresource.TransitionDependencySet),
	}
	target.BuildResourceDependency("resource", resourcestatus.ResourceStatus(1), containerstatus.ContainerPulled)

	resources := make(map[string]taskresource.TaskResource)
	resources["resource1"] = mockResource // different resource name
	resolved := verifyTransitionDependenciesResolved(target, nil, resources)
	assert.Equal(t, ErrResourceDependencyNotResolved, resolved)
}

// TestVerifyTransitionDependenciesResolvedForResource verifies the logic of function TaskResourceDependenciesAreResolved
func TestVerifyTransitionDependenciesResolvedForResource(t *testing.T) {
	testcases := []struct {
		Name            string
		TargetKnown     resourcestatus.ResourceStatus
		TargetDesired   resourcestatus.ResourceStatus
		TargetNext      resourcestatus.ResourceStatus
		DependencyName  string
		DependencyKnown containerstatus.ContainerStatus
		SatisfiedStatus containerstatus.ContainerStatus
		ResolvedErr     error
	}{
		{
			Name:            "desired created, resource created depends on container resources provisioned",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(1),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStatusNone,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolvedForResource,
		},
		{
			Name:            "desired removed, resource created depends on container resources provisioned",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(2),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStatusNone,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolvedForResource,
		},
		{
			Name:            "desired created, resource created depends on resourcesProvisioned, dependency resourcesProvisioned",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(1),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerResourcesProvisioned,
			SatisfiedStatus: containerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     nil,
		},
		{
			Name:            "desired created, resource created depends on created, dependency passed created",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(1),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerRunning,
			SatisfiedStatus: containerstatus.ContainerCreated,
			ResolvedErr:     nil,
		},
		{
			Name:            "desired removed, resource removed depends on stopped, dependency resourceProvisioned",
			TargetKnown:     resourcestatus.ResourceStatus(1),
			TargetDesired:   resourcestatus.ResourceStatus(2),
			TargetNext:      resourcestatus.ResourceStatus(2),
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerResourcesProvisioned,
			SatisfiedStatus: containerstatus.ContainerStopped,
			ResolvedErr:     ErrContainerDependencyNotResolvedForResource,
		},
		{
			Name:            "desired removed, resource removed depends on stopped, dependency stopped",
			TargetKnown:     resourcestatus.ResourceStatus(1),
			TargetDesired:   resourcestatus.ResourceStatus(2),
			TargetNext:      resourcestatus.ResourceStatus(2),
			DependencyName:  "container",
			DependencyKnown: containerstatus.ContainerStopped,
			SatisfiedStatus: containerstatus.ContainerStopped,
			ResolvedErr:     nil,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockResource := mock_taskresource.NewMockTaskResource(ctrl)
			con := &apicontainer.Container{
				Name:              tc.DependencyName,
				KnownStatusUnsafe: tc.DependencyKnown,
			}
			containers := []*apicontainer.Container{
				con,
			}
			containerDep := []containerresource.ContainerDependency{
				{
					ContainerName:   tc.DependencyName,
					SatisfiedStatus: tc.SatisfiedStatus,
				},
			}
			gomock.InOrder(
				mockResource.EXPECT().NextKnownState().Return(tc.TargetNext),
				mockResource.EXPECT().GetContainerDependencies(tc.TargetNext).Return(containerDep),
			)
			resolved := TaskResourceDependenciesAreResolved(mockResource, containers)
			assert.Equal(t, tc.ResolvedErr, resolved)
		})
	}

}

// TestResourceTransitionDependencyNotFound verifies if error is thrown when the container dependency for resource
// is not found in the list of task containers
func TestResourceTransitionDependencyNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	containerDep := []containerresource.ContainerDependency{
		{
			ContainerName:   "container2",
			SatisfiedStatus: containerstatus.ContainerStopped,
		},
	}
	con := &apicontainer.Container{
		Name:                      "container1",
		KnownStatusUnsafe:         containerstatus.ContainerStopped,
		TransitionDependenciesMap: make(map[containerstatus.ContainerStatus]containerresource.TransitionDependencySet),
	}
	existingContainers := []*apicontainer.Container{
		con,
	}
	gomock.InOrder(
		mockResource.EXPECT().NextKnownState().Return(resourcestatus.ResourceStatus(2)),
		mockResource.EXPECT().GetContainerDependencies(resourcestatus.ResourceStatus(2)).Return(containerDep),
	)
	resolved := TaskResourceDependenciesAreResolved(mockResource, existingContainers)
	assert.Equal(t, ErrContainerDependencyNotResolvedForResource, resolved)
}

func TestResourceTransitionDependencyNoDependency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockResource := mock_taskresource.NewMockTaskResource(ctrl)
	con := &apicontainer.Container{
		Name:                      "container1",
		KnownStatusUnsafe:         containerstatus.ContainerStopped,
		TransitionDependenciesMap: make(map[containerstatus.ContainerStatus]containerresource.TransitionDependencySet),
	}
	existingContainers := []*apicontainer.Container{
		con,
	}
	gomock.InOrder(
		mockResource.EXPECT().NextKnownState().Return(resourcestatus.ResourceStatus(2)),
		mockResource.EXPECT().GetContainerDependencies(resourcestatus.ResourceStatus(2)).Return(nil),
	)
	assert.Nil(t, TaskResourceDependenciesAreResolved(mockResource, existingContainers))
}

func TestContainerOrderingCanResolve(t *testing.T) {
	testcases := []struct {
		TargetDesired       containerstatus.ContainerStatus
		DependencyDesired   containerstatus.ContainerStatus
		DependencyKnown     containerstatus.ContainerStatus
		DependencyCondition string
		ExitCode            int
		Resolvable          bool
	}{
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyDesired:   containerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyDesired:   containerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyDesired:   containerstatus.ContainerZombie,
			DependencyCondition: createCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerCreated,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerRunning,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyDesired:   containerstatus.ContainerCreated,
			DependencyCondition: startCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyDesired:   containerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerZombie,
			DependencyCondition: startCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerStopped,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerRunning,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerStopped,
			ExitCode:            0,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			ExitCode:            0,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			ExitCode:            1,
			DependencyCondition: successCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyDesired:   containerstatus.ContainerRunning,
			DependencyCondition: completeCondition,
			Resolvable:          true,
		},
	}
	cfg := config.Config{}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+V:%s", tc.TargetDesired.String(), tc.DependencyDesired.String()),
			assertContainerOrderingCanResolve(containerOrderingDependenciesCanResolve, tc.TargetDesired, tc.DependencyDesired, tc.DependencyKnown, tc.DependencyCondition, tc.ExitCode, tc.Resolvable, &cfg))
	}
}

func TestContainerOrderingIsResolved(t *testing.T) {
	testcases := []struct {
		TargetDesired       containerstatus.ContainerStatus
		DependencyKnown     containerstatus.ContainerStatus
		DependencyCondition string
		Resolved            bool
		ExitCode            int
	}{
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerCreated,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerCreated,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerCreated,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerZombie,
			DependencyCondition: startCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerZombie,
			DependencyCondition: startCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: successCondition,
			ExitCode:            0,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: successCondition,
			ExitCode:            1,
			Resolved:            false,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			ExitCode:            0,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			ExitCode:            1,
			Resolved:            true,
		},
		{
			TargetDesired:       containerstatus.ContainerRunning,
			DependencyKnown:     containerstatus.ContainerRunning,
			DependencyCondition: completeCondition,
			Resolved:            false,
		},
	}
	cfg := config.Config{}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:%s+V:%s", tc.TargetDesired.String(), tc.DependencyKnown.String()),
			assertContainerOrderingResolved(containerOrderingDependenciesIsResolved, tc.TargetDesired, tc.DependencyKnown, tc.DependencyCondition, tc.ExitCode, tc.Resolved, &cfg))
	}
}

func TestContainerOrderingIsResolvedWithDependentContainersPullUpfront(t *testing.T) {
	testcases := []struct {
		TargetKnown         containerstatus.ContainerStatus
		DependencyKnown     containerstatus.ContainerStatus
		DependencyCondition string
		Resolved            bool
		ExitCode            int
	}{
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerPulled,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerCreated,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: startCondition,
			Resolved:            false,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerPulled,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerCreated,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerRunning,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerStopped,
			ExitCode:            1,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerPulled,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: successCondition,
			Resolved:            false,
		},
		{
			TargetKnown:         containerstatus.ContainerPulled,
			DependencyKnown:     containerstatus.ContainerStopped,
			ExitCode:            1,
			DependencyCondition: successCondition,
			Resolved:            false,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerPulled,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerCreated,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerRunning,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerStatusNone,
			DependencyKnown:     containerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         containerstatus.ContainerPulled,
			DependencyKnown:     containerstatus.ContainerStatusNone,
			DependencyCondition: completeCondition,
			Resolved:            false,
		},
	}

	cfg := config.Config{
		DependentContainersPullUpfront: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("DependencyCondition:%s+T:%s+V:%s", tc.DependencyCondition, tc.TargetKnown.String(), tc.DependencyKnown.String()),
			assertContainerTargetOrderingResolved(containerOrderingDependenciesIsResolved, tc.TargetKnown, tc.DependencyKnown, tc.DependencyCondition, tc.ExitCode, tc.Resolved, &cfg))
	}
}

func assertContainerTargetOrderingResolved(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetKnown, depKnown containerstatus.ContainerStatus, depCond string, exitcode int, expectedResolvable bool, cfg *config.Config) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			KnownStatusUnsafe: targetKnown,
		}
		dep := &apicontainer.Container{
			KnownStatusUnsafe:   depKnown,
			KnownExitCodeUnsafe: aws.Int(exitcode),
		}

		resolvable := f(target, dep, depCond, cfg)
		assert.Equal(t, expectedResolvable, resolvable)
	}
}

func assertContainerOrderingCanResolve(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetDesired, depDesired, depKnown containerstatus.ContainerStatus, depCond string, exitcode int, expectedResolvable bool, cfg *config.Config) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			DesiredStatusUnsafe: targetDesired,
		}
		dep := &apicontainer.Container{
			DesiredStatusUnsafe: depDesired,
			KnownStatusUnsafe:   depKnown,
			KnownExitCodeUnsafe: aws.Int(exitcode),
		}
		resolvable := f(target, dep, depCond, cfg)
		assert.Equal(t, expectedResolvable, resolvable)
	}
}

func assertContainerOrderingResolved(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetDesired, depKnown containerstatus.ContainerStatus, depCond string, exitcode int, expectedResolved bool, cfg *config.Config) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			DesiredStatusUnsafe: targetDesired,
		}
		dep := &apicontainer.Container{
			KnownStatusUnsafe:   depKnown,
			KnownExitCodeUnsafe: aws.Int(exitcode),
		}
		resolved := f(target, dep, depCond, cfg)
		assert.Equal(t, expectedResolved, resolved)
	}
}

func TestContainerOrderingHealthyConditionIsResolved(t *testing.T) {
	testcases := []struct {
		TargetDesired                 containerstatus.ContainerStatus
		TargetKnown                   containerstatus.ContainerStatus
		DependencyKnownHealthStatus   containerstatus.ContainerHealthStatus
		HealthCheckType               string
		DependencyKnownHealthExitCode int
		DependencyCondition           string
		Resolved                      bool
	}{
		{
			TargetDesired:               containerstatus.ContainerCreated,
			DependencyKnownHealthStatus: containerstatus.ContainerHealthy,
			HealthCheckType:             "docker",
			DependencyCondition:         healthyCondition,
			Resolved:                    true,
		},
		{
			TargetDesired:                 containerstatus.ContainerCreated,
			DependencyKnownHealthStatus:   containerstatus.ContainerUnhealthy,
			HealthCheckType:               "docker",
			DependencyKnownHealthExitCode: 1,
			DependencyCondition:           healthyCondition,
			Resolved:                      false,
		},
		{
			TargetDesired: containerstatus.ContainerCreated,
			Resolved:      false,
		},
	}
	cfg := config.Config{}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("DependencyKnownHealthStatus:%s+T:%s+V:%s", tc.DependencyKnownHealthStatus, tc.TargetDesired.String(), tc.DependencyKnownHealthStatus.String()),
			assertContainerOrderingHealthyConditionResolved(containerOrderingDependenciesIsResolved, tc.TargetDesired, tc.TargetKnown, tc.DependencyKnownHealthStatus, tc.HealthCheckType, tc.DependencyKnownHealthExitCode, tc.DependencyCondition, tc.Resolved, &cfg))
	}
}

func TestContainerOrderingHealthyConditionIsResolvedWithDependentContainersPullUpfront(t *testing.T) {
	testcases := []struct {
		TargetDesired                 containerstatus.ContainerStatus
		TargetKnown                   containerstatus.ContainerStatus
		DependencyKnownHealthStatus   containerstatus.ContainerHealthStatus
		HealthCheckType               string
		DependencyKnownHealthExitCode int
		DependencyCondition           string
		Resolved                      bool
	}{
		{
			TargetKnown:                 containerstatus.ContainerStatusNone,
			DependencyKnownHealthStatus: containerstatus.ContainerHealthy,
			HealthCheckType:             "docker",
			DependencyCondition:         healthyCondition,
			Resolved:                    true,
		},
		{
			TargetKnown:                   containerstatus.ContainerStatusNone,
			DependencyKnownHealthStatus:   containerstatus.ContainerUnhealthy,
			HealthCheckType:               "docker",
			DependencyKnownHealthExitCode: 1,
			DependencyCondition:           healthyCondition,
			Resolved:                      true,
		},
		{
			TargetKnown:                   containerstatus.ContainerPulled,
			DependencyKnownHealthStatus:   containerstatus.ContainerUnhealthy,
			HealthCheckType:               "docker",
			DependencyKnownHealthExitCode: 1,
			DependencyCondition:           healthyCondition,
			Resolved:                      false,
		},
	}
	cfg := config.Config{
		DependentContainersPullUpfront: config.BooleanDefaultFalse{Value: config.ExplicitlyEnabled},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("DependencyKnownHealthStatus:%s+T:%s+V:%s", tc.DependencyKnownHealthStatus, tc.TargetKnown.String(), tc.DependencyKnownHealthStatus.String()),
			assertContainerOrderingHealthyConditionResolved(containerOrderingDependenciesIsResolved, tc.TargetDesired, tc.TargetKnown, tc.DependencyKnownHealthStatus, tc.HealthCheckType, tc.DependencyKnownHealthExitCode, tc.DependencyCondition, tc.Resolved, &cfg))
	}
}

func assertContainerOrderingHealthyConditionResolved(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetDesired containerstatus.ContainerStatus, targetKnown containerstatus.ContainerStatus, depHealthKnown containerstatus.ContainerHealthStatus, healthCheckEnabled string, depHealthKnownExitCode int, depCond string, expectedResolved bool, cfg *config.Config) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			DesiredStatusUnsafe: targetDesired,
			KnownStatusUnsafe:   targetKnown,
		}
		dep := &apicontainer.Container{
			Health: containerresource.HealthStatus{
				Status:   depHealthKnown,
				ExitCode: depHealthKnownExitCode,
			},
			HealthCheckType: healthCheckEnabled,
		}
		resolved := f(target, dep, depCond, cfg)
		assert.Equal(t, expectedResolved, resolved)
	}
}

func dependsOn(vals ...string) []apicontainer.DependsOn {
	d := make([]apicontainer.DependsOn, len(vals))
	for i, val := range vals {
		d[i] = apicontainer.DependsOn{ContainerName: val}
	}
	return d
}

// TestVerifyShutdownOrder validates that the shutdown graph works in the inverse order of the startup graph
// This test always uses a running target
func TestVerifyShutdownOrder(t *testing.T) {

	// dependencies aren't transitive, so we can check multiple levels within here
	others := map[string]*apicontainer.Container{
		"A": {
			Name:              "A",
			DependsOnUnsafe:   dependsOn("B"),
			KnownStatusUnsafe: containerstatus.ContainerStopped,
		},
		"B": {
			Name:              "B",
			DependsOnUnsafe:   dependsOn("C", "D"),
			KnownStatusUnsafe: containerstatus.ContainerRunning,
		},
		"C": {
			Name:              "C",
			DependsOnUnsafe:   dependsOn("E", "F"),
			KnownStatusUnsafe: containerstatus.ContainerStopped,
		},
		"D": {
			Name:              "D",
			DependsOnUnsafe:   dependsOn("E"),
			KnownStatusUnsafe: containerstatus.ContainerRunning,
		},
	}

	testCases := []struct {
		TestName      string
		TargetName    string
		ShouldResolve bool
	}{
		{
			TestName:      "Dependency is already stopped",
			TargetName:    "F",
			ShouldResolve: true,
		},
		{
			TestName:      "Running with a running dependency",
			TargetName:    "D",
			ShouldResolve: false,
		},
		{
			TestName:      "One dependency running, one stopped",
			TargetName:    "E",
			ShouldResolve: false,
		},
		{
			TestName:      "No dependencies declared",
			TargetName:    "Z",
			ShouldResolve: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.TestName, func(t *testing.T) {
			// create a target container
			target := &apicontainer.Container{
				Name:                tc.TargetName,
				KnownStatusUnsafe:   containerstatus.ContainerRunning,
				DesiredStatusUnsafe: containerstatus.ContainerStopped,
			}

			// Validation
			if tc.ShouldResolve {
				assert.NoError(t, verifyShutdownOrder(target, others))
			} else {
				assert.Error(t, verifyShutdownOrder(target, others))
			}
		})
	}
}

func TestStartTimeoutForContainerOrdering(t *testing.T) {
	testcases := []struct {
		DependencyStartedAt    time.Time
		DependencyStartTimeout uint
		DependencyCondition    string
		ExpectedTimedOut       bool
	}{
		{
			DependencyStartedAt:    time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			DependencyStartTimeout: 10,
			DependencyCondition:    healthyCondition,
			ExpectedTimedOut:       true,
		},
		{
			DependencyStartedAt:    time.Date(4000, 1, 1, 0, 0, 0, 0, time.UTC),
			DependencyStartTimeout: 10,
			DependencyCondition:    successCondition,
			ExpectedTimedOut:       false,
		},
		{
			DependencyStartedAt:    time.Time{},
			DependencyStartTimeout: 10,
			DependencyCondition:    successCondition,
			ExpectedTimedOut:       false,
		},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("T:dependency time out: %v", tc.ExpectedTimedOut),
			assertContainerOrderingNotTimedOut(hasDependencyTimedOut, tc.DependencyStartedAt, tc.DependencyStartTimeout, tc.DependencyCondition, tc.ExpectedTimedOut))
	}
}

func assertContainerOrderingNotTimedOut(f func(dep *apicontainer.Container, depCond string) bool, startedAt time.Time, timeout uint, depCond string, expectedTimedOut bool) func(t *testing.T) {
	return func(t *testing.T) {
		dep := &apicontainer.Container{
			Name:         "dep",
			StartTimeout: timeout,
		}

		dep.SetStartedAt(startedAt)

		timedOut := f(dep, depCond)
		assert.Equal(t, expectedTimedOut, timedOut)
	}
}

func TestVerifyContainerOrderingStatusResolvableFailOnDependencyWontStart(t *testing.T) {
	targetName := "target"
	dependencyName := "dependency"
	target := &apicontainer.Container{
		Name:                targetName,
		KnownStatusUnsafe:   containerstatus.ContainerPulled,
		DesiredStatusUnsafe: containerstatus.ContainerCreated,
		DependsOnUnsafe: []apicontainer.DependsOn{
			{
				ContainerName: dependencyName,
			},
		},
	}
	dep := &apicontainer.Container{
		Name:                dependencyName,
		KnownStatusUnsafe:   containerstatus.ContainerPulled,
		DesiredStatusUnsafe: containerstatus.ContainerStopped,
		AppliedStatus:       containerstatus.ContainerStatusNone,
	}
	contMap := map[string]*apicontainer.Container{
		targetName:     target,
		dependencyName: dep,
	}
	dummyResolves := func(*apicontainer.Container, *apicontainer.Container, string, *config.Config) bool {
		return true
	}
	_, err := verifyContainerOrderingStatusResolvable(target, contMap, &config.Config{}, dummyResolves)
	assert.Error(t, err)
}
