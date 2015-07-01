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
				Ports: []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}},
			},
		},
	}

	overridden := testTask.Overridden()
	if overridden.Containers[0] == testTask.Containers[0] {
		t.Error("Containers were pointer-equal, not overridden")
	}
}

func TestDockerConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}, PortBinding{20, 20, "", TransportProtocolUDP}},
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	_, ok := config.ExposedPorts["10/tcp"]
	if !ok {
		t.Fatal("Could not get exposed ports 10/tcp")
	}
	_, ok = config.ExposedPorts["20/udp"]
	if !ok {
		t.Fatal("Could not get exposed ports 20/udp")
	}
}

func TestDockerConfigCPUShareZero(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
				Cpu:  0,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 2 {
		t.Error("CPU shares of 0 did not get changed to 2")
	}
}

func TestDockerConfigCPUShareMinimum(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
				Cpu:  1,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 2 {
		t.Error("CPU shares of 1 did not get changed to 2")
	}
}

func TestDockerConfigCPUShareUnchanged(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name: "c1",
				Cpu:  100,
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Error(err)
	}

	if config.CPUShares != 100 {
		t.Error("CPU shares unexpectedly changed")
	}
}

func TestDockerHostConfigPortBinding(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
				Ports: []PortBinding{PortBinding{10, 10, "", TransportProtocolTCP}, PortBinding{20, 20, "", TransportProtocolUDP}},
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
	bindings, ok = config.PortBindings["20/udp"]
	if !ok {
		t.Fatal("Could not get port bindings")
	}
	if len(bindings) != 1 {
		t.Fatal("Wrong number of bindings")
	}
	if bindings[0].HostPort != "20" {
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

func TestDockerConfigLabels(t *testing.T) {
	testTask := &Task{
		Arn:     "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		Family:  "myFamily",
		Version: "1",
		Containers: []*Container{
			&Container{
				Name: "c1",
			},
		},
	}

	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]string{
		"com.amazonaws.ecs.task-arn":                "arn:aws:ecs:us-east-1:012345678910:task/c09f0188-7f87-4b0f-bfc3-16296622b6fe",
		"com.amazonaws.ecs.container-name":          "c1",
		"com.amazonaws.ecs.task-definition-family":  "myFamily",
		"com.amazonaws.ecs.task-definition-version": "1",
	}
	if !reflect.DeepEqual(config.Labels, expected) {
		t.Fatal("Expected default ecs labels to be set, was: ", config.Labels)
	}
}

func TestDockerRunCommandFlags(t *testing.T) {
	testTask := &Task{
		Containers: []*Container{
			&Container{
				Name:  "c1",
			        Tty: true,
			        OpenStdin: true,
			},
		},
	}
	config, err := testTask.DockerConfig(testTask.Containers[0])
	if err != nil {
		t.Fatal(err)
	}

	if !config.Tty {
		t.Fatal("Expected tty to be true, got false")
	}
	if !config.OpenStdin {
		t.Fatal("Expected openStdin to be true, got false")
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
						Protocol:      strptr("udp"),
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
						Protocol:      TransportProtocolUDP,
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
		StartSequenceNumber: 42,
	}

	seqNum := int64(42)
	task, err := TaskFromACS(&taskFromAcs, &ecsacs.PayloadMessage{SeqNum: &seqNum})
	if err != nil {
		t.Fatalf("Should be able to handle acs task: %v", err)
	}
	if !reflect.DeepEqual(task.Containers, expectedTask.Containers) {
		t.Fatal("Should be equal")
	}
	if !reflect.DeepEqual(task.Volumes, expectedTask.Volumes) {
		t.Fatal("Should be equal")
	}
	if !reflect.DeepEqual(task.StartSequenceNumber, expectedTask.StartSequenceNumber) {
		t.Fatal("Should be equal")
	}
	if !reflect.DeepEqual(task.StopSequenceNumber, expectedTask.StopSequenceNumber) {
		t.Fatal("Should be equal")
	}
}
