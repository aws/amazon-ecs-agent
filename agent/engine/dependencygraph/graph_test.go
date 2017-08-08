// +build !integration
// Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

	"fmt"

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

func runningContainer(name string, links, volumes []string) *api.Container {
	return &api.Container{
		Name:                name,
		Links:               links,
		VolumesFrom:         volumeStrToVol(volumes),
		DesiredStatusUnsafe: api.ContainerRunning,
	}
}
func createdContainer(name string, links, volumes []string) *api.Container {
	return &api.Container{
		Name:                name,
		Links:               links,
		VolumesFrom:         volumeStrToVol(volumes),
		DesiredStatusUnsafe: api.ContainerCreated,
	}
}

func TestValidDependencies(t *testing.T) {
	// Empty task
	task := &api.Task{}
	resolveable := ValidDependencies(task)
	if !resolveable {
		t.Error("The zero dependency graph should resolve")
	}

	task = &api.Task{
		Containers: []*api.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: api.ContainerRunning,
			},
		},
	}
	resolveable = ValidDependencies(task)
	if !resolveable {
		t.Error("One container should resolve trivially")
	}

	// Webserver stack
	php := runningContainer("php", []string{"db"}, []string{})
	db := runningContainer("db", []string{}, []string{"dbdatavolume"})
	dbdata := createdContainer("dbdatavolume", []string{}, []string{})
	webserver := runningContainer("webserver", []string{"php"}, []string{"htmldata"})
	htmldata := runningContainer("htmldata", []string{}, []string{"sharedcssfiles"})
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{})

	task = &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolveable = ValidDependencies(task)
	if !resolveable {
		t.Error("The webserver group should resolve just fine")
	}

	// Unresolveable: cycle
	task = &api.Task{
		Containers: []*api.Container{
			runningContainer("a", []string{"b"}, []string{}),
			runningContainer("b", []string{"a"}, []string{}),
		},
	}
	resolveable = ValidDependencies(task)
	if resolveable {
		t.Error("Cycle should not be resolveable")
	}
	// Unresolveable, reference doesn't exist
	task = &api.Task{
		Containers: []*api.Container{
			runningContainer("php", []string{"db"}, []string{}),
		},
	}
	resolveable = ValidDependencies(task)
	if resolveable {
		t.Error("Nonexistent reference shouldn't resolve")
	}
}

func TestDependenciesAreResolved(t *testing.T) {
	task := &api.Task{
		Containers: []*api.Container{
			{
				Name:                "redis",
				DesiredStatusUnsafe: api.ContainerRunning,
			},
		},
	}
	resolved := DependenciesAreResolved(task.Containers[0], task.Containers)
	if !resolved {
		t.Error("One container should resolve trivially")
	}

	// Webserver stack
	php := runningContainer("php", []string{"db"}, []string{})
	db := runningContainer("db", []string{}, []string{"dbdatavolume"})
	dbdata := createdContainer("dbdatavolume", []string{}, []string{})
	webserver := runningContainer("webserver", []string{"php"}, []string{"htmldata"})
	htmldata := runningContainer("htmldata", []string{}, []string{"sharedcssfiles"})
	sharedcssfiles := createdContainer("sharedcssfiles", []string{}, []string{})

	task = &api.Task{
		Containers: []*api.Container{
			php, db, dbdata, webserver, htmldata, sharedcssfiles,
		},
	}

	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Shouldn't be resolved; db isn't running")
	}
	resolved = DependenciesAreResolved(db, task.Containers)
	if resolved {
		t.Error("Shouldn't be resolved; dbdatavolume isn't created")
	}
	resolved = DependenciesAreResolved(dbdata, task.Containers)
	if !resolved {
		t.Error("data volume with no deps should resolve")
	}
	dbdata.KnownStatusUnsafe = api.ContainerCreated

	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Php shouldn't run, db is not created")
	}
	db.KnownStatusUnsafe = api.ContainerCreated
	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Php shouldn't run, db is not running")
	}

	resolved = DependenciesAreResolved(db, task.Containers)
	if !resolved {
		t.Error("db should be resolved, dbdata volume is Created")
	}
	db.KnownStatusUnsafe = api.ContainerRunning

	resolved = DependenciesAreResolved(php, task.Containers)
	if !resolved {
		t.Error("Php should resolve")
	}
}

func TestRunningDependsOnDependencies(t *testing.T) {
	c1 := &api.Container{
		Name:              "a",
		KnownStatusUnsafe: api.ContainerStatusNone,
	}
	c2 := &api.Container{
		Name:                "b",
		KnownStatusUnsafe:   api.ContainerStatusNone,
		DesiredStatusUnsafe: api.ContainerCreated,
		RunDependencies:     []string{"a"},
	}
	task := &api.Task{Containers: []*api.Container{c1, c2}}

	if DependenciesAreResolved(c2, task.Containers) {
		t.Error("Dependencies should not be resolved")
	}
	task.Containers[1].SetDesiredStatus(api.ContainerRunning)
	if DependenciesAreResolved(c2, task.Containers) {
		t.Error("Dependencies should not be resolved")
	}
	task.Containers[0].KnownStatusUnsafe = api.ContainerRunning

	if !DependenciesAreResolved(c2, task.Containers) {
		t.Error("Dependencies should be resolved")
	}
	task.Containers[1].SetDesiredStatus(api.ContainerCreated)
	if !DependenciesAreResolved(c1, task.Containers) {
		t.Error("Dependencies should be resolved")
	}
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

func TestOnRunIsResolved(t *testing.T) {
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
			assertResolved(onRunIsResolved, tc.TargetDesired, tc.RunKnown, tc.Resolved))
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
