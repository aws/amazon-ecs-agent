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
	"net/netip"
	"reflect"
	"testing"

	apierrors "github.com/aws/amazon-ecs-agent/ecs-agent/api/errors"

	"github.com/moby/moby/api/types/network"
)

func TestPortBindingFromDockerPortBinding(t *testing.T) {
	pairs := []struct {
		dockerPortBindings network.PortMap
		ecsPortBindings    []PortBinding
	}{
		{
			network.PortMap{
				network.MustParsePort("53/udp"): []network.PortBinding{
					{HostIP: netip.MustParseAddr("1.2.3.4"), HostPort: "55"},
				},
			},
			[]PortBinding{
				{
					BindIP:        "1.2.3.4",
					HostPort:      55,
					ContainerPort: 53,
					Protocol:      TransportProtocolUDP,
				},
			},
		},
		{
			network.PortMap{
				network.MustParsePort("80/tcp"): []network.PortBinding{
					{HostIP: netip.MustParseAddr("2.3.4.5"), HostPort: "8080"},
					{HostIP: netip.MustParseAddr("5.6.7.8"), HostPort: "80"},
				},
			},
			[]PortBinding{
				{
					BindIP:        "2.3.4.5",
					HostPort:      8080,
					ContainerPort: 80,
					Protocol:      TransportProtocolTCP,
				},
				{
					BindIP:        "5.6.7.8",
					HostPort:      80,
					ContainerPort: 80,
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
	// Note: moby v29's network.Port is a validated struct, so a port with a
	// non-numeric container port (e.g. "woof/tcp") can no longer be
	// represented as input; that error path is unreachable and is not tested
	// here. The remaining cases still cover both error names.
	badInputs := []struct {
		dockerPortBindings network.PortMap
		errorName          string
	}{
		{
			network.PortMap{
				network.MustParsePort("80/tcp"): []network.PortBinding{
					{HostIP: netip.MustParseAddr("2.3.4.5"), HostPort: "8080"},
					{HostIP: netip.MustParseAddr("5.6.7.8"), HostPort: "bark"},
				},
			},
			UnparseablePortErrorName,
		},
		{
			network.PortMap{
				network.MustParsePort("80/bark"): []network.PortBinding{
					{HostIP: netip.MustParseAddr("2.3.4.5"), HostPort: "8080"},
					{HostIP: netip.MustParseAddr("5.6.7.8"), HostPort: "80"},
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
