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

package container

import (
	"reflect"
	"testing"

	apierrors "github.com/aws/amazon-ecs-agent/agent/api/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/docker/go-connections/nat"
)

func TestPortBindingFromDockerPortBinding(t *testing.T) {
	pairs := []struct {
		dockerPortBindings nat.PortMap
		ecsPortBindings    []PortBinding
	}{
		{
			nat.PortMap{
				nat.Port("53/udp"): []nat.PortBinding{
					{HostIP: "1.2.3.4", HostPort: "55"},
				},
			},
			[]PortBinding{
				{
					BindIP:        "1.2.3.4",
					HostPort:      55,
					ContainerPort: aws.Uint16(53),
					Protocol:      TransportProtocolUDP,
				},
			},
		},
		{
			nat.PortMap{
				nat.Port("80/tcp"): []nat.PortBinding{
					{HostIP: "2.3.4.5", HostPort: "8080"},
					{HostIP: "5.6.7.8", HostPort: "80"},
				},
			},
			[]PortBinding{
				{
					BindIP:        "2.3.4.5",
					HostPort:      8080,
					ContainerPort: aws.Uint16(80),
					Protocol:      TransportProtocolTCP,
				},
				{
					BindIP:        "5.6.7.8",
					HostPort:      80,
					ContainerPort: aws.Uint16(80),
					Protocol:      TransportProtocolTCP,
				},
			},
		},
	}

	for i, pair := range pairs {
		converted, err := PortBindingFromDockerPortBinding(pair.dockerPortBindings)
		if err != nil {
			t.Errorf("Error converting port binding pair #%v: %v", i, err)
		}
		if !reflect.DeepEqual(pair.ecsPortBindings, converted) {
			t.Errorf("Converted bindings didn't match expected for #%v: expected %+v, actual %+v", i, pair.ecsPortBindings, converted)
		}
	}
}

func TestPortBindingErrors(t *testing.T) {
	badInputs := []struct {
		dockerPortBindings nat.PortMap
		errorName          string
	}{
		{
			nat.PortMap{
				nat.Port("woof/tcp"): []nat.PortBinding{
					{HostIP: "2.3.4.5", HostPort: "8080"},
					{HostIP: "5.6.7.8", HostPort: "80"},
				},
			},
			UnparseablePortErrorName,
		},
		{
			nat.PortMap{
				nat.Port("80/tcp"): []nat.PortBinding{
					{HostIP: "2.3.4.5", HostPort: "8080"},
					{HostIP: "5.6.7.8", HostPort: "bark"},
				},
			},
			UnparseablePortErrorName,
		},
		{
			nat.PortMap{
				nat.Port("80/bark"): []nat.PortBinding{
					{HostIP: "2.3.4.5", HostPort: "8080"},
					{HostIP: "5.6.7.8", HostPort: "80"},
				},
			},
			UnrecognizedTransportProtocolErrorName,
		},
	}

	for i, pair := range badInputs {
		_, err := PortBindingFromDockerPortBinding(pair.dockerPortBindings)
		if err == nil {
			t.Errorf("Expected error converting port binding pair #%v", i)
		}
		namedErr, ok := err.(apierrors.NamedError)
		if !ok {
			t.Errorf("Expected err to implement NamedError")
		}
		if namedErr.ErrorName() != pair.errorName {
			t.Errorf("Expected %s but was %s", pair.errorName, namedErr.ErrorName())
		}
	}
}
