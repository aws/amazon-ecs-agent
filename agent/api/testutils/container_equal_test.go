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

package testutils

import (
	"testing"

	. "github.com/aws/amazon-ecs-agent/agent/api"
)

func TestContainerEqual(t *testing.T) {
	one := 1
	onePtr := &one
	anotherOne := 1
	anotherOnePtr := &anotherOne
	two := 2
	twoPtr := &two
	equalPairs := []Container{
		Container{Name: "name"}, Container{Name: "name"},
		Container{Image: "nginx"}, Container{Image: "nginx"},
		Container{Command: []string{"c"}}, Container{Command: []string{"c"}},
		Container{Cpu: 1}, Container{Cpu: 1},
		Container{Memory: 1}, Container{Memory: 1},
		Container{Links: []string{"1", "2"}}, Container{Links: []string{"1", "2"}},
		Container{Links: []string{"1", "2"}}, Container{Links: []string{"2", "1"}},
		Container{VolumesFrom: []VolumeFrom{VolumeFrom{"1", false}, VolumeFrom{"2", true}}}, Container{VolumesFrom: []VolumeFrom{VolumeFrom{"1", false}, VolumeFrom{"2", true}}},
		Container{VolumesFrom: []VolumeFrom{VolumeFrom{"1", false}, VolumeFrom{"2", true}}}, Container{VolumesFrom: []VolumeFrom{VolumeFrom{"2", true}, VolumeFrom{"1", false}}},
		Container{Ports: []PortBinding{PortBinding{1, 2, "1"}}}, Container{Ports: []PortBinding{PortBinding{1, 2, "1"}}},
		Container{Essential: true}, Container{Essential: true},
		Container{EntryPoint: nil}, Container{EntryPoint: nil},
		Container{EntryPoint: &[]string{"1", "2"}}, Container{EntryPoint: &[]string{"1", "2"}},
		Container{Environment: map[string]string{}}, Container{Environment: map[string]string{}},
		Container{Environment: map[string]string{"a": "b", "c": "d"}}, Container{Environment: map[string]string{"c": "d", "a": "b"}},
		Container{DesiredStatus: ContainerRunning}, Container{DesiredStatus: ContainerRunning},
		Container{AppliedStatus: ContainerRunning}, Container{AppliedStatus: ContainerRunning},
		Container{KnownStatus: ContainerRunning}, Container{KnownStatus: ContainerRunning},
		Container{KnownExitCode: nil}, Container{KnownExitCode: nil},
		Container{KnownExitCode: onePtr}, Container{KnownExitCode: anotherOnePtr},
	}
	unequalPairs := []Container{
		Container{Name: "name"}, Container{Name: "名前"},
		Container{Image: "nginx"}, Container{Image: "えんじんえっくす"},
		Container{Command: []string{"c"}}, Container{Command: []string{"し"}},
		Container{Command: []string{"c", "b"}}, Container{Command: []string{"b", "c"}},
		Container{Cpu: 1}, Container{Cpu: 2e2},
		Container{Memory: 1}, Container{Memory: 2e2},
		Container{Links: []string{"1", "2"}}, Container{Links: []string{"1", "二"}},
		Container{VolumesFrom: []VolumeFrom{VolumeFrom{"1", false}, VolumeFrom{"2", true}}}, Container{VolumesFrom: []VolumeFrom{VolumeFrom{"1", false}, VolumeFrom{"二", false}}},
		Container{Ports: []PortBinding{PortBinding{1, 2, "1"}}}, Container{Ports: []PortBinding{PortBinding{1, 2, "二"}}},
		Container{Ports: []PortBinding{PortBinding{1, 2, "1"}}}, Container{Ports: []PortBinding{PortBinding{1, 22, "1"}}},
		Container{Essential: true}, Container{Essential: false},
		Container{EntryPoint: nil}, Container{EntryPoint: &[]string{"nonnil"}},
		Container{EntryPoint: &[]string{"1", "2"}}, Container{EntryPoint: &[]string{"2", "1"}},
		Container{EntryPoint: &[]string{"1", "2"}}, Container{EntryPoint: &[]string{"1", "二"}},
		Container{Environment: map[string]string{"a": "b", "c": "d"}}, Container{Environment: map[string]string{"し": "d", "a": "b"}},
		Container{DesiredStatus: ContainerRunning}, Container{DesiredStatus: ContainerStopped},
		Container{AppliedStatus: ContainerRunning}, Container{AppliedStatus: ContainerStopped},
		Container{KnownStatus: ContainerRunning}, Container{KnownStatus: ContainerStopped},
		Container{KnownExitCode: nil}, Container{KnownExitCode: onePtr},
		Container{KnownExitCode: onePtr}, Container{KnownExitCode: twoPtr},
	}

	for i := 0; i < len(equalPairs); i += 2 {
		if !ContainersEqual(&equalPairs[i], &equalPairs[i+1]) {
			t.Error(i, equalPairs[i], " should equal ", equalPairs[i+1])
		}
		// Should be symetric
		if !ContainersEqual(&equalPairs[i+1], &equalPairs[i]) {
			t.Error(i, "(symetric)", equalPairs[i+1], " should equal ", equalPairs[i])
		}
	}

	for i := 0; i < len(unequalPairs); i += 2 {
		if ContainersEqual(&unequalPairs[i], &unequalPairs[i+1]) {
			t.Error(i, unequalPairs[i], " shouldn't equal ", unequalPairs[i+1])
		}
		//symetric
		if ContainersEqual(&unequalPairs[i+1], &unequalPairs[i]) {
			t.Error(i, "(symetric)", unequalPairs[i+1], " shouldn't equal ", unequalPairs[i])
		}
	}
}
