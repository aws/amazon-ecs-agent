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

package dependencygraph

import (
	"fmt"
	"testing"
	"time"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apitask "github.com/aws/amazon-ecs-agent/agent/api/task"
	"github.com/aws/amazon-ecs-agent/agent/config"
	"github.com/aws/amazon-ecs-agent/agent/taskresource"
	mock_taskresource "github.com/aws/amazon-ecs-agent/agent/taskresource/mocks"
	resourcestatus "github.com/aws/amazon-ecs-agent/agent/taskresource/status"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/ecs-agent/api/container/status"
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

func steadyStateContainer(name string, dependsOn []apicontainer.DependsOn, desiredState apicontainerstatus.ContainerStatus, steadyState apicontainerstatus.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.DependsOnUnsafe = dependsOn
	container.DesiredStatusUnsafe = desiredState
	return container
}

func createdContainer(name string, dependsOn []apicontainer.DependsOn, steadyState apicontainerstatus.ContainerStatus) *apicontainer.Container {
	container := apicontainer.NewContainerWithSteadyState(steadyState)
	container.Name = name
	container.DependsOnUnsafe = dependsOn
	container.DesiredStatusUnsafe = apicontainerstatus.ContainerCreated
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
				DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task, &cfg)
	assert.True(t, resolveable, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: startCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	db := steadyStateContainer("db", []apicontainer.DependsOn{{ContainerName: "dbdatavolume", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []apicontainer.DependsOn{}, apicontainerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []apicontainer.DependsOn{{ContainerName: "php", Condition: startCondition}, {ContainerName: "htmldata", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []apicontainer.DependsOn{{ContainerName: "sharedcssfiles", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []apicontainer.DependsOn{}, apicontainerstatus.ContainerRunning)

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
			steadyStateContainer("a", []apicontainer.DependsOn{{ContainerName: "b", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning),
			steadyStateContainer("b", []apicontainer.DependsOn{{ContainerName: "a", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning),
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
			steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning),
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
				DesiredStatusUnsafe: apicontainerstatus.ContainerRunning,
			},
		},
	}
	cfg := config.Config{}
	_, err := DependenciesAreResolved(task.Containers[0], task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "One container should resolve trivially")

	// Webserver stack
	php := steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: startCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	db := steadyStateContainer("db", []apicontainer.DependsOn{{ContainerName: "dbdatavolume", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []apicontainer.DependsOn{}, apicontainerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []apicontainer.DependsOn{{ContainerName: "php", Condition: startCondition}, {ContainerName: "htmldata", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []apicontainer.DependsOn{{ContainerName: "sharedcssfiles", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []apicontainer.DependsOn{}, apicontainerstatus.ContainerRunning)

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

	dbdata.SetKnownStatus(apicontainerstatus.ContainerCreated)
	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Php shouldn't run, db is not created")

	db.SetKnownStatus(apicontainerstatus.ContainerCreated)
	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Php shouldn't run, db is not running")

	_, err = DependenciesAreResolved(db, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "db should be resolved, dbdata volume is Created")
	db.SetKnownStatus(apicontainerstatus.ContainerRunning)

	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
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
	cfg := config.Config{}
	_, err := DependenciesAreResolved(c2, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Dependencies should not be resolved")

	task.Containers[1].SetDesiredStatus(apicontainerstatus.ContainerRunning)
	_, err = DependenciesAreResolved(c2, task.Containers, "", nil, nil, &cfg)
	assert.Error(t, err, "Dependencies should not be resolved")

	task.Containers[0].SetKnownStatus(apicontainerstatus.ContainerRunning)
	_, err = DependenciesAreResolved(c2, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Dependencies should be resolved")

	task.Containers[1].SetDesiredStatus(apicontainerstatus.ContainerCreated)
	_, err = DependenciesAreResolved(c1, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Dependencies should be resolved")
}

func TestRunDependenciesWhenSteadyStateIsResourcesProvisionedForOneContainer(t *testing.T) {
	// Webserver stack
	php := steadyStateContainer("php", []apicontainer.DependsOn{{ContainerName: "db", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	db := steadyStateContainer("db", []apicontainer.DependsOn{{ContainerName: "dbdatavolume", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	dbdata := createdContainer("dbdatavolume", []apicontainer.DependsOn{}, apicontainerstatus.ContainerRunning)
	webserver := steadyStateContainer("webserver", []apicontainer.DependsOn{{ContainerName: "php", Condition: createCondition}, {ContainerName: "htmldata", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	htmldata := steadyStateContainer("htmldata", []apicontainer.DependsOn{{ContainerName: "sharedcssfiles", Condition: createCondition}}, apicontainerstatus.ContainerRunning, apicontainerstatus.ContainerRunning)
	sharedcssfiles := createdContainer("sharedcssfiles", []apicontainer.DependsOn{}, apicontainerstatus.ContainerRunning)
	// The Pause container, being added to the webserver stack
	pause := steadyStateContainer("pause", []apicontainer.DependsOn{}, apicontainerstatus.ContainerResourcesProvisioned, apicontainerstatus.ContainerResourcesProvisioned)

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
		_, err = DependenciesAreResolved(container, task.Containers, "", nil, nil, &cfg)
		assert.Error(t, err, "Shouldn't be resolved; pause isn't running")
	}
	pause.KnownStatusUnsafe = apicontainerstatus.ContainerResourcesProvisioned
	// Dependecies should be resolved now that the 'pause' container has
	// transitioned into RESOURCES_PROVISIONED
	_, err = DependenciesAreResolved(php, task.Containers, "", nil, nil, &cfg)
	assert.NoError(t, err, "Php should resolve")
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

// TestVerifyTransitionDependenciesResolvedForResource verifies the logic of function TaskResourceDependenciesAreResolved
func TestVerifyTransitionDependenciesResolvedForResource(t *testing.T) {
	testcases := []struct {
		Name            string
		TargetKnown     resourcestatus.ResourceStatus
		TargetDesired   resourcestatus.ResourceStatus
		TargetNext      resourcestatus.ResourceStatus
		DependencyName  string
		DependencyKnown apicontainerstatus.ContainerStatus
		SatisfiedStatus apicontainerstatus.ContainerStatus
		ResolvedErr     error
	}{
		{
			Name:            "desired created, resource created depends on container resources provisioned",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(1),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStatusNone,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolvedForResource,
		},
		{
			Name:            "desired removed, resource created depends on container resources provisioned",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(2),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStatusNone,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     ErrContainerDependencyNotResolvedForResource,
		},
		{
			Name:            "desired created, resource created depends on resourcesProvisioned, dependency resourcesProvisioned",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(1),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerResourcesProvisioned,
			SatisfiedStatus: apicontainerstatus.ContainerResourcesProvisioned,
			ResolvedErr:     nil,
		},
		{
			Name:            "desired created, resource created depends on created, dependency passed created",
			TargetKnown:     resourcestatus.ResourceStatus(0),
			TargetDesired:   resourcestatus.ResourceStatus(1),
			TargetNext:      resourcestatus.ResourceStatus(1),
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerRunning,
			SatisfiedStatus: apicontainerstatus.ContainerCreated,
			ResolvedErr:     nil,
		},
		{
			Name:            "desired removed, resource removed depends on stopped, dependency resourceProvisioned",
			TargetKnown:     resourcestatus.ResourceStatus(1),
			TargetDesired:   resourcestatus.ResourceStatus(2),
			TargetNext:      resourcestatus.ResourceStatus(2),
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerResourcesProvisioned,
			SatisfiedStatus: apicontainerstatus.ContainerStopped,
			ResolvedErr:     ErrContainerDependencyNotResolvedForResource,
		},
		{
			Name:            "desired removed, resource removed depends on stopped, dependency stopped",
			TargetKnown:     resourcestatus.ResourceStatus(1),
			TargetDesired:   resourcestatus.ResourceStatus(2),
			TargetNext:      resourcestatus.ResourceStatus(2),
			DependencyName:  "container",
			DependencyKnown: apicontainerstatus.ContainerStopped,
			SatisfiedStatus: apicontainerstatus.ContainerStopped,
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
			containerDep := []apicontainer.ContainerDependency{
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
	containerDep := []apicontainer.ContainerDependency{
		{
			ContainerName:   "container2",
			SatisfiedStatus: apicontainerstatus.ContainerStopped,
		},
	}
	con := &apicontainer.Container{
		Name:                      "container1",
		KnownStatusUnsafe:         apicontainerstatus.ContainerStopped,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
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
		KnownStatusUnsafe:         apicontainerstatus.ContainerStopped,
		TransitionDependenciesMap: make(map[apicontainerstatus.ContainerStatus]apicontainer.TransitionDependencySet),
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
		TargetDesired       apicontainerstatus.ContainerStatus
		DependencyDesired   apicontainerstatus.ContainerStatus
		DependencyKnown     apicontainerstatus.ContainerStatus
		DependencyCondition string
		ExitCode            int
		Resolvable          bool
	}{
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyDesired:   apicontainerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyDesired:   apicontainerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyDesired:   apicontainerstatus.ContainerZombie,
			DependencyCondition: createCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerCreated,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerRunning,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyDesired:   apicontainerstatus.ContainerCreated,
			DependencyCondition: startCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyDesired:   apicontainerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerZombie,
			DependencyCondition: startCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerStopped,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerRunning,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerStopped,
			ExitCode:            0,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			ExitCode:            0,
			DependencyCondition: successCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			ExitCode:            1,
			DependencyCondition: successCondition,
			Resolvable:          false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			Resolvable:          true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyDesired:   apicontainerstatus.ContainerRunning,
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
		TargetDesired       apicontainerstatus.ContainerStatus
		DependencyKnown     apicontainerstatus.ContainerStatus
		DependencyCondition string
		Resolved            bool
		ExitCode            int
	}{
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerCreated,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: createCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerCreated,
			DependencyCondition: createCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerCreated,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerZombie,
			DependencyCondition: startCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerRunning,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerZombie,
			DependencyCondition: startCondition,
			Resolved:            false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerPulled,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: successCondition,
			ExitCode:            0,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: successCondition,
			ExitCode:            1,
			Resolved:            false,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			ExitCode:            0,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			ExitCode:            1,
			Resolved:            true,
		},
		{
			TargetDesired:       apicontainerstatus.ContainerRunning,
			DependencyKnown:     apicontainerstatus.ContainerRunning,
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
		TargetKnown         apicontainerstatus.ContainerStatus
		DependencyKnown     apicontainerstatus.ContainerStatus
		DependencyCondition string
		Resolved            bool
		ExitCode            int
	}{
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerPulled,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: startCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerCreated,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: startCondition,
			Resolved:            false,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerPulled,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerCreated,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerRunning,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			ExitCode:            1,
			DependencyCondition: successCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerPulled,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: successCondition,
			Resolved:            false,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerPulled,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			ExitCode:            1,
			DependencyCondition: successCondition,
			Resolved:            false,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerPulled,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerCreated,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerRunning,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerStatusNone,
			DependencyKnown:     apicontainerstatus.ContainerStopped,
			DependencyCondition: completeCondition,
			Resolved:            true,
		},
		{
			TargetKnown:         apicontainerstatus.ContainerPulled,
			DependencyKnown:     apicontainerstatus.ContainerStatusNone,
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

func assertContainerTargetOrderingResolved(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetKnown, depKnown apicontainerstatus.ContainerStatus, depCond string, exitcode int, expectedResolvable bool, cfg *config.Config) func(t *testing.T) {
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

func assertContainerOrderingCanResolve(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetDesired, depDesired, depKnown apicontainerstatus.ContainerStatus, depCond string, exitcode int, expectedResolvable bool, cfg *config.Config) func(t *testing.T) {
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

func assertContainerOrderingResolved(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetDesired, depKnown apicontainerstatus.ContainerStatus, depCond string, exitcode int, expectedResolved bool, cfg *config.Config) func(t *testing.T) {
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
		TargetDesired                 apicontainerstatus.ContainerStatus
		TargetKnown                   apicontainerstatus.ContainerStatus
		DependencyKnownHealthStatus   apicontainerstatus.ContainerHealthStatus
		HealthCheckType               string
		DependencyKnownHealthExitCode int
		DependencyCondition           string
		Resolved                      bool
	}{
		{
			TargetDesired:               apicontainerstatus.ContainerCreated,
			DependencyKnownHealthStatus: apicontainerstatus.ContainerHealthy,
			HealthCheckType:             "docker",
			DependencyCondition:         healthyCondition,
			Resolved:                    true,
		},
		{
			TargetDesired:                 apicontainerstatus.ContainerCreated,
			DependencyKnownHealthStatus:   apicontainerstatus.ContainerUnhealthy,
			HealthCheckType:               "docker",
			DependencyKnownHealthExitCode: 1,
			DependencyCondition:           healthyCondition,
			Resolved:                      false,
		},
		{
			TargetDesired: apicontainerstatus.ContainerCreated,
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
		TargetDesired                 apicontainerstatus.ContainerStatus
		TargetKnown                   apicontainerstatus.ContainerStatus
		DependencyKnownHealthStatus   apicontainerstatus.ContainerHealthStatus
		HealthCheckType               string
		DependencyKnownHealthExitCode int
		DependencyCondition           string
		Resolved                      bool
	}{
		{
			TargetKnown:                 apicontainerstatus.ContainerStatusNone,
			DependencyKnownHealthStatus: apicontainerstatus.ContainerHealthy,
			HealthCheckType:             "docker",
			DependencyCondition:         healthyCondition,
			Resolved:                    true,
		},
		{
			TargetKnown:                   apicontainerstatus.ContainerStatusNone,
			DependencyKnownHealthStatus:   apicontainerstatus.ContainerUnhealthy,
			HealthCheckType:               "docker",
			DependencyKnownHealthExitCode: 1,
			DependencyCondition:           healthyCondition,
			Resolved:                      true,
		},
		{
			TargetKnown:                   apicontainerstatus.ContainerPulled,
			DependencyKnownHealthStatus:   apicontainerstatus.ContainerUnhealthy,
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

func assertContainerOrderingHealthyConditionResolved(f func(target *apicontainer.Container, dep *apicontainer.Container, depCond string, cfg *config.Config) bool, targetDesired apicontainerstatus.ContainerStatus, targetKnown apicontainerstatus.ContainerStatus, depHealthKnown apicontainerstatus.ContainerHealthStatus, healthCheckEnabled string, depHealthKnownExitCode int, depCond string, expectedResolved bool, cfg *config.Config) func(t *testing.T) {
	return func(t *testing.T) {
		target := &apicontainer.Container{
			DesiredStatusUnsafe: targetDesired,
			KnownStatusUnsafe:   targetKnown,
		}
		dep := &apicontainer.Container{
			Health: apicontainer.HealthStatus{
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
			KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
		},
		"B": {
			Name:              "B",
			DependsOnUnsafe:   dependsOn("C", "D"),
			KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
		},
		"C": {
			Name:              "C",
			DependsOnUnsafe:   dependsOn("E", "F"),
			KnownStatusUnsafe: apicontainerstatus.ContainerStopped,
		},
		"D": {
			Name:              "D",
			DependsOnUnsafe:   dependsOn("E"),
			KnownStatusUnsafe: apicontainerstatus.ContainerRunning,
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
				KnownStatusUnsafe:   apicontainerstatus.ContainerRunning,
				DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
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
		KnownStatusUnsafe:   apicontainerstatus.ContainerPulled,
		DesiredStatusUnsafe: apicontainerstatus.ContainerCreated,
		DependsOnUnsafe: []apicontainer.DependsOn{
			{
				ContainerName: dependencyName,
			},
		},
	}
	dep := &apicontainer.Container{
		Name:                dependencyName,
		KnownStatusUnsafe:   apicontainerstatus.ContainerPulled,
		DesiredStatusUnsafe: apicontainerstatus.ContainerStopped,
		AppliedStatus:       apicontainerstatus.ContainerStatusNone,
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
