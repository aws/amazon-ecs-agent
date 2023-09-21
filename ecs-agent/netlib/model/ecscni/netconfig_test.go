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

//go:build unit
// +build unit

package ecscni

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildNetworkConfig(t *testing.T) {
	nsPath := "nspath"
	cfg := &TestCNIConfig{
		CNIConfig: CNIConfig{
			NetNSPath: nsPath,
		},
		NetworkInterfaceName: DefaultInterfaceName,
	}

	rt := BuildRuntimeConfig(cfg)
	require.Equal(t, nsPath, rt.NetNS)
	require.Equal(t, DefaultInterfaceName, rt.IfName)
	require.Equal(t, "container-id", rt.ContainerID)
}
