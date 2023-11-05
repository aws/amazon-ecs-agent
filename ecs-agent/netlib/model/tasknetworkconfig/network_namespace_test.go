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

package tasknetworkconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetworkNamespace_GetPrimaryInterface(t *testing.T) {
	netns := &NetworkNamespace{
		NetworkInterfaces: getTestNetworkInterfaces(),
	}
	assert.Equal(t, primaryInterfaceName, netns.GetPrimaryInterface().Name)

	netns = &NetworkNamespace{}
	assert.Empty(t, netns.GetPrimaryInterface())
}

// TestNewNetworkNamespace tests creation of a new NetworkNamespace object.
func TestNewNetworkNamespace(t *testing.T) {
	netIFs := getTestNetworkInterfaces()
	netns, err := NewNetworkNamespace(
		primaryNetNSName,
		primaryNetNSPath,
		0,
		nil,
		netIFs...)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(netns.NetworkInterfaces))
	assert.Equal(t, primaryNetNSName, netns.Name)
	assert.Equal(t, primaryNetNSPath, netns.Path)
	assert.Equal(t, 0, netns.Index)
	assert.Empty(t, netns.AppMeshConfig)
	assert.Equal(t, *netIFs[0], *netns.NetworkInterfaces[0])
	assert.Equal(t, *netIFs[1], *netns.NetworkInterfaces[1])
}
