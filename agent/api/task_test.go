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

package api

import (
	"reflect"
	"testing"
)

func dockerMap(task *Task) map[string]*DockerContainer {
	m := make(map[string]*DockerContainer)
	for _, c := range task.Containers {
		m[c.Name] = &DockerContainer{DockerId: "dockerid-" + c.Name, DockerName: "dockername-" + c.Name, Container: c}
	}
	return m
}

func TestTaskOverridden(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, ""}},
			},
		},
	}

	overridden := testTask.Overridden()
	if overridden.Containers[0] == testTask.Containers[0] {
		t.Error("Containers were pointer-equal, not overridden")
	}
}

func TestDockerHostConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, ""}},
			},
		},
	}

	config, err := testTask.DockerHostConfig(testTask.Containers[0], dockerMap(testTask))
	if err != nil {
		t.Error(err)
	}

	bindings, ok := config.PortBindings["10/tcp"]
	if !ok {
		t.Fatal("Could not get port bindings")
	}
	if len(bindings) != 1 {
		t.Fatal("Wrong number of bindings")
	}
	if bindings[0].HostPort != "10" {
		t.Error("Wrong hostport")
	}
	if bindings[0].HostIP != "0.0.0.0" {
		t.Error("Wrong hostIP")
	}
}

func TestDockerHostConfigVolumesFrom(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
			},
			&Container{
				Name:        "c2",
				VolumesFrom: []VolumeFrom{VolumeFrom{SourceContainer: "c1"}},
			},
		},
	}

	config, err := testTask.DockerHostConfig(testTask.Containers[1], dockerMap(testTask))
	if err != nil {
		t.Fatal("Error creating config: ", err)
	}
	if !reflect.DeepEqual(config.VolumesFrom, []string{"dockername-c1"}) {
		t.Error("Expected volumesFrom to be resolved, was: ", config.VolumesFrom)
	}
}
