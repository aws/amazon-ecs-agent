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

package testutils

import (
	"fmt"
	"testing"

	apicontainer "github.com/aws/amazon-ecs-agent/agent/api/container"
	apicontainerstatus "github.com/aws/amazon-ecs-agent/agent/api/container/status"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

func TestContainerEqual(t *testing.T) {

	exitCodeContainer := func(p *int) apicontainer.Container {
		c := apicontainer.Container{}
		c.SetKnownExitCode(p)
		return c
	}

	testCases := []struct {
		lhs           apicontainer.Container
		rhs           apicontainer.Container
		shouldBeEqual bool
	}{
		// Equal Pairs
		{apicontainer.Container{Name: "name"}, apicontainer.Container{Name: "name"}, true},
		{apicontainer.Container{Image: "nginx"}, apicontainer.Container{Image: "nginx"}, true},
		{apicontainer.Container{Command: []string{"c"}}, apicontainer.Container{Command: []string{"c"}}, true},
		{apicontainer.Container{CPU: 1}, apicontainer.Container{CPU: 1}, true},
		{apicontainer.Container{Memory: 1}, apicontainer.Container{Memory: 1}, true},
		{apicontainer.Container{Links: []string{"1", "2"}}, apicontainer.Container{Links: []string{"1", "2"}}, true},
		{apicontainer.Container{Links: []string{"1", "2"}}, apicontainer.Container{Links: []string{"2", "1"}}, true},
		{apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "1", Protocol: apicontainer.TransportProtocolTCP}}}, apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "1", Protocol: apicontainer.TransportProtocolTCP}}}, true},
		{apicontainer.Container{Essential: true}, apicontainer.Container{Essential: true}, true},
		{apicontainer.Container{EntryPoint: nil}, apicontainer.Container{EntryPoint: nil}, true},
		{apicontainer.Container{EntryPoint: &[]string{"1", "2"}}, apicontainer.Container{EntryPoint: &[]string{"1", "2"}}, true},
		{apicontainer.Container{Environment: map[string]string{}}, apicontainer.Container{Environment: map[string]string{}}, true},
		{apicontainer.Container{Environment: map[string]string{"a": "b", "c": "d"}}, apicontainer.Container{Environment: map[string]string{"c": "d", "a": "b"}}, true},
		{apicontainer.Container{DesiredStatusUnsafe: apicontainerstatus.ContainerRunning}, apicontainer.Container{DesiredStatusUnsafe: apicontainerstatus.ContainerRunning}, true},
		{apicontainer.Container{AppliedStatus: apicontainerstatus.ContainerRunning}, apicontainer.Container{AppliedStatus: apicontainerstatus.ContainerRunning}, true},
		{apicontainer.Container{KnownStatusUnsafe: apicontainerstatus.ContainerRunning}, apicontainer.Container{KnownStatusUnsafe: apicontainerstatus.ContainerRunning}, true},
		{exitCodeContainer(aws.Int(1)), exitCodeContainer(aws.Int(1)), true},
		{exitCodeContainer(nil), exitCodeContainer(nil), true},
		// Unequal Pairs
		{apicontainer.Container{Name: "name"}, apicontainer.Container{Name: "名前"}, false},
		{apicontainer.Container{Image: "nginx"}, apicontainer.Container{Image: "えんじんえっくす"}, false},
		{apicontainer.Container{Command: []string{"c"}}, apicontainer.Container{Command: []string{"し"}}, false},
		{apicontainer.Container{Command: []string{"c", "b"}}, apicontainer.Container{Command: []string{"b", "c"}}, false},
		{apicontainer.Container{CPU: 1}, apicontainer.Container{CPU: 2e2}, false},
		{apicontainer.Container{Memory: 1}, apicontainer.Container{Memory: 2e2}, false},
		{apicontainer.Container{Links: []string{"1", "2"}}, apicontainer.Container{Links: []string{"1", "二"}}, false},
		{apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "1", Protocol: apicontainer.TransportProtocolTCP}}}, apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "二", Protocol: apicontainer.TransportProtocolTCP}}}, false},
		{apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "1", Protocol: apicontainer.TransportProtocolTCP}}}, apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 22, BindIP: "1", Protocol: apicontainer.TransportProtocolTCP}}}, false},
		{apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "1", Protocol: apicontainer.TransportProtocolTCP}}}, apicontainer.Container{Ports: []apicontainer.PortBinding{{ContainerPort: aws.Uint16(1), HostPort: 2, BindIP: "1", Protocol: apicontainer.TransportProtocolUDP}}}, false},
		{apicontainer.Container{Essential: true}, apicontainer.Container{Essential: false}, false},
		{apicontainer.Container{EntryPoint: nil}, apicontainer.Container{EntryPoint: &[]string{"nonnil"}}, false},
		{apicontainer.Container{EntryPoint: &[]string{"1", "2"}}, apicontainer.Container{EntryPoint: &[]string{"2", "1"}}, false},
		{apicontainer.Container{EntryPoint: &[]string{"1", "2"}}, apicontainer.Container{EntryPoint: &[]string{"1", "二"}}, false},
		{apicontainer.Container{Environment: map[string]string{"a": "b", "c": "d"}}, apicontainer.Container{Environment: map[string]string{"し": "d", "a": "b"}}, false},
		{apicontainer.Container{DesiredStatusUnsafe: apicontainerstatus.ContainerRunning}, apicontainer.Container{DesiredStatusUnsafe: apicontainerstatus.ContainerStopped}, false},
		{apicontainer.Container{AppliedStatus: apicontainerstatus.ContainerRunning}, apicontainer.Container{AppliedStatus: apicontainerstatus.ContainerStopped}, false},
		{apicontainer.Container{KnownStatusUnsafe: apicontainerstatus.ContainerRunning}, apicontainer.Container{KnownStatusUnsafe: apicontainerstatus.ContainerStopped}, false},
		{exitCodeContainer(aws.Int(0)), exitCodeContainer(aws.Int(42)), false},
		{exitCodeContainer(nil), exitCodeContainer(aws.Int(12)), false},
	}

	for index, tc := range testCases {
		t.Run(fmt.Sprintf("index %d expected %t", index, tc.shouldBeEqual), func(t *testing.T) {
			assert.Equal(t, ContainersEqual(&tc.lhs, &tc.rhs), tc.shouldBeEqual, "ContainersEqual not working as expected. Check index failure.")
			// Symetric
			assert.Equal(t, ContainersEqual(&tc.rhs, &tc.lhs), tc.shouldBeEqual, "Symetric equality check failed. Check index failure.")
		})
	}
}
