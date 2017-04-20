// Copyright 2014-2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
		{Name: "name"}, {Name: "name"},
		{Image: "nginx"}, {Image: "nginx"},
		{Command: []string{"c"}}, {Command: []string{"c"}},
		{CPU: 1}, {CPU: 1},
		{Memory: 1}, {Memory: 1},
		{Links: []string{"1", "2"}}, {Links: []string{"1", "2"}},
		{Links: []string{"1", "2"}}, {Links: []string{"2", "1"}},
		{VolumesFrom: []VolumeFrom{{"1", false}, {"2", true}}}, {VolumesFrom: []VolumeFrom{{"1", false}, {"2", true}}},
		{VolumesFrom: []VolumeFrom{{"1", false}, {"2", true}}}, {VolumesFrom: []VolumeFrom{{"2", true}, {"1", false}}},
		{Ports: []PortBinding{{1, 2, "1", TransportProtocolTCP}}}, {Ports: []PortBinding{{1, 2, "1", TransportProtocolTCP}}},
		{Essential: true}, {Essential: true},
		{EntryPoint: nil}, {EntryPoint: nil},
		{EntryPoint: &[]string{"1", "2"}}, {EntryPoint: &[]string{"1", "2"}},
		{Environment: map[string]string{}}, {Environment: map[string]string{}},
		{Environment: map[string]string{"a": "b", "c": "d"}}, {Environment: map[string]string{"c": "d", "a": "b"}},
		{DesiredStatusUnsafe: ContainerRunning}, {DesiredStatusUnsafe: ContainerRunning},
		{AppliedStatus: ContainerRunning}, {AppliedStatus: ContainerRunning},
		{KnownStatusUnsafe: ContainerRunning}, {KnownStatusUnsafe: ContainerRunning},
		{KnownExitCode: nil}, {KnownExitCode: nil},
		{KnownExitCode: onePtr}, {KnownExitCode: anotherOnePtr},
	}
	unequalPairs := []Container{
		{Name: "name"}, {Name: "名前"},
		{Image: "nginx"}, {Image: "えんじんえっくす"},
		{Command: []string{"c"}}, {Command: []string{"し"}},
		{Command: []string{"c", "b"}}, {Command: []string{"b", "c"}},
		{CPU: 1}, {CPU: 2e2},
		{Memory: 1}, {Memory: 2e2},
		{Links: []string{"1", "2"}}, {Links: []string{"1", "二"}},
		{VolumesFrom: []VolumeFrom{{"1", false}, {"2", true}}}, {VolumesFrom: []VolumeFrom{{"1", false}, {"二", false}}},
		{Ports: []PortBinding{{1, 2, "1", TransportProtocolTCP}}}, {Ports: []PortBinding{{1, 2, "二", TransportProtocolTCP}}},
		{Ports: []PortBinding{{1, 2, "1", TransportProtocolTCP}}}, {Ports: []PortBinding{{1, 22, "1", TransportProtocolTCP}}},
		{Ports: []PortBinding{{1, 2, "1", TransportProtocolTCP}}}, {Ports: []PortBinding{{1, 2, "1", TransportProtocolUDP}}},
		{Essential: true}, {Essential: false},
		{EntryPoint: nil}, {EntryPoint: &[]string{"nonnil"}},
		{EntryPoint: &[]string{"1", "2"}}, {EntryPoint: &[]string{"2", "1"}},
		{EntryPoint: &[]string{"1", "2"}}, {EntryPoint: &[]string{"1", "二"}},
		{Environment: map[string]string{"a": "b", "c": "d"}}, {Environment: map[string]string{"し": "d", "a": "b"}},
		{DesiredStatusUnsafe: ContainerRunning}, {DesiredStatusUnsafe: ContainerStopped},
		{AppliedStatus: ContainerRunning}, {AppliedStatus: ContainerStopped},
		{KnownStatusUnsafe: ContainerRunning}, {KnownStatusUnsafe: ContainerStopped},
		{KnownExitCode: nil}, {KnownExitCode: onePtr},
		{KnownExitCode: onePtr}, {KnownExitCode: twoPtr},
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
