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

	"github.com/aws/amazon-ecs-agent/agent/api"
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
		Name:          name,
		Links:         links,
		VolumesFrom:   volumeStrToVol(volumes),
		DesiredStatus: api.ContainerRunning,
	}
}
func createdContainer(name string, links, volumes []string) *api.Container {
	return &api.Container{
		Name:          name,
		Links:         links,
		VolumesFrom:   volumeStrToVol(volumes),
		DesiredStatus: api.ContainerCreated,
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
			&api.Container{
				Name:          "redis",
				DesiredStatus: api.ContainerRunning,
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
			&api.Container{
				Name:          "redis",
				DesiredStatus: api.ContainerRunning,
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
	dbdata.KnownStatus = api.ContainerCreated

	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Php shouldn't run, db is not created")
	}
	db.KnownStatus = api.ContainerCreated
	resolved = DependenciesAreResolved(php, task.Containers)
	if resolved {
		t.Error("Php shouldn't run, db is not running")
	}

	resolved = DependenciesAreResolved(db, task.Containers)
	if !resolved {
		t.Error("db should be resolved, dbdata volume is Created")
	}
	db.KnownStatus = api.ContainerRunning

	resolved = DependenciesAreResolved(php, task.Containers)
	if !resolved {
		t.Error("Php should resolve")
	}
}

func TestRunningependsOnDependencies(t *testing.T) {
	c1 := &api.Container{
		Name:        "a",
		KnownStatus: api.ContainerStatusNone,
	}
	c2 := &api.Container{
		Name:            "b",
		KnownStatus:     api.ContainerStatusNone,
		DesiredStatus:   api.ContainerCreated,
		RunDependencies: []string{"a"},
	}
	task := &api.Task{Containers: []*api.Container{c1, c2}}

	if DependenciesAreResolved(c2, task.Containers) {
		t.Error("Dependencies should not be resolved")
	}
	task.Containers[1].DesiredStatus = api.ContainerRunning
	if DependenciesAreResolved(c2, task.Containers) {
		t.Error("Dependencies should not be resolved")
	}
	task.Containers[0].KnownStatus = api.ContainerRunning

	if !DependenciesAreResolved(c2, task.Containers) {
		t.Error("Dependencies should be resolved")
	}
	task.Containers[1].DesiredStatus = api.ContainerCreated
	if !DependenciesAreResolved(c1, task.Containers) {
		t.Error("Dependencies should be resolved")
	}
}
