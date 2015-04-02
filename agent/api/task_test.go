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

	"github.com/aws/amazon-ecs-agent/agent/acs/model/ecsacs"
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

func TestTaskFromACS(t *testing.T) {
	strptr := func(s string) *string {
		return &s
	}
	intptr := func(i int64) *int64 {
		return &i
	}
	boolptr := func(b bool) *bool {
		return &b
	}
	// Testing type conversions, bleh. At least the type conversion itself
	// doesn't look this messy.
	taskFromAcs := ecsacs.Task{
		Arn:           strptr("myArn"),
		DesiredStatus: strptr("RUNNING"),
		Family:        strptr("myFamily"),
		Version:       strptr("1"),
		Containers: []*ecsacs.Container{
			&ecsacs.Container{
				Name:        strptr("myName"),
				Cpu:         intptr(10),
				Command:     []*string{strptr("command"), strptr("command2")},
				EntryPoint:  []*string{strptr("sh"), strptr("-c")},
				Environment: &map[string]*string{"key": strptr("value")},
				Essential:   boolptr(true),
				Image:       strptr("image:tag"),
				Links:       []*string{strptr("link1"), strptr("link2")},
				Memory:      intptr(100),
				MountPoints: []*ecsacs.MountPoint{
					&ecsacs.MountPoint{
						ContainerPath: strptr("/container/path"),
						ReadOnly:      boolptr(true),
						SourceVolume:  strptr("sourceVolume"),
					},
				},
				Overrides: strptr(`{"command":["a","b","c"]}`),
				PortMappings: []*ecsacs.PortMapping{
					&ecsacs.PortMapping{
						HostPort:      intptr(800),
						ContainerPort: intptr(900),
					},
				},
				VolumesFrom: []*ecsacs.VolumeFrom{
					&ecsacs.VolumeFrom{
						ReadOnly:        boolptr(true),
						SourceContainer: strptr("volumeLink"),
					},
				},
			},
		},
		Volumes: []*ecsacs.Volume{
			&ecsacs.Volume{
				Name: strptr("volName"),
				Host: &ecsacs.HostVolumeProperties{
					SourcePath: strptr("/host/path"),
				},
			},
		},
	}
	expectedTask := &Task{
		Arn:           "myArn",
		DesiredStatus: TaskRunning,
		Family:        "myFamily",
		Version:       "1",
		Containers: []*Container{
			&Container{
				Name:        "myName",
				Image:       "image:tag",
				Command:     []string{"command", "command2"},
				Links:       []string{"link1", "link2"},
				EntryPoint:  &[]string{"sh", "-c"},
				Essential:   true,
				Environment: map[string]string{"key": "value"},
				Cpu:         10,
				Memory:      100,
				MountPoints: []MountPoint{
					MountPoint{
						ContainerPath: "/container/path",
						ReadOnly:      true,
						SourceVolume:  "sourceVolume",
					},
				},
				Overrides: ContainerOverrides{
					Command: &[]string{"a", "b", "c"},
				},
				Ports: []PortBinding{
					PortBinding{
						HostPort:      800,
						ContainerPort: 900,
					},
				},
				VolumesFrom: []VolumeFrom{
					VolumeFrom{
						ReadOnly:        true,
						SourceContainer: "volumeLink",
					},
				},
			},
		},
		Volumes: []TaskVolume{
			TaskVolume{
				Name: "volName",
				Volume: &FSHostVolume{
					FSSourcePath: "/host/path",
				},
			},
		},
	}

	task, err := TaskFromACS(&taskFromAcs)
	if err != nil {
		t.Fatalf("Should be able to handle acs task: %v", err)
	}
	if !reflect.DeepEqual(task.Containers, expectedTask.Containers) {
		t.Fatal("Should be equal")
	}
}
